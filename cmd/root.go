package cmd

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

var globalFlags struct {
	quiet   bool
	verbose bool
}

var rootCmd = &cobra.Command{
	Use:   "breeze",
	Short: "Bundle Helm charts for air-gapped environments",
	Long:  "Breeze downloads Helm charts with all dependencies and container images, bundling them into a portable tar.gz archive.",
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&globalFlags.quiet, "quiet", "q", false, "suppress progress output")
	rootCmd.PersistentFlags().BoolVarP(&globalFlags.verbose, "verbose", "v", false, "show detailed debug output")
}

func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return rootCmd.ExecuteContext(ctx)
}
