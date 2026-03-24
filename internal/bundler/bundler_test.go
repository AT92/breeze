package bundler

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"breeze/internal/bundle"
	"breeze/internal/chart"
	"breeze/internal/image"
	"breeze/internal/log"

	helmchart "helm.sh/helm/v3/pkg/chart"
)

// --- Fakes ---

type fakeFetcher struct {
	chartName    string
	chartVersion string
	appVersion   string
	deps         []*helmchart.Dependency
	archivePath  string
	err          error
}

func (f *fakeFetcher) Fetch(ref, version, repoURL, tmpDir string) (*chart.FetchResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	chrt := &helmchart.Chart{
		Metadata: &helmchart.Metadata{
			Name:         f.chartName,
			Version:      f.chartVersion,
			AppVersion:   f.appVersion,
			Dependencies: f.deps,
		},
	}
	return &chart.FetchResult{Chart: chrt, ArchivePath: f.archivePath}, nil
}

type fakeImageExtractor struct {
	images []string
	err    error
}

func (f *fakeImageExtractor) Extract(chrt *helmchart.Chart, overrides map[string]interface{}) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.images, nil
}

type fakePuller struct {
	results []image.PullResult
	errs    []error
}

func (f *fakePuller) PullAll(ctx context.Context, refs []string, platform string, concurrency int, destDir string) ([]image.PullResult, []error) {
	if len(f.errs) > 0 {
		return nil, f.errs
	}
	// Create fake tar files so the archive step can find them
	for _, r := range f.results {
		filePath := filepath.Join(destDir, r.File)
		os.MkdirAll(filepath.Dir(filePath), 0755)
		os.WriteFile(filePath, []byte("fake image data"), 0644)
	}
	return f.results, nil
}

type fakeCreator struct {
	called       bool
	manifest     *bundle.Manifest
	chartArchive string
	images       map[string]string
	err          error
}

func (f *fakeCreator) Create(outputPath string, manifest *bundle.Manifest, chartArchive string, imageTarballs map[string]string) error {
	f.called = true
	f.manifest = manifest
	f.chartArchive = chartArchive
	f.images = imageTarballs
	if f.err != nil {
		return f.err
	}
	// Create a dummy file so stat succeeds
	return os.WriteFile(outputPath, []byte("fake bundle"), 0644)
}

// --- Tests ---

func TestBundlerRun_FullPipeline(t *testing.T) {
	tmpDir := t.TempDir()
	chartArchive := filepath.Join(tmpDir, "my-chart-1.0.0.tgz")
	os.WriteFile(chartArchive, []byte("fake chart"), 0644)
	outputPath := filepath.Join(tmpDir, "output-bundle.tar.gz")

	creator := &fakeCreator{}
	var out bytes.Buffer

	b := &Bundler{
		Fetcher: &fakeFetcher{
			chartName:    "my-chart",
			chartVersion: "1.0.0",
			appVersion:   "2.0.0",
			archivePath:  chartArchive,
			deps: []*helmchart.Dependency{
				{Name: "redis", Version: "17.0.0", Repository: "https://charts.bitnami.com/bitnami"},
			},
		},
		ImageExtractor: &fakeImageExtractor{
			images: []string{"docker.io/library/nginx:1.25", "docker.io/bitnami/redis:7.2"},
		},
		ImagePuller: &fakePuller{
			results: []image.PullResult{
				{Reference: "docker.io/library/nginx:1.25", Digest: "sha256:aaa", File: "nginx.tar", Size: 1000},
				{Reference: "docker.io/bitnami/redis:7.2", Digest: "sha256:bbb", File: "redis.tar", Size: 2000},
			},
		},
		BundleCreator: creator,
		Log:           log.NewWithWriter(&out, log.LevelNormal),
	}

	err := b.Run(context.Background(), Options{
		ChartRef:    "my-chart",
		Version:     "1.0.0",
		RepoURL:     "https://example.com",
		Output:      outputPath,
		Platform:    "linux/amd64",
		Concurrency: 2,
	})

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify creator was called
	if !creator.called {
		t.Fatal("BundleCreator.Create was not called")
	}

	// Verify manifest
	m := creator.manifest
	if m.Metadata.Name != "my-chart" {
		t.Errorf("manifest name = %q, want %q", m.Metadata.Name, "my-chart")
	}
	if m.Metadata.Version != "1.0.0" {
		t.Errorf("manifest version = %q, want %q", m.Metadata.Version, "1.0.0")
	}
	if m.Chart.AppVersion != "2.0.0" {
		t.Errorf("chart appVersion = %q, want %q", m.Chart.AppVersion, "2.0.0")
	}
	if len(m.Chart.Dependencies) != 1 {
		t.Fatalf("dependencies count = %d, want 1", len(m.Chart.Dependencies))
	}
	if m.Chart.Dependencies[0].Name != "redis" {
		t.Errorf("dependency name = %q, want %q", m.Chart.Dependencies[0].Name, "redis")
	}
	if len(m.Images) != 2 {
		t.Fatalf("images count = %d, want 2", len(m.Images))
	}
	if m.Images[0].Reference != "docker.io/library/nginx:1.25" {
		t.Errorf("image[0] ref = %q, want %q", m.Images[0].Reference, "docker.io/library/nginx:1.25")
	}
	if m.Images[0].Platform != "linux/amd64" {
		t.Errorf("image[0] platform = %q, want %q", m.Images[0].Platform, "linux/amd64")
	}

	// Verify progress output
	progress := out.String()
	if !strings.Contains(progress, "Fetching chart...") {
		t.Error("missing 'Fetching chart...' in output")
	}
	if !strings.Contains(progress, "Found 2 image(s)") {
		t.Error("missing image count in output")
	}
	if !strings.Contains(progress, "Bundle created") {
		t.Error("missing 'Bundle created' in output")
	}
}

func TestBundlerRun_SkipImages(t *testing.T) {
	tmpDir := t.TempDir()
	chartArchive := filepath.Join(tmpDir, "my-chart-1.0.0.tgz")
	os.WriteFile(chartArchive, []byte("fake chart"), 0644)
	outputPath := filepath.Join(tmpDir, "output-bundle.tar.gz")

	creator := &fakeCreator{}

	b := &Bundler{
		Fetcher: &fakeFetcher{
			chartName:    "my-chart",
			chartVersion: "1.0.0",
			archivePath:  chartArchive,
		},
		ImageExtractor: &fakeImageExtractor{images: []string{"should-not-be-pulled"}},
		ImagePuller:    &fakePuller{}, // should not be called
		BundleCreator:  creator,
		Log:            log.NewWithWriter(&bytes.Buffer{}, log.LevelNormal),
	}

	err := b.Run(context.Background(), Options{
		ChartRef:    "my-chart",
		Output:      outputPath,
		Platform:    "linux/amd64",
		SkipImages:  true,
		Concurrency: 1,
	})

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(creator.manifest.Images) != 0 {
		t.Errorf("expected 0 images with --skip-images, got %d", len(creator.manifest.Images))
	}
}

func TestBundlerRun_FetchError(t *testing.T) {
	b := &Bundler{
		Fetcher:        &fakeFetcher{err: fmt.Errorf("repo not found")},
		ImageExtractor: &fakeImageExtractor{},
		ImagePuller:    &fakePuller{},
		BundleCreator:  &fakeCreator{},
		Log:            log.NewWithWriter(&bytes.Buffer{}, log.LevelNormal),
	}

	err := b.Run(context.Background(), Options{
		ChartRef:    "nonexistent",
		Platform:    "linux/amd64",
		Concurrency: 1,
	})

	if err == nil {
		t.Fatal("expected error from fetch failure")
	}
	if !strings.Contains(err.Error(), "fetching chart") {
		t.Errorf("error = %q, want it to contain 'fetching chart'", err.Error())
	}
}

func TestBundlerRun_ImagePullError_ContinuesWithPartialBundle(t *testing.T) {
	tmpDir := t.TempDir()
	chartArchive := filepath.Join(tmpDir, "chart.tgz")
	os.WriteFile(chartArchive, []byte("fake"), 0644)
	outputPath := filepath.Join(tmpDir, "bundle.tar.gz")

	var out bytes.Buffer
	creator := &fakeCreator{}

	b := &Bundler{
		Fetcher: &fakeFetcher{
			chartName:    "chart",
			chartVersion: "1.0.0",
			archivePath:  chartArchive,
		},
		ImageExtractor: &fakeImageExtractor{images: []string{"bad-image:latest"}},
		ImagePuller: &fakePuller{
			errs: []error{fmt.Errorf("pull failed: UNAUTHORIZED")},
		},
		BundleCreator: creator,
		Log:           log.NewWithWriter(&out, log.LevelNormal),
	}

	err := b.Run(context.Background(), Options{
		ChartRef:    "chart",
		Output:      outputPath,
		Platform:    "linux/amd64",
		Concurrency: 1,
	})

	if err != nil {
		t.Fatalf("expected no error (partial bundle), got: %v", err)
	}
	if !creator.called {
		t.Fatal("expected bundle to be created despite pull failures")
	}
	if !strings.Contains(out.String(), "Warning: failed to pull") {
		t.Error("expected warning about failed pulls in output")
	}
}

func TestBundlerRun_NoImagesFound(t *testing.T) {
	tmpDir := t.TempDir()
	chartArchive := filepath.Join(tmpDir, "chart.tgz")
	os.WriteFile(chartArchive, []byte("fake"), 0644)
	outputPath := filepath.Join(tmpDir, "bundle.tar.gz")

	creator := &fakeCreator{}
	var out bytes.Buffer

	b := &Bundler{
		Fetcher: &fakeFetcher{
			chartName:    "chart",
			chartVersion: "1.0.0",
			archivePath:  chartArchive,
		},
		ImageExtractor: &fakeImageExtractor{images: []string{}},
		ImagePuller:    &fakePuller{},
		BundleCreator:  creator,
		Log:           log.NewWithWriter(&out, log.LevelNormal),
	}

	err := b.Run(context.Background(), Options{
		ChartRef:    "chart",
		Output:      outputPath,
		Platform:    "linux/amd64",
		Concurrency: 1,
	})

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(out.String(), "Warning: no container images found") {
		t.Error("expected warning about no images found")
	}
	if len(creator.manifest.Images) != 0 {
		t.Errorf("expected 0 images, got %d", len(creator.manifest.Images))
	}
}

func TestBundlerRun_DefaultOutputPath(t *testing.T) {
	tmpDir := t.TempDir()
	chartArchive := filepath.Join(tmpDir, "chart.tgz")
	os.WriteFile(chartArchive, []byte("fake"), 0644)

	// Change to tmpDir so the default output path lands there
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	creator := &fakeCreator{}

	b := &Bundler{
		Fetcher: &fakeFetcher{
			chartName:    "my-app",
			chartVersion: "3.2.1",
			archivePath:  chartArchive,
		},
		ImageExtractor: &fakeImageExtractor{images: []string{}},
		ImagePuller:    &fakePuller{},
		BundleCreator:  creator,
		Log:            log.NewWithWriter(&bytes.Buffer{}, log.LevelNormal),
	}

	err := b.Run(context.Background(), Options{
		ChartRef:    "my-app",
		SkipImages:  true,
		Platform:    "linux/amd64",
		Concurrency: 1,
	})

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify the default output filename was used
	expectedOutput := "my-app-3.2.1-bundle.tar.gz"
	if _, err := os.Stat(filepath.Join(tmpDir, expectedOutput)); err != nil {
		t.Errorf("expected default output file %q to exist", expectedOutput)
	}
}
