package chart

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

// FetchResult holds a loaded chart and its archive path on disk.
type FetchResult struct {
	Chart       *chart.Chart
	ArchivePath string
}

// Fetch downloads or loads a Helm chart and resolves its dependencies.
func Fetch(reference string, version string, repoURL string, tempDir string) (*FetchResult, error) {
	settings := cli.New()

	if isLocalReference(reference) {
		return fetchLocal(reference, tempDir, settings)
	}
	if repoURL == "" {
		return nil, fmt.Errorf("chart %q not found locally and --repo not specified; use --repo to fetch from a remote repository or provide a valid local path (e.g., ./%s)", reference, reference)
	}
	return fetchRemote(reference, version, repoURL, tempDir, settings)
}

func isLocalReference(reference string) bool {
	if strings.HasPrefix(reference, "./") || strings.HasPrefix(reference, "/") || strings.HasPrefix(reference, "../") {
		return true
	}
	fileInfo, err := os.Stat(reference)
	if err != nil {
		return false
	}
	return fileInfo.IsDir() || strings.HasSuffix(reference, ".tgz") || strings.HasSuffix(reference, ".tar.gz")
}

func fetchLocal(reference string, tempDir string, settings *cli.EnvSettings) (*FetchResult, error) {
	fileInfo, err := os.Stat(reference)
	if err != nil {
		return nil, fmt.Errorf("chart path not found: %w", err)
	}

	if !fileInfo.IsDir() {
		return loadChartArchive(reference)
	}

	return loadChartDirectory(reference, tempDir, settings)
}

func loadChartArchive(archivePath string) (*FetchResult, error) {
	helmChart, err := loader.LoadFile(archivePath)
	if err != nil {
		return nil, fmt.Errorf("loading chart archive: %w", err)
	}
	return &FetchResult{Chart: helmChart, ArchivePath: archivePath}, nil
}

func loadChartDirectory(directoryPath string, tempDir string, settings *cli.EnvSettings) (*FetchResult, error) {
	helmChart, err := loader.LoadDir(directoryPath)
	if err != nil {
		return nil, fmt.Errorf("loading chart directory: %w", err)
	}

	if err := resolveDependencies(directoryPath, helmChart, settings); err != nil {
		return nil, err
	}

	helmChart, err = loader.LoadDir(directoryPath)
	if err != nil {
		return nil, fmt.Errorf("reloading chart after dependency resolution: %w", err)
	}

	archivePath, err := chartutil.Save(helmChart, tempDir)
	if err != nil {
		return nil, fmt.Errorf("packaging chart: %w", err)
	}

	return &FetchResult{Chart: helmChart, ArchivePath: archivePath}, nil
}

func fetchRemote(chartName string, version string, repoURL string, tempDir string, settings *cli.EnvSettings) (*FetchResult, error) {
	if err := os.MkdirAll(settings.RepositoryCache, 0755); err != nil {
		return nil, fmt.Errorf("creating repo cache dir: %w", err)
	}

	repoEntry := &repo.Entry{Name: "breeze-tmp", URL: repoURL}

	if err := downloadRepositoryIndex(repoEntry, settings); err != nil {
		return nil, err
	}

	cleanupRepo, err := addTemporaryRepoEntry(repoEntry, settings)
	if err != nil {
		return nil, err
	}
	defer cleanupRepo()

	chartURL, err := repo.FindChartInRepoURL(repoURL, chartName, version, "", "", "", getter.All(settings))
	if err != nil {
		return nil, fmt.Errorf("finding chart in repo: %w", err)
	}

	downloadDir := filepath.Join(tempDir, "download")
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return nil, fmt.Errorf("creating download dir: %w", err)
	}

	chartDownloader := &downloader.ChartDownloader{
		Out:              os.Stderr,
		RepositoryCache:  settings.RepositoryCache,
		RepositoryConfig: settings.RepositoryConfig,
		Getters:          getter.All(settings),
	}

	savedPath, _, err := chartDownloader.DownloadTo(chartURL, version, downloadDir)
	if err != nil {
		return nil, fmt.Errorf("downloading chart: %w", err)
	}

	helmChart, err := loader.LoadFile(savedPath)
	if err != nil {
		return nil, fmt.Errorf("loading downloaded chart: %w", err)
	}

	savedPath, err = resolveRemoteDependencies(helmChart, savedPath, tempDir, downloadDir, settings)
	if err != nil {
		return nil, err
	}

	helmChart, err = loader.LoadFile(savedPath)
	if err != nil {
		return nil, fmt.Errorf("loading chart after dependency resolution: %w", err)
	}

	return &FetchResult{Chart: helmChart, ArchivePath: savedPath}, nil
}

func downloadRepositoryIndex(repoEntry *repo.Entry, settings *cli.EnvSettings) error {
	chartRepository, err := repo.NewChartRepository(repoEntry, getter.All(settings))
	if err != nil {
		return fmt.Errorf("creating chart repository: %w", err)
	}
	chartRepository.CachePath = settings.RepositoryCache

	if _, err := chartRepository.DownloadIndexFile(); err != nil {
		return fmt.Errorf("downloading repository index: %w", err)
	}
	return nil
}

func addTemporaryRepoEntry(repoEntry *repo.Entry, settings *cli.EnvSettings) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(settings.RepositoryConfig), 0755); err != nil {
		return nil, fmt.Errorf("creating repo config dir: %w", err)
	}

	repoFile, err := repo.LoadFile(settings.RepositoryConfig)
	if err != nil {
		repoFile = repo.NewFile()
	}

	existedBefore := repoFile.Has(repoEntry.Name)
	repoFile.Update(repoEntry)
	if err := repoFile.WriteFile(settings.RepositoryConfig, 0644); err != nil {
		return nil, fmt.Errorf("writing repo config: %w", err)
	}

	cleanup := func() {
		if !existedBefore {
			cleanupFile, loadErr := repo.LoadFile(settings.RepositoryConfig)
			if loadErr == nil && cleanupFile.Remove(repoEntry.Name) {
				_ = cleanupFile.WriteFile(settings.RepositoryConfig, 0644)
			}
		}
	}
	return cleanup, nil
}

func resolveRemoteDependencies(helmChart *chart.Chart, savedPath string, tempDir string, downloadDir string, settings *cli.EnvSettings) (string, error) {
	if len(helmChart.Metadata.Dependencies) == 0 {
		return savedPath, nil
	}

	extractDir := filepath.Join(tempDir, "extracted")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return "", fmt.Errorf("creating extract dir: %w", err)
	}
	if err := chartutil.ExpandFile(extractDir, savedPath); err != nil {
		return "", fmt.Errorf("extracting chart: %w", err)
	}

	chartDir := filepath.Join(extractDir, helmChart.Name())
	if err := resolveDependencies(chartDir, helmChart, settings); err != nil {
		return "", err
	}

	reloadedChart, err := loader.LoadDir(chartDir)
	if err != nil {
		return "", fmt.Errorf("reloading chart: %w", err)
	}

	repackagedPath, err := chartutil.Save(reloadedChart, downloadDir)
	if err != nil {
		return "", fmt.Errorf("repackaging chart: %w", err)
	}

	return repackagedPath, nil
}

func resolveDependencies(chartDir string, helmChart *chart.Chart, settings *cli.EnvSettings) error {
	if len(helmChart.Metadata.Dependencies) == 0 {
		return nil
	}

	dependencyManager := &downloader.Manager{
		Out:              os.Stderr,
		ChartPath:        chartDir,
		SkipUpdate:       false,
		Getters:          getter.All(settings),
		RepositoryCache:  settings.RepositoryCache,
		RepositoryConfig: settings.RepositoryConfig,
	}

	if err := dependencyManager.Update(); err != nil {
		if buildErr := dependencyManager.Build(); buildErr != nil {
			return fmt.Errorf("resolving chart dependencies: %w (build also failed: %v)", err, buildErr)
		}
	}
	return nil
}
