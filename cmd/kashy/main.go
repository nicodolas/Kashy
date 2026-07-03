package main

import (
	"fmt"
	"os"

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
		cmdUpdate(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
