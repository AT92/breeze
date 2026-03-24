package cmd

import (
	"fmt"

	"breeze/internal/bundle"

	"github.com/spf13/cobra"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect <bundle.tar.gz>",
	Short: "Display the manifest of a breeze bundle",
	Args:  cobra.ExactArgs(1),
	RunE:  runInspect,
}

func init() {
	rootCmd.AddCommand(inspectCmd)
}

func runInspect(cmd *cobra.Command, args []string) error {
	manifest, err := bundle.ReadManifest(args[0])
	if err != nil {
		return fmt.Errorf("reading bundle: %w", err)
	}

	data, err := manifest.Marshal()
	if err != nil {
		return fmt.Errorf("formatting manifest: %w", err)
	}

	fmt.Print(string(data))
	return nil
}
