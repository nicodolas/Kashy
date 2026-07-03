package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/nicodolas/kashy/internal/daemon"
	"github.com/nicodolas/kashy/internal/httpserver"
	"github.com/nicodolas/kashy/internal/kashyconfig"
	"github.com/nicodolas/kashy/internal/provider"
	"github.com/nicodolas/kashy/internal/proxy"
	"github.com/nicodolas/kashy/internal/session"
	"github.com/nicodolas/kashy/internal/updater"
	"github.com/nicodolas/kashy/internal/version"
	"github.com/spf13/cobra"
)

// cmdStart starts the Kashy spending proxy and status server.
func cmdStart() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start Kashy (spending proxy :4000 + status server :4001)",
		Long: `Start the Kashy spending layer.

Any AI agent pointing to http://localhost:4000/v1 will have its
costs tracked in real time with configurable budget limits.`,
		Run: func(cmd *cobra.Command, args []string) {
			migrateFromNico()

			// Check for updates before starting — timeout 3s, never blocks startup
			if result, err := updater.CheckLatest(version.String()); err == nil && result.Available {
				fmt.Printf("[kashy] ✨ v%s → %s available! Run: kashy update\n",
					version.String(), result.Version)
			}

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

			if err := daemon.WriteSelf(); err != nil {
				fmt.Fprintf(os.Stderr, "[kashy] warning: could not write pidfile: %v\n", err)
			}
			defer daemon.RemoveSelf()

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

// cmdStop sends a stop signal to the running kashy daemon via pidfile.
func cmdStop() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the running Kashy daemon",
		Run: func(cmd *cobra.Command, args []string) {
			if err := daemon.StopDaemon(); err != nil {
				fmt.Fprintf(os.Stderr, "[kashy] %v\n", err)
				os.Exit(1)
			}
			fmt.Println("[kashy] stopped.")
		},
	}
}
