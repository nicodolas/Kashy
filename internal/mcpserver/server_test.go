package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nicodolas/kashy/internal/session"
)

// callTool is a helper that invokes a registered tool by name with given arguments.
func callTool(t *testing.T, store *session.Store, apiKey string, toolName string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	s := buildServer(store, apiKey)
	st := s.GetTool(toolName)
	if st == nil {
		t.Fatalf("tool %q not registered", toolName)
	}
	req := mcp.CallToolRequest{}
	req.Params.Name = toolName
	req.Params.Arguments = args
	result, err := st.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("tool %q handler error: %v", toolName, err)
	}
	return result
}

// resultText extracts the text content from a tool result.
func resultText(result *mcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func TestBuildServerRegistersBasicTools(t *testing.T) {
	store := session.New(t.TempDir())
	s := buildServer(store, "")

	required := []string{
		"kashy_cost_status",
		"kashy_cost_history",
		"kashy_verify_done",
		"kashy_reset_budget",
	}
	for _, name := range required {
		if s.GetTool(name) == nil {
			t.Errorf("expected tool %q to be registered", name)
		}
	}
}

func TestBuildServerWithKeyRegistersExtraTools(t *testing.T) {
	store := session.New(t.TempDir())
	s := buildServer(store, "sk-test-key")

	extra := []string{"kashy_budget_remaining", "kashy_account_usage"}
	for _, name := range extra {
		if s.GetTool(name) == nil {
			t.Errorf("expected tool %q to be registered when apiKey is set", name)
		}
	}
}

func TestBuildServerWithoutKeyOmitsExtraTools(t *testing.T) {
	store := session.New(t.TempDir())
	s := buildServer(store, "")

	omitted := []string{"kashy_budget_remaining", "kashy_account_usage"}
	for _, name := range omitted {
		if s.GetTool(name) != nil {
			t.Errorf("tool %q should NOT be registered without apiKey", name)
		}
	}
}

func TestCostStatusToolReturnsJSON(t *testing.T) {
	store := session.New(t.TempDir())
	result := callTool(t, store, "", "kashy_cost_status", nil)

	text := resultText(result)
	if text == "" {
		t.Fatal("kashy_cost_status returned empty text")
	}
	// Must be valid JSON
	if !strings.Contains(text, "{") {
		t.Errorf("expected JSON response, got: %s", text)
	}
}

func TestCostHistoryToolEmptyStore(t *testing.T) {
	store := session.New(t.TempDir())
	result := callTool(t, store, "", "kashy_cost_history", map[string]any{"limit": 10.0})

	text := resultText(result)
	// Empty store → should return "null" or "[]"
	if text == "" {
		t.Fatal("kashy_cost_history returned empty text")
	}
}

func TestCostHistoryToolWithData(t *testing.T) {
	store := session.New(t.TempDir())
	store.UpdateCost("claude-haiku", 100, 50, 0.001)
	store.UpdateCost("claude-haiku", 200, 100, 0.002)

	result := callTool(t, store, "", "kashy_cost_history", map[string]any{"limit": 5.0})
	text := resultText(result)

	if !strings.Contains(text, "claude-haiku") {
		t.Errorf("expected history to contain model name, got: %s", text)
	}
}

func TestResetBudgetToolClearsState(t *testing.T) {
	store := session.New(t.TempDir())
	store.UpdateCost("model", 100, 50, 0.5)

	result := callTool(t, store, "", "kashy_reset_budget", nil)
	text := resultText(result)

	if !strings.Contains(text, "$0.00") {
		t.Errorf("expected reset confirmation, got: %s", text)
	}
	st := store.ReadState()
	if st.TotalCostUSD != 0 {
		t.Errorf("expected 0 cost after reset, got %f", st.TotalCostUSD)
	}
}

func TestVerifyDoneToolMissingTask(t *testing.T) {
	store := session.New(t.TempDir())
	s := buildServer(store, "")
	st := s.GetTool("kashy_verify_done")
	if st == nil {
		t.Fatal("kashy_verify_done not registered")
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = "kashy_verify_done"
	req.Params.Arguments = map[string]any{} // no "task"

	result, err := st.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return an error result, not panic
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestVerifyDoneToolNoTestSuite(t *testing.T) {
	store := session.New(t.TempDir())
	result := callTool(t, store, "", "kashy_verify_done", map[string]any{
		"task":       "test task",
		"target_dir": t.TempDir(), // empty dir — no test suite
	})

	text := resultText(result)
	if !strings.Contains(text, "Verdict:") {
		t.Errorf("expected Verdict in output, got: %s", text)
	}
}
