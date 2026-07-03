package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/nicodolas/kashy/internal/doctor"
	"github.com/nicodolas/kashy/internal/httpserver"
	"github.com/nicodolas/kashy/internal/kashyconfig"
	"github.com/nicodolas/kashy/internal/mcpserver"
	"github.com/nicodolas/kashy/internal/openrouter"
	"github.com/nicodolas/kashy/internal/provider"
	"github.com/nicodolas/kashy/internal/proxy"
	"github.com/nicodolas/kashy/internal/session"
	"github.com/nicodolas/kashy/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "kashy",
		Short:   "Kashy — watches your AI spending so you don't have to",
		Version: version.Full(),
	}

	rootCmd.AddCommand(
		cmdStart(),
		cmdStop(),
		cmdStatus(),
		cmdHistory(),
		cmdBalance(),
		cmdConfig(),
		cmdDoctor(),
		cmdMCP(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// cmdStart starts the Kashy spending proxy and status server.
func cmdStart() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start Kashy (spending proxy :4000 + status server :4001)",
		Long: `Start the Kashy spending layer.

Any AI agent pointing to http://localhost:4000/v1 will have its
costs tracked in real time with configurable budget limits.`,
		Run: func(cmd *cobra.Command, args []string) {
			// Auto-migrate API key from ~/.nico/config.toml if ~/.kashy not set up
			migrateFromNico()

			cfg := kashyconfig.Load()
			apiKey := cfg.Providers.OpenRouter.APIKey
			if apiKey == "" {
				fmt.Fprintln(os.Stderr, "[kashy] error: API key not set")
				fmt.Fprintln(os.Stderr, "  Run: kashy config set-key sk-or-...")
				os.Exit(1)
			}

			store := session.Default()
			prov := provider.OpenRouter(apiKey)
			pricing := &provider.PricingCache{}
			go func() { _ = pricing.FetchPricing(prov) }()

			costProxy := proxy.New(proxy.Config{
				Provider:        prov,
				Store:           store,
				Pricing:         pricing,
				SessionHardStop: cfg.Budget.SessionHardStop,
				WarnAt:          cfg.Budget.WarnAt,
				DailyLimit:      cfg.Budget.DailyLimit,
			})
			costProxy.SetUsageCallback(func(e proxy.UsageEvent) {
				fmt.Printf("[kashy] %s — prompt:%d comp:%d → $%.6f\n",
					e.Model, e.PromptTok, e.CompTok, e.CostUSD)
			})

			statusSrv := httpserver.New(store)

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			proxyErr := make(chan error, 1)
			statusErr := make(chan error, 1)

			go func() {
				fmt.Println("[kashy] proxy started — point your agent to http://localhost:4000/v1")
				proxyErr <- http.ListenAndServe(":4000", costProxy.Handler())
			}()
			go func() {
				fmt.Println("[kashy] status server started — http://localhost:4001/status")
				statusErr <- http.ListenAndServe(":4001", statusSrv.Handler())
			}()

			fmt.Printf("[kashy] Kashy %s is watching. Budget: $%.2f/session | Press Ctrl+C to stop.\n",
				version.String(), cfg.Budget.SessionHardStop)

			select {
			case err := <-proxyErr:
				fmt.Fprintf(os.Stderr, "[kashy] proxy error: %v\n", err)
			case err := <-statusErr:
				fmt.Fprintf(os.Stderr, "[kashy] status server error: %v\n", err)
			case <-ctx.Done():
				fmt.Println("\n[kashy] stopped.")
			}
		},
	}
}

// cmdStop prints stop instructions.
func cmdStop() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop Kashy daemon",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("[kashy] Send Ctrl+C to the running 'kashy start' process to stop.")
		},
	}
}

// cmdStatus shows current session spending.
func cmdStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current session spending",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := kashyconfig.Load()
			store := session.Default()
			st := store.ReadState()

			if st.SessionID == "" {
				fmt.Println("[kashy] No active session. Run 'kashy start' first.")
				return
			}

			fmt.Printf("Kashy %s — Session Spending\n", version.String())
			fmt.Println("─────────────────────────────────────")
			fmt.Printf("Session:       %s\n", st.SessionID)
			fmt.Printf("Cost (USD):    $%.6f\n", st.TotalCostUSD)
			fmt.Printf("Prompt tokens: %d\n", st.PromptTokens)
			fmt.Printf("Comp tokens:   %d\n", st.CompTokens)
			fmt.Printf("LLM calls:     %d\n", st.CallCount)
			fmt.Printf("Last model:    %s\n", st.LastModel)

			pct := 0.0
			if cfg.Budget.SessionHardStop > 0 {
				pct = st.TotalCostUSD / cfg.Budget.SessionHardStop * 100
			}
			bar := spendingBar(pct, 20)
			fmt.Printf("\nBudget:        [%s] %.1f%% of $%.2f\n", bar, pct, cfg.Budget.SessionHardStop)
		},
	}
}

// cmdHistory shows spending by day for the last 30 days.
func cmdHistory() *cobra.Command {
	return &cobra.Command{
		Use:   "history",
		Short: "Show AI spending history (last 30 days)",
		Run: func(cmd *cobra.Command, args []string) {
			store := session.Default()
			entries, err := store.ReadHistory(500)
			if err != nil || len(entries) == 0 {
				fmt.Println("[kashy] No spending history yet.")
				return
			}

			// Group by date
			type daySummary struct {
				calls  int
				tokens int
				cost   float64
			}
			byDay := make(map[string]*daySummary)
			var days []string

			cutoff := time.Now().AddDate(0, 0, -30)
			for _, e := range entries {
				if e.Timestamp.Before(cutoff) {
					continue
				}
				day := e.Timestamp.Format("2006-01-02")
				if _, ok := byDay[day]; !ok {
					byDay[day] = &daySummary{}
					days = append(days, day)
				}
				byDay[day].calls++
				byDay[day].tokens += e.PromptTok + e.CompTok
				byDay[day].cost += e.CostUSD
			}

			if len(days) == 0 {
				fmt.Println("[kashy] No spending in the last 30 days.")
				return
			}

			fmt.Printf("%-12s  %6s  %10s  %10s\n", "Date", "Calls", "Tokens", "Cost (USD)")
			fmt.Println(strings.Repeat("─", 46))
			total := 0.0
			for _, day := range days {
				d := byDay[day]
				fmt.Printf("%-12s  %6d  %10d  $%9.6f\n", day, d.calls, d.tokens, d.cost)
				total += d.cost
			}
			fmt.Println(strings.Repeat("─", 46))
			fmt.Printf("%-12s  %6s  %10s  $%9.6f\n", "Total", "", "", total)
		},
	}
}

// cmdBalance shows live OpenRouter spending from their API directly.
func cmdBalance() *cobra.Command {
	return &cobra.Command{
		Use:   "balance",
		Short: "Show live OpenRouter spending (today, week, month)",
		Long:  "Queries OpenRouter API directly for real-time spending data. No proxy needed.",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := kashyconfig.Load()
			apiKey := cfg.Providers.OpenRouter.APIKey
			if apiKey == "" {
				fmt.Fprintln(os.Stderr, "[kashy] error: API key not set. Run: kashy config set-key sk-or-...")
				os.Exit(1)
			}
			client := openrouter.New(apiKey)
			keyInfo, err := client.GetKeyInfo()
			if err != nil {
				fmt.Fprintf(os.Stderr, "[kashy] error fetching balance: %v\n", err)
				os.Exit(1)
			}
			fmt.Print(keyInfo.Summary())
		},
	}
}

// cmdConfig manages Kashy configuration.
func cmdConfig() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or update Kashy configuration",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := kashyconfig.Load()
			fmt.Print(kashyconfig.Show(cfg))
		},
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "set-key <api-key>",
			Short: "Set the OpenRouter API key",
			Args:  cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				if err := kashyconfig.SetAPIKey(args[0]); err != nil {
					fmt.Fprintf(os.Stderr, "[kashy] error: %v\n", err)
					os.Exit(1)
				}
				masked := args[0]
				if len(masked) > 12 {
					masked = masked[:8] + "..." + masked[len(masked)-4:]
				}
				fmt.Printf("[kashy] ✅ API key saved: %s\n", masked)
				fmt.Printf("[kashy] Config: %s\n", kashyconfig.ConfigFilePath())
			},
		},
		&cobra.Command{
			Use:   "show",
			Short: "Show current configuration",
			Run: func(cmd *cobra.Command, args []string) {
				fmt.Print(kashyconfig.Show(kashyconfig.Load()))
			},
		},
		&cobra.Command{
			Use:   "set-budget <usd>",
			Short: "Set session hard-stop budget in USD",
			Args:  cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				var amount float64
				if _, err := fmt.Sscanf(args[0], "%f", &amount); err != nil || amount <= 0 {
					fmt.Fprintln(os.Stderr, "[kashy] error: amount must be a positive number (e.g. 0.50)")
					os.Exit(1)
				}
				cfg := kashyconfig.Load()
				cfg.Budget.SessionHardStop = amount
				if err := kashyconfig.Save(cfg); err != nil {
					fmt.Fprintf(os.Stderr, "[kashy] error: %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("[kashy] ✅ Session budget set to $%.2f\n", amount)
			},
		},
	)
	return cmd
}

// cmdDoctor checks agent connections and optionally fixes them.
func cmdDoctor() *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check which agents are connected to Kashy proxy",
		Run: func(cmd *cobra.Command, args []string) {
			if fix {
				applyFixes()
			}
			statuses := doctor.Check()
			fmt.Print(doctor.Report(statuses))
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "Auto-patch agent config files to connect to Kashy")
	return cmd
}

// applyFixes patches known agent config files to use Kashy proxy.
func applyFixes() {
	home, _ := os.UserHomeDir()
	proxyURL := "http://localhost:4000/v1"

	// Fix OMX — .codex/config.toml
	omxConfig := filepath.Join(home, ".codex", "config.toml")
	if data, err := os.ReadFile(omxConfig); err == nil {
		content := string(data)
		if !strings.Contains(content, "localhost:4000") {
			if strings.Contains(content, "openai_base_url") {
				// Replace existing
				lines := strings.Split(content, "\n")
				for i, line := range lines {
					if strings.HasPrefix(strings.TrimSpace(line), "openai_base_url") {
						lines[i] = fmt.Sprintf(`openai_base_url = "%s"`, proxyURL)
					}
				}
				content = strings.Join(lines, "\n")
			} else {
				content += fmt.Sprintf("\nopenai_base_url = \"%s\"\n", proxyURL)
			}
			os.WriteFile(omxConfig, []byte(content), 0644)
			fmt.Printf("[kashy] ✅ Patched OMX config: %s\n", omxConfig)
		}
	}

	// Fix Kiro MCP — add kashy to mcp.json
	kiroMCP := filepath.Join(home, ".kiro", "settings", "mcp.json")
	if _, err := os.Stat(kiroMCP); err == nil {
		fmt.Printf("[kashy] ℹ️  Kiro MCP: already managed. Run 'kashy mcp' for MCP server config.\n")
	}

	fmt.Println("[kashy] Run 'kashy doctor' to verify connections.")
}

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

// spendingBar renders an ASCII bar for budget visualization.
func spendingBar(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	bar := ""
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	return bar
}


// migrateFromNico copies API key from ~/.nico/config.toml to ~/.kashy/config.toml
// if the kashy config doesn't have a key yet. One-time migration for nico→kashy users.
func migrateFromNico() {
	home, _ := os.UserHomeDir()
	kashyCfgPath := filepath.Join(home, ".kashy", "config.toml")
	nicoCfgPath := filepath.Join(home, ".nico", "config.toml")

	// If kashy already has a key, skip
	current := kashyconfig.Load()
	if current.Providers.OpenRouter.APIKey != "" {
		return
	}

	// Check if nico config exists
	if _, err := os.Stat(nicoCfgPath); err != nil {
		return
	}

	// Copy nico config to kashy
	if err := os.MkdirAll(filepath.Dir(kashyCfgPath), 0755); err != nil {
		return
	}
	data, err := os.ReadFile(nicoCfgPath)
	if err != nil {
		return
	}
	if err := os.WriteFile(kashyCfgPath, data, 0644); err != nil {
		return
	}
	fmt.Println("[kashy] ℹ️  Migrated config from ~/.nico/ — you're all set.")
}

