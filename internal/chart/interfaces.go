package chart

import (
	helmchart "helm.sh/helm/v3/pkg/chart"
)

// Fetcher downloads or loads a Helm chart and resolves its dependencies.
type Fetcher interface {
	Fetch(reference, version, repoURL, tempDir string) (*FetchResult, error)
}

// ImageExtractor finds container image references in a Helm chart.
type ImageExtractor interface {
	Extract(helmChart *helmchart.Chart, valuesOverrides map[string]interface{}) ([]string, error)
}

// DefaultFetcher implements Fetcher using the Helm SDK.
type DefaultFetcher struct{}

func (fetcher *DefaultFetcher) Fetch(reference, version, repoURL, tempDir string) (*FetchResult, error) {
	return Fetch(reference, version, repoURL, tempDir)
}

// DefaultImageExtractor implements ImageExtractor using template rendering and values walking.
type DefaultImageExtractor struct{}

func (extractor *DefaultImageExtractor) Extract(helmChart *helmchart.Chart, valuesOverrides map[string]interface{}) ([]string, error) {
	return ExtractImages(helmChart, valuesOverrides)
}
