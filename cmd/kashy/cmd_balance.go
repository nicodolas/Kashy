package main

import (
	"fmt"
	"os"

	"github.com/nicodolas/kashy/internal/kashyconfig"
	"github.com/nicodolas/kashy/internal/openrouter"
	"github.com/spf13/cobra"
)

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
