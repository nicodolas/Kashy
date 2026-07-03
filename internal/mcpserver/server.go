// Package mcpserver exposes Kashy tools via the Model Context Protocol.
// Agents (Claude Code, OpenCode, OMX) can call these tools directly.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/nicodolas/kashy/internal/openrouter"
	"github.com/nicodolas/kashy/internal/session"
	"github.com/nicodolas/kashy/internal/verify"
)

// Serve starts the MCP server in stdio mode (standard for MCP tool servers).
// Call from `kashy mcp` — blocks until stdin closes.
func Serve(store *session.Store) error {
	return ServeWithKey(store, "")
}

// ServeWithKey starts the MCP server with an optional OpenRouter API key
// for direct account queries (kashy_budget_remaining, kashy_account_usage).
func ServeWithKey(store *session.Store, apiKey string) error {
	s := server.NewMCPServer(
		"Kashy",
		"0.1.0",
		server.WithToolCapabilities(false),
	)

	// ─── kashy_cost_status ─────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("kashy_cost_status",
			mcp.WithDescription("Get current session cost, token usage, and budget status."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			st := store.ReadState()
			data, err := json.MarshalIndent(st, "", "  ")
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("marshal error: %v", err)), nil
			}
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// ─── kashy_cost_history ────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("kashy_cost_history",
			mcp.WithDescription("Get the last N session history entries (model calls with cost)."),
			mcp.WithNumber("limit",
				mcp.Description("Number of entries to return (default 10, max 50)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			limit := int(req.GetFloat("limit", 10))
			if limit <= 0 {
				limit = 10
			}
			if limit > 50 {
				limit = 50
			}
			entries, err := store.ReadHistory(limit)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("read history: %v", err)), nil
			}
			data, _ := json.MarshalIndent(entries, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// ─── kashy_verify_done ─────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("kashy_verify_done",
			mcp.WithDescription("Verify a completed task: runs tests + LLM review. Returns AUTO-CHECKED, NEEDS-REVIEW, FAILED, or NO-TESTS."),
			mcp.WithString("task",
				mcp.Required(),
				mcp.Description("Description of the task that was completed"),
			),
			mcp.WithString("target_dir",
				mcp.Description("Project directory to run tests in (defaults to cwd)"),
			),
			mcp.WithString("diff",
				mcp.Description("Git diff or code changes for LLM review (optional)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			task := req.GetString("task", "")
			if task == "" {
				return mcp.NewToolResultError("task is required"), nil
			}
			targetDir := req.GetString("target_dir", "")
			if targetDir == "" {
				var err error
				targetDir, err = os.Getwd()
				if err != nil {
					targetDir = "."
				}
			}
			diff := req.GetString("diff", "")

			result := verify.VerifyDone(verify.GateConfig{
				TargetDir: targetDir,
				Task:      task,
				Diff:      diff,
			})

			out := fmt.Sprintf("Verdict: %s\n\nDetails: %s", result.Verdict, result.Details)
			if result.TestOutput != "" {
				out += "\n\nTest Output:\n" + result.TestOutput
			}
			if result.LLMOutput != "" {
				out += "\n\nLLM Review:\n" + result.LLMOutput
			}
			return mcp.NewToolResultText(out), nil
		},
	)

	// ─── kashy_reset_budget ────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("kashy_reset_budget",
			mcp.WithDescription("Reset the session cost accumulator to $0.00."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if err := store.ClearState(); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("reset error: %v", err)), nil
			}
			return mcp.NewToolResultText("Session budget reset to $0.00."), nil
		},
	)

	// ─── kashy_budget_remaining ───────────────────────────────────────────
	// Queries OpenRouter API directly for real account credit balance.
	if apiKey != "" {
		orClient := openrouter.New(apiKey)

		s.AddTool(
			mcp.NewTool("kashy_budget_remaining",
				mcp.WithDescription("Check your OpenRouter spending: usage today, this week, this month, and key limits. Uses live OpenRouter API data."),
			),
			func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				keyInfo, err := orClient.GetKeyInfo()
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("OpenRouter API error: %v", err)), nil
				}
				return mcp.NewToolResultText(keyInfo.Summary()), nil
			},
		)

		s.AddTool(
			mcp.NewTool("kashy_account_usage",
				mcp.WithDescription("Get detailed OpenRouter account usage stats including rate limits and key info."),
			),
			func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				keyInfo, err := orClient.GetKeyInfo()
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("OpenRouter API error: %v", err)), nil
				}
				return mcp.NewToolResultText(keyInfo.Summary()), nil
			},
		)
	}

	return server.ServeStdio(s)
}

