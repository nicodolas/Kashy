package verify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectTestCommandGoMod(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/x\ngo 1.21\n"), 0644)
	cmd, found := DetectTestCommand(dir)
	if !found {
		t.Fatal("expected to find go test")
	}
	if cmd != "go test ./..." {
		t.Errorf("got %q, want %q", cmd, "go test ./...")
	}
}

func TestDetectTestCommandNpmPackageJson(t *testing.T) {
	dir := t.TempDir()
	pkg := map[string]interface{}{
		"scripts": map[string]string{"test": "jest"},
	}
	data, _ := json.Marshal(pkg)
	os.WriteFile(filepath.Join(dir, "package.json"), data, 0644)
	cmd, found := DetectTestCommand(dir)
	if !found {
		t.Fatal("expected to find npm test")
	}
	if cmd != "npm test" {
		t.Errorf("got %q, want %q", cmd, "npm test")
	}
}

func TestDetectTestCommandNotFound(t *testing.T) {
	dir := t.TempDir()
	// empty dir — no known test file
	_, found := DetectTestCommand(dir)
	if found {
		t.Error("expected not found in empty dir")
	}
}

// TestDetectTestCommandCargo: dir có Cargo.toml → "cargo test"
func TestDetectTestCommandCargo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"myapp\"\n"), 0644)
	cmd, found := DetectTestCommand(dir)
	if !found {
		t.Fatal("expected to find cargo test")
	}
	if cmd != "cargo test" {
		t.Errorf("got %q, want %q", cmd, "cargo test")
	}
}

// TestDetectTestCommandPytestIni: dir có pytest.ini → "pytest"
func TestDetectTestCommandPytestIni(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pytest.ini"), []byte("[pytest]\n"), 0644)
	cmd, found := DetectTestCommand(dir)
	if !found {
		t.Fatal("expected to find pytest")
	}
	if cmd != "pytest" {
		t.Errorf("got %q, want %q", cmd, "pytest")
	}
}

// TestDetectTestCommandPyproject: dir có pyproject.toml → "pytest"
func TestDetectTestCommandPyproject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[tool.pytest]\n"), 0644)
	cmd, found := DetectTestCommand(dir)
	if !found {
		t.Fatal("expected to find pytest")
	}
	if cmd != "pytest" {
		t.Errorf("got %q, want %q", cmd, "pytest")
	}
}

// TestRunTestsPassingGoModule: tạo minimal Go module với passing test → passed=true
func TestRunTestsPassingGoModule(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, "pass_test.go"), []byte("package testmod\n\nimport \"testing\"\n\nfunc TestPass(t *testing.T) {}\n"), 0644)

	passed, output, err := RunTests(dir)
	if err != nil {
		t.Fatalf("RunTests error: %v", err)
	}
	if !passed {
		t.Errorf("expected passed=true, output: %s", output)
	}
}

// TestRunTestsFailingGoModule: tạo minimal Go module với failing test → passed=false
func TestRunTestsFailingGoModule(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, "fail_test.go"), []byte("package testmod\n\nimport \"testing\"\n\nfunc TestFail(t *testing.T) { t.Fatal(\"intentional fail\") }\n"), 0644)

	passed, output, err := RunTests(dir)
	if err != nil {
		t.Fatalf("RunTests error: %v", err)
	}
	if passed {
		t.Error("expected passed=false for failing test")
	}
	if output == "" {
		t.Error("expected non-empty output for failing test")
	}
}

func fakeLLMServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": content}},
			},
		})
	}))
}

// TestLLMReviewPass: mock trả "PASS\n..." → verdict="PASS"
func TestLLMReviewPass(t *testing.T) {
	srv := fakeLLMServer(t, "PASS\nLooks correct.")
	defer srv.Close()

	verdict, explanation, err := LLMReview(LLMReviewRequest{
		Task: "add feature", Diff: "+ code", BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("LLMReview error: %v", err)
	}
	if verdict != "PASS" {
		t.Errorf("verdict: got %q, want %q", verdict, "PASS")
	}
	if explanation == "" {
		t.Error("expected non-empty explanation")
	}
}

// TestLLMReviewWarn: mock trả "WARN\n..." → verdict="WARN"
func TestLLMReviewWarn(t *testing.T) {
	srv := fakeLLMServer(t, "WARN\nMinor issues.")
	defer srv.Close()

	verdict, _, err := LLMReview(LLMReviewRequest{
		Task: "task", Diff: "diff", BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("LLMReview error: %v", err)
	}
	if verdict != "WARN" {
		t.Errorf("verdict: got %q, want %q", verdict, "WARN")
	}
}

// TestLLMReviewFail: mock trả "FAIL\n..." → verdict="FAIL"
func TestLLMReviewFail(t *testing.T) {
	srv := fakeLLMServer(t, "FAIL\nBug found.")
	defer srv.Close()

	verdict, _, err := LLMReview(LLMReviewRequest{
		Task: "task", Diff: "diff", BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("LLMReview error: %v", err)
	}
	if verdict != "FAIL" {
		t.Errorf("verdict: got %q, want %q", verdict, "FAIL")
	}
}

// TestLLMReviewUnknownResponse: content không bắt đầu bằng PASS/WARN/FAIL → "WARN"
func TestLLMReviewUnknownResponse(t *testing.T) {
	srv := fakeLLMServer(t, "SOMETHING_ELSE content here")
	defer srv.Close()

	verdict, _, err := LLMReview(LLMReviewRequest{
		Task: "task", Diff: "diff", BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("LLMReview error: %v", err)
	}
	if verdict != "WARN" {
		t.Errorf("expected WARN for unknown response, got %q", verdict)
	}
}

// TestLLMReviewHTTPError: server trả 500 → error != nil
func TestLLMReviewHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, _, err := LLMReview(LLMReviewRequest{
		Task: "task", Diff: "diff", BaseURL: srv.URL,
	})
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

// TestVerifyDoneAutoChecked: dir có passing Go tests + LLM PASS → VerdictAutoChecked
func TestVerifyDoneAutoChecked(t *testing.T) {
	// Tạo minimal passing Go module
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module autocheck\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, "ok_test.go"), []byte("package autocheck\n\nimport \"testing\"\n\nfunc TestOK(t *testing.T) {}\n"), 0644)

	// LLM mock server trả PASS
	llmSrv := fakeLLMServer(t, "PASS\nAll good.")
	defer llmSrv.Close()

	result := VerifyDone(GateConfig{
		TargetDir:  dir,
		Task:       "test feature",
		Diff:       "+ func Feature() {}",
		LLMBaseURL: llmSrv.URL,
	})
	if result.Verdict != VerdictAutoChecked {
		t.Errorf("expected AUTO-CHECKED, got %s (details: %s)", result.Verdict, result.Details)
	}
}

func TestVerifyDoneNoTestSuiteNoLLM(t *testing.T) {
	dir := t.TempDir()
	result := VerifyDone(GateConfig{
		TargetDir: dir,
		Task:      "some task",
	})
	if result.Verdict != VerdictNoTests {
		t.Errorf("expected NO-TESTS, got %s", result.Verdict)
	}
}

func TestVerifyDoneLLMPassReturnsAutoChecked(t *testing.T) {
	// Fake LLM server that returns PASS
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "PASS\nLooks good."}},
			},
		})
	}))
	defer llmServer.Close()

	dir := t.TempDir()
	result := VerifyDone(GateConfig{
		TargetDir:  dir,
		Task:       "add login",
		Diff:       "+ func Login() {}",
		LLMBaseURL: llmServer.URL,
	})
	if result.Verdict != VerdictNoTests {
		// No test suite → VerdictNoTests even when LLM passes
		t.Errorf("expected NO-TESTS (no test suite), got %s", result.Verdict)
	}
}

func TestVerifyDoneLLMFailReturnsFailed(t *testing.T) {
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "FAIL\nSQL injection vulnerability."}},
			},
		})
	}))
	defer llmServer.Close()

	dir := t.TempDir()
	result := VerifyDone(GateConfig{
		TargetDir:  dir,
		Task:       "add query",
		Diff:       "+ db.Query(userInput)",
		LLMBaseURL: llmServer.URL,
	})
	if result.Verdict != VerdictFailed {
		t.Errorf("expected FAILED, got %s", result.Verdict)
	}
	if result.LLMOutput == "" {
		t.Error("expected LLMOutput to explain the failure")
	}
}

func TestVerifyDoneLLMWarnReturnsNeedsReview(t *testing.T) {
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "WARN\nMissing error handling."}},
			},
		})
	}))
	defer llmServer.Close()

	dir := t.TempDir()
	result := VerifyDone(GateConfig{
		TargetDir:  dir,
		Task:       "add handler",
		Diff:       "+ func Handle() {}",
		LLMBaseURL: llmServer.URL,
	})
	if result.Verdict != VerdictNeedsReview {
		t.Errorf("expected NEEDS-REVIEW, got %s", result.Verdict)
	}
}
