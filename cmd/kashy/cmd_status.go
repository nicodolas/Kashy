package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/nicodolas/kashy/internal/kashyconfig"
	"github.com/nicodolas/kashy/internal/session"
	"github.com/nicodolas/kashy/internal/version"
	"github.com/spf13/cobra"
)

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
