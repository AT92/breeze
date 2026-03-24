package cmd

import (
	"breeze/internal/bundler"
	"breeze/internal/log"

	"github.com/spf13/cobra"
)

var bundleFlags struct {
	version     string
	values      []string
	set         []string
	output      string
	platform    string
	repo        string
	skipImages  bool
	dryRun      bool
	concurrency int
}

var bundleCmd = &cobra.Command{
	Use:   "bundle <chart-ref>",
	Short: "Bundle a Helm chart for air-gapped installation",
	Long: `Download a Helm chart with all dependencies and container images,
packaging everything into a compressed tar archive.

The chart-ref can be:
  - A local directory:  breeze bundle ./my-chart/
  - A local .tgz file:  breeze bundle my-chart-1.0.0.tgz
  - A remote chart:     breeze bundle nginx --repo https://charts.bitnami.com/bitnami`,
	Args: cobra.ExactArgs(1),
	RunE: runBundle,
}

func init() {
	f := bundleCmd.Flags()
	f.StringVar(&bundleFlags.version, "version", "", "chart version (for remote charts)")
	f.StringSliceVarP(&bundleFlags.values, "values", "f", nil, "values file(s) for template rendering")
	f.StringSliceVar(&bundleFlags.set, "set", nil, "set values on the command line (key=val)")
	f.StringVarP(&bundleFlags.output, "output", "o", "", "output file path (default: <chart>-<version>-bundle.tar.gz)")
	f.StringVar(&bundleFlags.platform, "platform", "linux/amd64", "target platform for container images")
	f.StringVar(&bundleFlags.repo, "repo", "", "Helm repository URL")
	f.BoolVar(&bundleFlags.skipImages, "skip-images", false, "skip downloading container images")
	f.BoolVar(&bundleFlags.dryRun, "dry-run", false, "list images without downloading or creating a bundle")
	f.IntVar(&bundleFlags.concurrency, "concurrency", 4, "number of parallel image downloads")

	rootCmd.AddCommand(bundleCmd)
}

func runBundle(cmd *cobra.Command, args []string) error {
	logLevel := log.LevelNormal
	if globalFlags.quiet {
		logLevel = log.LevelQuiet
	} else if globalFlags.verbose {
		logLevel = log.LevelVerbose
	}

	return bundler.New(logLevel).Run(cmd.Context(), bundler.Options{
		ChartRef:    args[0],
		Version:     bundleFlags.version,
		RepoURL:     bundleFlags.repo,
		Values:      bundleFlags.values,
		Set:         bundleFlags.set,
		Output:      bundleFlags.output,
		Platform:    bundleFlags.platform,
		SkipImages:  bundleFlags.skipImages,
		DryRun:      bundleFlags.dryRun,
		Concurrency: bundleFlags.concurrency,
	})
}
