package main

import (
	"fmt"
	"os"

	"github.com/nicodolas/kashy/internal/kashyconfig"
	"github.com/spf13/cobra"
)

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
