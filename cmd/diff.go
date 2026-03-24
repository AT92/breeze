package cmd

import (
	"fmt"
	"os"

	"breeze/internal/bundle"
	"breeze/internal/diff"

	"github.com/spf13/cobra"
)

const (
	exitCodeDifferences = 0
	exitCodeError       = 1
	exitCodeIdentical   = 2
)

var diffFlags struct {
	output       string
	outputFormat string
}

var diffCmd = &cobra.Command{
	Use:   "diff <old-bundle.tar.gz> <new-bundle.tar.gz>",
	Short: "Compare two breeze bundles and optionally generate a delta bundle",
	Long: `Compare two bundle archives by reading their manifests and classifying
each image as added, removed, changed, or unchanged.

Optionally generate a delta bundle containing only the changed/added artifacts.

Exit codes:
  0 — bundles differ
  1 — error
  2 — bundles are identical`,
	Args: cobra.ExactArgs(2),
	RunE: runDiff,
}

func init() {
	diffCmd.Flags().StringVarP(&diffFlags.output, "output", "o", "", "generate a delta bundle at this path")
	diffCmd.Flags().StringVar(&diffFlags.outputFormat, "output-format", "text", "report format: text, json, or yaml")

	rootCmd.AddCommand(diffCmd)
}

func runDiff(cmd *cobra.Command, args []string) error {
	oldBundlePath := args[0]
	newBundlePath := args[1]

	oldManifest, err := bundle.ReadManifest(oldBundlePath)
	if err != nil {
		return fmt.Errorf("reading old bundle: %w", err)
	}

	newManifest, err := bundle.ReadManifest(newBundlePath)
	if err != nil {
		return fmt.Errorf("reading new bundle: %w", err)
	}

	diffResult := diff.CompareManifests(oldManifest, newManifest)

	report, err := formatDiffReport(diffResult, diffFlags.outputFormat)
	if err != nil {
		return err
	}
	fmt.Print(report)

	if diffFlags.output != "" && !diffResult.IsIdentical() {
		if err := diff.GenerateDeltaBundle(diffFlags.output, oldBundlePath, newBundlePath, diffResult, newManifest); err != nil {
			return fmt.Errorf("generating delta bundle: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Delta bundle written to %s\n", diffFlags.output)
	}

	if diffResult.IsIdentical() {
		os.Exit(exitCodeIdentical)
	}

	return nil
}

func formatDiffReport(diffResult *diff.DiffResult, outputFormat string) (string, error) {
	switch outputFormat {
	case "text":
		return diff.FormatText(diffResult), nil
	case "json":
		return diff.FormatJSON(diffResult)
	case "yaml":
		return diff.FormatYAML(diffResult)
	default:
		return "", fmt.Errorf("unknown output format %q (supported: text, json, yaml)", outputFormat)
	}
}
