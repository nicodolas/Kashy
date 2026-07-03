package main

import (
	"fmt"
	"os"

	"github.com/nicodolas/kashy/internal/kashyconfig"
	"github.com/nicodolas/kashy/internal/mcpserver"
	"github.com/nicodolas/kashy/internal/session"
	"github.com/spf13/cobra"
)

// cmdMCP starts the MCP stdio server.
func cmdMCP() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start Kashy as an MCP stdio server for agent integration",
		Long: `Start Kashy as an MCP tool server.

Add to your agent config (e.g. Kiro settings.json):
  {
    "mcpServers": {
      "kashy": {
        "command": "kashy",
        "args": ["mcp"]
      }
    }
  }

Tools: kashy_cost_status, kashy_cost_history, kashy_verify_done, kashy_reset_budget,
       kashy_budget_remaining (live OpenRouter credits), kashy_account_usage`,
		Run: func(cmd *cobra.Command, args []string) {
			store := session.Default()
			cfg := kashyconfig.Load()
			if err := mcpserver.ServeWithKey(store, cfg.Providers.OpenRouter.APIKey); err != nil {
				fmt.Fprintf(os.Stderr, "[kashy] mcp error: %v\n", err)
				os.Exit(1)
			}
		},
	}
}
