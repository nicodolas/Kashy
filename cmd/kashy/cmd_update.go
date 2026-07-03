package main

import (
	"fmt"
	"os"

	"github.com/nicodolas/kashy/internal/updater"
	"github.com/nicodolas/kashy/internal/version"
	"github.com/spf13/cobra"
)

// cmdUpdate checks for and applies the latest Kashy release.
func cmdUpdate() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update Kashy to the latest version",
		Long: `Check GitHub for a newer Kashy release and replace the current binary.

The update downloads the correct binary for your OS/arch, extracts it,
and replaces the running executable in-place.

If the update fails, the current version is left unchanged.`,
		Run: func(cmd *cobra.Command, args []string) {
			current := version.String()
			fmt.Printf("[kashy] current version: %s\n", current)
			fmt.Print("[kashy] checking for updates... ")

			result, err := updater.CheckLatest(current)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\n[kashy] error checking for updates: %v\n", err)
				fmt.Fprintln(os.Stderr, "[kashy] check your internet connection or visit github.com/nicodolas/kashy/releases")
				os.Exit(1)
			}

			if !result.Available {
				fmt.Printf("already up to date (%s)\n", current)
				return
			}

			fmt.Printf("update available: %s\n", result.Version)

			exe, err := os.Executable()
			if err != nil {
				fmt.Fprintf(os.Stderr, "[kashy] error finding current executable: %v\n", err)
				os.Exit(1)
			}

			if err := updater.SelfUpdate(result, exe); err != nil {
				fmt.Fprintf(os.Stderr, "[kashy] update failed: %v\n", err)
				fmt.Fprintln(os.Stderr, "[kashy] you can update manually at: github.com/nicodolas/kashy/releases")
				os.Exit(1)
			}

			fmt.Printf("[kashy] ✅ updated to %s — restart kashy to use the new version\n", result.Version)
		},
	}
}
