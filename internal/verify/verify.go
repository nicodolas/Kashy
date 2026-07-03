// Package verify implements the Kashy Verify Gate:
// auto-detect test suite → run tests → LLM review → verdict.
package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Verdict is the outcome of a verification pass.
type Verdict string

const (
	VerdictAutoChecked  Verdict = "AUTO-CHECKED"   // tests pass + LLM OK
	VerdictNeedsReview  Verdict = "NEEDS-REVIEW"   // tests pass + LLM warns
	VerdictFailed       Verdict = "FAILED"         // tests failed
	VerdictNoTests      Verdict = "NO-TESTS"       // no test suite found, LLM only
)

// Result holds the full verification outcome.
type Result struct {
	Verdict    Verdict
	TestOutput string
	LLMOutput  string
	Details    string
}

// DetectTestCommand scans targetDir for a known test runner.
// Returns ("", false) when none is found.
func DetectTestCommand(targetDir string) (cmd string, found bool) {
	// package.json scripts.test
	pkgJSON := filepath.Join(targetDir, "package.json")
	if data, err := os.ReadFile(pkgJSON); err == nil {
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		if json.Unmarshal(data, &pkg) == nil {
			if t, ok := pkg.Scripts["test"]; ok && t != "" {
				return "npm test", true
			}
		}
	}

	// pytest
	for _, name := range []string{"pytest.ini", "setup.cfg", "pyproject.toml"} {
		if _, err := os.Stat(filepath.Join(targetDir, name)); err == nil {
			return "pytest", true
		}
	}

	// go test
	if _, err := os.Stat(filepath.Join(targetDir, "go.mod")); err == nil {
		return "go test ./...", true
	}

	// Cargo (Rust)
	if _, err := os.Stat(filepath.Join(targetDir, "Cargo.toml")); err == nil {
		return "cargo test", true
	}

	return "", false
}

// RunTests executes the detected test command in targetDir with a 120s timeout.
func RunTests(targetDir string) (passed bool, output string, err error) {
	cmdStr, found := DetectTestCommand(targetDir)
	if !found {
		return false, "", fmt.Errorf("no test suite detected")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	parts := strings.Fields(cmdStr)
	if len(parts) == 1 {
		cmd = exec.CommandContext(ctx, parts[0])
	} else {
		cmd = exec.CommandContext(ctx, parts[0], parts[1:]...)
	}
	cmd.Dir = targetDir

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	runErr := cmd.Run()
	out := buf.String()
	if runErr != nil {
		return false, out, nil // not an error — just tests failed
	}
	return true, out, nil
}

// LLMReviewRequest is sent to the LLM for code review.
type LLMReviewRequest struct {
	Task        string
	Diff        string
	BaseURL     string
	APIKey      string
	Model       string // default: "anthropic/claude-3-haiku"
}

// LLMReview calls a cheap model to review the diff against the task description.
// Returns PASS, WARN, or FAIL plus the model's explanation.
func LLMReview(req LLMReviewRequest) (verdict string, explanation string, err error) {
	if req.Model == "" {
		req.Model = "anthropic/claude-3-haiku"
	}

	prompt := fmt.Sprintf(`You are a senior code reviewer. Review the following code diff against the original task.

TASK:
%s

DIFF:
%s

Respond with exactly one of: PASS, WARN, or FAIL on the first line.
Then explain briefly (2-5 sentences) why.

PASS = change looks correct, no obvious issues
WARN = change is mostly correct but has minor issues or missing edge cases
FAIL = change has significant bugs, spec violations, or security issues`, req.Task, req.Diff)

	payload := map[string]interface{}{
		"model": req.Model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": 300,
	}
	body, _ := json.Marshal(payload)

	httpReq, err := http.NewRequest("POST", req.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if req.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", "", fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read response: %w", err)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil || len(result.Choices) == 0 {
		return "", "", fmt.Errorf("parse response: %w (body: %s)", err, string(respBody))
	}

	content := strings.TrimSpace(result.Choices[0].Message.Content)
	lines := strings.SplitN(content, "\n", 2)
	firstLine := strings.ToUpper(strings.TrimSpace(lines[0]))
	explanation = ""
	if len(lines) > 1 {
		explanation = strings.TrimSpace(lines[1])
	}

	switch {
	case strings.HasPrefix(firstLine, "PASS"):
		return "PASS", explanation, nil
	case strings.HasPrefix(firstLine, "WARN"):
		return "WARN", explanation, nil
	case strings.HasPrefix(firstLine, "FAIL"):
		return "FAIL", explanation, nil
	default:
		return "WARN", content, nil // unknown response → conservative warn
	}
}

// GateConfig holds config for VerifyDone.
type GateConfig struct {
	TargetDir string
	Task      string
	Diff      string // optional: if empty, auto-generates from git diff HEAD
	// LLM config (optional — if empty, skip LLM review)
	LLMBaseURL string
	LLMAPIKey  string
	LLMModel   string
}

// autoGitDiff runs `git diff HEAD` in targetDir and returns the output.
// Returns empty string if git is unavailable or no changes.
func autoGitDiff(targetDir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", targetDir, "diff", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	diff := string(out)
	if len(diff) > 8000 {
		diff = diff[:8000] + "\n... (truncated)"
	}
	return diff
}

// VerifyDone runs the full verification pipeline:
// 1. Detect + run tests
// 2. LLM review (if LLMBaseURL is set)
// 3. Return combined verdict
func VerifyDone(cfg GateConfig) Result {
	var testOutput string
	var testsPassed bool

	cmdStr, found := DetectTestCommand(cfg.TargetDir)
	if !found {
		testOutput = "No test suite detected — running LLM review only."
	} else {
		var err error
		testsPassed, testOutput, err = RunTests(cfg.TargetDir)
		if err != nil {
			testOutput = fmt.Sprintf("Test runner error: %v", err)
		}
		if !testsPassed {
			return Result{
				Verdict:    VerdictFailed,
				TestOutput: fmt.Sprintf("[%s] %s", cmdStr, testOutput),
				Details:    "Tests failed. Fix before marking done.",
			}
		}
	}

	// LLM review
	llmVerdict := "PASS"
	llmOutput := ""
	if cfg.LLMBaseURL != "" && (cfg.Task != "" || cfg.Diff != "") {
		// Auto-generate git diff if not provided
		diff := cfg.Diff
		if diff == "" && cfg.TargetDir != "" {
			diff = autoGitDiff(cfg.TargetDir)
		}
		v, explanation, err := LLMReview(LLMReviewRequest{
			Task:    cfg.Task,
			Diff:    diff,
			BaseURL: cfg.LLMBaseURL,
			APIKey:  cfg.LLMAPIKey,
			Model:   cfg.LLMModel,
		})
		if err != nil {
			llmOutput = fmt.Sprintf("LLM review unavailable: %v", err)
			llmVerdict = "WARN"
		} else {
			llmVerdict = v
			llmOutput = explanation
		}
	}

	switch llmVerdict {
	case "FAIL":
		return Result{
			Verdict:    VerdictFailed,
			TestOutput: testOutput,
			LLMOutput:  llmOutput,
			Details:    "LLM review: FAIL. " + llmOutput,
		}
	case "WARN":
		return Result{
			Verdict:    VerdictNeedsReview,
			TestOutput: testOutput,
			LLMOutput:  llmOutput,
			Details:    "LLM review flagged issues: " + llmOutput,
		}
	default:
		if !found {
			return Result{
				Verdict:    VerdictNoTests,
				TestOutput: testOutput,
				LLMOutput:  llmOutput,
				Details:    "No test suite. LLM review: PASS.",
			}
		}
		return Result{
			Verdict:    VerdictAutoChecked,
			TestOutput: testOutput,
			LLMOutput:  llmOutput,
			Details:    "Tests pass. LLM review: PASS.",
		}
	}
}

