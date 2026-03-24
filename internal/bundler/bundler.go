package bundler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"breeze/internal/bundle"
	"breeze/internal/chart"
	"breeze/internal/image"
	"breeze/internal/log"

	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/strvals"
)

// Options configures a bundle operation.
type Options struct {
	ChartRef    string
	Version     string
	RepoURL     string
	Values      []string
	Set         []string
	Output      string
	Platform    string
	SkipImages  bool
	DryRun      bool
	Concurrency int
}

// Bundler orchestrates the chart-fetch, image-extract, image-pull, and archive pipeline.
type Bundler struct {
	Fetcher        chart.Fetcher
	ImageExtractor chart.ImageExtractor
	ImagePuller    image.Puller
	BundleCreator  bundle.Creator
	Log            *log.Logger
}

// New creates a Bundler with default implementations at the given log level.
func New(logLevel log.Level) *Bundler {
	return &Bundler{
		Fetcher:        &chart.DefaultFetcher{},
		ImageExtractor: &chart.DefaultImageExtractor{},
		ImagePuller:    &image.DefaultPuller{},
		BundleCreator:  &bundle.DefaultCreator{},
		Log:            log.New(logLevel),
	}
}

// Run executes the full bundle pipeline. If the context is cancelled (e.g. Ctrl+C),
// the operation is aborted and all artifacts including partial output files are cleaned up.
func (bundler *Bundler) Run(ctx context.Context, options Options) error {
	tempDir, err := os.MkdirTemp("", "breeze-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	fetchResult, err := bundler.fetchChart(options, tempDir)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("cancelled: %w", err)
	}

	overrides, err := mergeValues(options.Values, options.Set)
	if err != nil {
		return fmt.Errorf("parsing values: %w", err)
	}

	outputPath := resolveOutputPath(options.Output, fetchResult.Chart.Name(), fetchResult.Chart.Metadata.Version)
	manifest := buildManifest(fetchResult)

	imageTarballs, imageInfos, err := bundler.downloadImages(ctx, options, fetchResult, overrides, tempDir)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("cancelled: %w", err)
	}
	manifest.Images = imageInfos

	err = bundler.createBundle(outputPath, manifest, fetchResult.ArchivePath, imageTarballs)
	if err != nil {
		removePartialOutput(outputPath)
		return err
	}
	if err := ctx.Err(); err != nil {
		removePartialOutput(outputPath)
		return fmt.Errorf("cancelled: %w", err)
	}

	return nil
}

func removePartialOutput(outputPath string) {
	if outputPath != "" {
		os.Remove(outputPath)
	}
}

func (bundler *Bundler) fetchChart(options Options, tempDir string) (*chart.FetchResult, error) {
	bundler.Log.Info("Fetching chart...")
	fetchResult, err := bundler.Fetcher.Fetch(options.ChartRef, options.Version, options.RepoURL, tempDir)
	if err != nil {
		return nil, fmt.Errorf("fetching chart: %w", err)
	}
	bundler.Log.Info("Fetched %s (version %s)", fetchResult.Chart.Name(), fetchResult.Chart.Metadata.Version)
	bundler.Log.Debug("Chart has %d dependencies", len(fetchResult.Chart.Metadata.Dependencies))
	return fetchResult, nil
}

func (bundler *Bundler) downloadImages(ctx context.Context, options Options, fetchResult *chart.FetchResult, overrides map[string]interface{}, tempDir string) (map[string]string, []bundle.ImageInfo, error) {
	imageTarballs := make(map[string]string)
	var imageInfos []bundle.ImageInfo

	if options.SkipImages {
		return imageTarballs, imageInfos, nil
	}

	bundler.Log.Info("Extracting image references...")
	imageReferences, err := bundler.ImageExtractor.Extract(fetchResult.Chart, overrides)
	if err != nil {
		return nil, nil, fmt.Errorf("extracting images: %w", err)
	}

	if len(imageReferences) == 0 {
		bundler.Log.Warn("no container images found in chart")
		return imageTarballs, imageInfos, nil
	}

	bundler.Log.Info("Found %d image(s):", len(imageReferences))
	for _, reference := range imageReferences {
		bundler.Log.Info("  - %s", reference)
	}

	if options.DryRun {
		bundler.Log.Info("Dry run: skipping image download and bundle creation")
		return nil, nil, nil
	}

	bundler.Log.Info("Downloading images...")
	imageDir := filepath.Join(tempDir, "images")
	pullResults, pullErrors := bundler.ImagePuller.PullAll(ctx, imageReferences, options.Platform, options.Concurrency, imageDir)

	if ctx.Err() != nil {
		return nil, nil, fmt.Errorf("cancelled during image download: %w", ctx.Err())
	}

	if len(pullErrors) > 0 {
		bundler.Log.Warn("failed to pull %d image(s):", len(pullErrors))
		for _, pullError := range pullErrors {
			bundler.Log.Warn("  %s", pullError.Error())
		}
	}

	for _, result := range pullResults {
		imageTarballs[result.File] = filepath.Join(imageDir, result.File)
		imageInfos = append(imageInfos, bundle.ImageInfo{
			Reference: result.Reference,
			Digest:    result.Digest,
			Platform:  options.Platform,
			File:      "images/" + result.File,
			Size:      result.Size,
		})
		bundler.Log.Debug("Image %s -> %s (%d bytes)", result.Reference, result.File, result.Size)
	}

	return imageTarballs, imageInfos, nil
}

func (bundler *Bundler) createBundle(outputPath string, manifest *bundle.Manifest, chartArchive string, imageTarballs map[string]string) error {
	if imageTarballs == nil {
		return nil // dry-run mode
	}

	bundler.Log.Info("Creating bundle %s...", outputPath)
	if err := bundler.BundleCreator.Create(outputPath, manifest, chartArchive, imageTarballs); err != nil {
		return fmt.Errorf("creating bundle: %w", err)
	}

	fileInfo, err := os.Stat(outputPath)
	if err != nil {
		return fmt.Errorf("stat output: %w", err)
	}

	bundler.Log.Info("Bundle created: %s (%.1f MB)", outputPath, float64(fileInfo.Size())/1024/1024)
	return nil
}

func resolveOutputPath(explicit string, chartName string, chartVersion string) string {
	if explicit != "" {
		return explicit
	}
	return fmt.Sprintf("%s-%s-bundle.tar.gz", chartName, chartVersion)
}

func buildManifest(fetchResult *chart.FetchResult) *bundle.Manifest {
	helmChart := fetchResult.Chart
	manifest := bundle.NewManifest(helmChart.Name(), helmChart.Metadata.Version)
	manifest.Chart = bundle.ChartInfo{
		Name:       helmChart.Name(),
		Version:    helmChart.Metadata.Version,
		AppVersion: helmChart.Metadata.AppVersion,
		File:       "chart/" + filepath.Base(fetchResult.ArchivePath),
	}
	for _, dependency := range helmChart.Metadata.Dependencies {
		manifest.Chart.Dependencies = append(manifest.Chart.Dependencies, bundle.DependencyInfo{
			Name:       dependency.Name,
			Version:    dependency.Version,
			Repository: dependency.Repository,
		})
	}
	return manifest
}

func mergeValues(valueFiles []string, setValues []string) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for _, filePath := range valueFiles {
		parsedValues, err := chartutil.ReadValuesFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("reading values file %s: %w", filePath, err)
		}
		chartutil.CoalesceTables(result, parsedValues)
	}

	for _, setValue := range setValues {
		if err := strvals.ParseInto(setValue, result); err != nil {
			return nil, fmt.Errorf("parsing --set %q: %w", setValue, err)
		}
	}

	return result, nil
}
