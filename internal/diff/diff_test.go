package diff

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"breeze/internal/bundle"

	"gopkg.in/yaml.v3"
)

// --- Test helper: build a minimal bundle tar.gz in a temp directory ---

func buildTestBundle(t *testing.T, directory string, name string, manifest *bundle.Manifest, imageContents map[string]string) string {
	t.Helper()
	bundlePath := filepath.Join(directory, name)

	outputFile, err := os.Create(bundlePath)
	if err != nil {
		t.Fatalf("creating test bundle: %v", err)
	}
	defer outputFile.Close()

	gzipWriter := gzip.NewWriter(outputFile)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	manifestData, err := manifest.Marshal()
	if err != nil {
		t.Fatalf("marshaling manifest: %v", err)
	}
	writeTarEntry(t, tarWriter, "manifest.yaml", manifestData)
	writeTarEntry(t, tarWriter, manifest.Chart.File, []byte("fake chart archive"))

	for fileName, content := range imageContents {
		writeTarEntry(t, tarWriter, fileName, []byte(content))
	}

	return bundlePath
}

func writeTarEntry(t *testing.T, tarWriter *tar.Writer, name string, data []byte) {
	t.Helper()
	header := &tar.Header{Name: name, Mode: 0644, Size: int64(len(data))}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatalf("writing tar header for %s: %v", name, err)
	}
	if _, err := tarWriter.Write(data); err != nil {
		t.Fatalf("writing tar data for %s: %v", name, err)
	}
}

func makeManifest(chartName, chartVersion, appVersion string, images []bundle.ImageInfo) *bundle.Manifest {
	return &bundle.Manifest{
		APIVersion: "breeze/v1",
		Kind:       "Bundle",
		Metadata: bundle.ManifestMetadata{
			Name:          chartName,
			Version:       chartVersion,
			CreatedAt:     time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC),
			BreezeVersion: "0.4.0",
		},
		Chart: bundle.ChartInfo{
			Name:       chartName,
			Version:    chartVersion,
			AppVersion: appVersion,
			File:       "chart/" + chartName + "-" + chartVersion + ".tgz",
		},
		Images: images,
	}
}

func imageInfo(reference, digest, file string, size int64) bundle.ImageInfo {
	return bundle.ImageInfo{
		Reference: reference,
		Digest:    digest,
		Platform:  "linux/amd64",
		File:      file,
		Size:      size,
	}
}

// --- Comparison tests ---

func TestCompareManifests_IdenticalBundles(t *testing.T) {
	images := []bundle.ImageInfo{
		imageInfo("docker.io/library/nginx:1.25", "sha256:aaa", "images/nginx.tar", 1000),
	}
	oldManifest := makeManifest("my-chart", "1.0.0", "2.0.0", images)
	newManifest := makeManifest("my-chart", "1.0.0", "2.0.0", images)

	result := CompareManifests(oldManifest, newManifest)

	if !result.IsIdentical() {
		t.Error("expected bundles to be identical")
	}
	if result.Summary.UnchangedCount != 1 {
		t.Errorf("unchanged count = %d, want 1", result.Summary.UnchangedCount)
	}
}

func TestCompareManifests_AddedImagesOnly(t *testing.T) {
	oldManifest := makeManifest("my-chart", "1.0.0", "1.0.0", nil)
	newManifest := makeManifest("my-chart", "1.1.0", "1.1.0", []bundle.ImageInfo{
		imageInfo("docker.io/library/nginx:1.25", "sha256:aaa", "images/nginx.tar", 5000),
		imageInfo("docker.io/library/redis:7.2", "sha256:bbb", "images/redis.tar", 3000),
	})

	result := CompareManifests(oldManifest, newManifest)

	if result.IsIdentical() {
		t.Error("expected bundles to differ")
	}
	if len(result.Added) != 2 {
		t.Errorf("added count = %d, want 2", len(result.Added))
	}
	if len(result.Removed) != 0 {
		t.Errorf("removed count = %d, want 0", len(result.Removed))
	}
	if result.Summary.DeltaSize != 8000 {
		t.Errorf("delta size = %d, want 8000", result.Summary.DeltaSize)
	}
}

func TestCompareManifests_RemovedImagesOnly(t *testing.T) {
	oldManifest := makeManifest("my-chart", "1.0.0", "1.0.0", []bundle.ImageInfo{
		imageInfo("docker.io/library/nginx:1.25", "sha256:aaa", "images/nginx.tar", 5000),
	})
	newManifest := makeManifest("my-chart", "1.1.0", "1.1.0", nil)

	result := CompareManifests(oldManifest, newManifest)

	if len(result.Removed) != 1 {
		t.Errorf("removed count = %d, want 1", len(result.Removed))
	}
	if result.Removed[0].Reference != "docker.io/library/nginx:1.25" {
		t.Errorf("removed reference = %q", result.Removed[0].Reference)
	}
}

func TestCompareManifests_ChangedImages(t *testing.T) {
	oldManifest := makeManifest("my-chart", "1.0.0", "1.0.0", []bundle.ImageInfo{
		imageInfo("docker.io/library/nginx:1.25", "sha256:old111", "images/nginx.tar", 5000),
	})
	newManifest := makeManifest("my-chart", "1.1.0", "1.1.0", []bundle.ImageInfo{
		imageInfo("docker.io/library/nginx:1.25", "sha256:new222", "images/nginx.tar", 6000),
	})

	result := CompareManifests(oldManifest, newManifest)

	if len(result.Changed) != 1 {
		t.Fatalf("changed count = %d, want 1", len(result.Changed))
	}
	changed := result.Changed[0]
	if changed.OldDigest != "sha256:old111" || changed.NewDigest != "sha256:new222" {
		t.Errorf("digests = %q -> %q, want old111 -> new222", changed.OldDigest, changed.NewDigest)
	}
	if result.Summary.DeltaSize != 6000 {
		t.Errorf("delta size = %d, want 6000", result.Summary.DeltaSize)
	}
}

func TestCompareManifests_MixedChanges(t *testing.T) {
	oldManifest := makeManifest("my-chart", "1.0.0", "1.0.0", []bundle.ImageInfo{
		imageInfo("docker.io/library/nginx:1.25", "sha256:aaa", "images/nginx.tar", 5000),
		imageInfo("docker.io/library/redis:7.2", "sha256:bbb", "images/redis.tar", 3000),
		imageInfo("docker.io/library/postgres:16", "sha256:ccc", "images/pg.tar", 8000),
	})
	newManifest := makeManifest("my-chart", "1.1.0", "1.1.0", []bundle.ImageInfo{
		imageInfo("docker.io/library/nginx:1.25", "sha256:aaa", "images/nginx.tar", 5000),
		imageInfo("docker.io/library/redis:7.2", "sha256:bbb-new", "images/redis.tar", 3500),
		imageInfo("docker.io/library/mysql:8.0", "sha256:ddd", "images/mysql.tar", 7000),
	})

	result := CompareManifests(oldManifest, newManifest)

	if len(result.Unchanged) != 1 {
		t.Errorf("unchanged = %d, want 1 (nginx)", len(result.Unchanged))
	}
	if len(result.Changed) != 1 {
		t.Errorf("changed = %d, want 1 (redis)", len(result.Changed))
	}
	if len(result.Added) != 1 {
		t.Errorf("added = %d, want 1 (mysql)", len(result.Added))
	}
	if len(result.Removed) != 1 {
		t.Errorf("removed = %d, want 1 (postgres)", len(result.Removed))
	}
	if result.Summary.OldTotalSize != 16000 {
		t.Errorf("old total = %d, want 16000", result.Summary.OldTotalSize)
	}
	if result.Summary.NewTotalSize != 15500 {
		t.Errorf("new total = %d, want 15500", result.Summary.NewTotalSize)
	}
}

// --- Report format tests ---

func TestFormatText_Identical(t *testing.T) {
	images := []bundle.ImageInfo{imageInfo("nginx:1.25", "sha256:aaa", "images/n.tar", 1000)}
	result := CompareManifests(
		makeManifest("app", "1.0.0", "1.0.0", images),
		makeManifest("app", "1.0.0", "1.0.0", images),
	)
	text := FormatText(result)
	if !strings.Contains(text, "Bundles are identical") {
		t.Errorf("expected 'Bundles are identical', got: %s", text)
	}
}

func TestFormatText_WithChanges(t *testing.T) {
	result := CompareManifests(
		makeManifest("app", "1.0.0", "1.0.0", []bundle.ImageInfo{
			imageInfo("nginx:1.25", "sha256:old", "images/n.tar", 5000000),
		}),
		makeManifest("app", "1.1.0", "1.1.0", []bundle.ImageInfo{
			imageInfo("nginx:1.25", "sha256:new", "images/n.tar", 6000000),
			imageInfo("redis:7.2", "sha256:r", "images/r.tar", 3000000),
		}),
	)
	text := FormatText(result)
	if !strings.Contains(text, "1.0.0 → 1.1.0") {
		t.Error("missing version transition")
	}
	if !strings.Contains(text, "Images added (1)") {
		t.Error("missing added section")
	}
	if !strings.Contains(text, "Images changed (1)") {
		t.Error("missing changed section")
	}
	if !strings.Contains(text, "Summary:") {
		t.Error("missing summary")
	}
}

func TestFormatJSON_Parseable(t *testing.T) {
	result := CompareManifests(
		makeManifest("app", "1.0.0", "1.0.0", nil),
		makeManifest("app", "1.1.0", "1.1.0", []bundle.ImageInfo{
			imageInfo("nginx:1.25", "sha256:aaa", "images/n.tar", 5000),
		}),
	)
	jsonOutput, err := FormatJSON(result)
	if err != nil {
		t.Fatalf("FormatJSON error: %v", err)
	}
	var parsed DiffResult
	if err := json.Unmarshal([]byte(jsonOutput), &parsed); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}
	if parsed.Summary.AddedCount != 1 {
		t.Errorf("added count = %d, want 1", parsed.Summary.AddedCount)
	}
	if parsed.ChartName != "app" {
		t.Errorf("chart name = %q, want 'app'", parsed.ChartName)
	}
}

func TestFormatYAML_Parseable(t *testing.T) {
	result := CompareManifests(
		makeManifest("app", "1.0.0", "1.0.0", nil),
		makeManifest("app", "1.1.0", "1.1.0", []bundle.ImageInfo{
			imageInfo("nginx:1.25", "sha256:aaa", "images/n.tar", 5000),
		}),
	)
	yamlOutput, err := FormatYAML(result)
	if err != nil {
		t.Fatalf("FormatYAML error: %v", err)
	}
	var parsed DiffResult
	if err := yaml.Unmarshal([]byte(yamlOutput), &parsed); err != nil {
		t.Fatalf("YAML parse error: %v", err)
	}
	if parsed.Summary.AddedCount != 1 {
		t.Errorf("added count = %d, want 1", parsed.Summary.AddedCount)
	}
}

// --- Delta bundle tests ---

func TestGenerateDeltaBundle_CreatesValidArchive(t *testing.T) {
	tempDir := t.TempDir()

	sharedImage := imageInfo("docker.io/library/nginx:1.25", "sha256:aaa", "images/nginx.tar", 5000)
	addedImage := imageInfo("docker.io/library/redis:7.2", "sha256:bbb", "images/redis.tar", 3000)
	changedImage := imageInfo("docker.io/library/postgres:16", "sha256:ccc-new", "images/pg.tar", 8000)

	oldManifest := makeManifest("app", "1.0.0", "1.0.0", []bundle.ImageInfo{
		sharedImage,
		imageInfo("docker.io/library/postgres:16", "sha256:ccc-old", "images/pg.tar", 7000),
	})
	newManifest := makeManifest("app", "1.1.0", "1.1.0", []bundle.ImageInfo{
		sharedImage,
		addedImage,
		changedImage,
	})

	oldBundlePath := buildTestBundle(t, tempDir, "old.tar.gz", oldManifest, map[string]string{
		"images/nginx.tar": "nginx-image-data",
		"images/pg.tar":    "postgres-old-data",
	})
	newBundlePath := buildTestBundle(t, tempDir, "new.tar.gz", newManifest, map[string]string{
		"images/nginx.tar": "nginx-image-data",
		"images/redis.tar": "redis-image-data",
		"images/pg.tar":    "postgres-new-data",
	})

	diffResult := CompareManifests(oldManifest, newManifest)
	deltaPath := filepath.Join(tempDir, "delta.tar.gz")

	if err := GenerateDeltaBundle(deltaPath, oldBundlePath, newBundlePath, diffResult, newManifest); err != nil {
		t.Fatalf("GenerateDeltaBundle error: %v", err)
	}

	if _, err := os.Stat(deltaPath); err != nil {
		t.Fatalf("delta bundle not created: %v", err)
	}

	entries := listTarEntries(t, deltaPath)

	if !containsEntry(entries, "manifest.yaml") {
		t.Error("delta bundle missing manifest.yaml")
	}
	if !containsEntry(entries, "chart/app-1.1.0.tgz") {
		t.Error("delta bundle missing chart")
	}
	if !containsEntry(entries, "images/redis.tar") {
		t.Error("delta bundle missing added image (redis)")
	}
	if !containsEntry(entries, "images/pg.tar") {
		t.Error("delta bundle missing changed image (postgres)")
	}
	if containsEntry(entries, "images/nginx.tar") {
		t.Error("delta bundle should NOT contain unchanged image (nginx)")
	}

	deltaManifest := readManifestFromBundle(t, deltaPath)
	if !strings.Contains(string(deltaManifest), "DeltaBundle") {
		t.Error("delta manifest should have kind: DeltaBundle")
	}
	if !strings.Contains(string(deltaManifest), "baselineVersion") {
		t.Error("delta manifest should contain baselineVersion")
	}
}

func TestGenerateDeltaBundle_IdenticalBundles_NoOutput(t *testing.T) {
	images := []bundle.ImageInfo{
		imageInfo("docker.io/library/nginx:1.25", "sha256:aaa", "images/nginx.tar", 5000),
	}
	manifest := makeManifest("app", "1.0.0", "1.0.0", images)
	diffResult := CompareManifests(manifest, manifest)

	if !diffResult.IsIdentical() {
		t.Fatal("expected identical bundles")
	}
	// When identical, the command should skip delta generation (handled by cmd/diff.go)
}

// --- Error case tests ---

func TestCompareManifests_EmptyManifests(t *testing.T) {
	oldManifest := makeManifest("app", "1.0.0", "1.0.0", nil)
	newManifest := makeManifest("app", "1.0.0", "1.0.0", nil)

	result := CompareManifests(oldManifest, newManifest)

	if !result.IsIdentical() {
		t.Error("empty manifests should be identical")
	}
	if result.Summary.DeltaSize != 0 {
		t.Errorf("delta size = %d, want 0", result.Summary.DeltaSize)
	}
}

func TestReadManifest_MissingFile(t *testing.T) {
	_, err := bundle.ReadManifest("/nonexistent/path.tar.gz")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestReadManifest_InvalidTar(t *testing.T) {
	tempDir := t.TempDir()
	invalidPath := filepath.Join(tempDir, "invalid.tar.gz")
	os.WriteFile(invalidPath, []byte("not a tar"), 0644)

	_, err := bundle.ReadManifest(invalidPath)
	if err == nil {
		t.Error("expected error for invalid tar")
	}
}

func TestReadManifest_NoManifestInBundle(t *testing.T) {
	tempDir := t.TempDir()
	bundlePath := filepath.Join(tempDir, "no-manifest.tar.gz")

	outputFile, _ := os.Create(bundlePath)
	gzipWriter := gzip.NewWriter(outputFile)
	tarWriter := tar.NewWriter(gzipWriter)
	writeTarEntry(&testing.T{}, tarWriter, "some-other-file.txt", []byte("hello"))
	tarWriter.Close()
	gzipWriter.Close()
	outputFile.Close()

	_, err := bundle.ReadManifest(bundlePath)
	if err == nil {
		t.Error("expected error for bundle without manifest")
	}
	if !strings.Contains(err.Error(), "manifest.yaml not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Helper functions for test assertions ---

func listTarEntries(t *testing.T, bundlePath string) []string {
	t.Helper()
	file, err := os.Open(bundlePath)
	if err != nil {
		t.Fatalf("opening bundle: %v", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("reading gzip: %v", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	var entries []string
	for {
		header, err := tarReader.Next()
		if err != nil {
			break
		}
		entries = append(entries, header.Name)
	}
	return entries
}

func containsEntry(entries []string, name string) bool {
	for _, entry := range entries {
		if entry == name {
			return true
		}
	}
	return false
}

func readManifestFromBundle(t *testing.T, bundlePath string) []byte {
	t.Helper()
	file, err := os.Open(bundlePath)
	if err != nil {
		t.Fatalf("opening bundle: %v", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("reading gzip: %v", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err != nil {
			break
		}
		if header.Name == "manifest.yaml" {
			data := make([]byte, header.Size)
			tarReader.Read(data)
			return data
		}
	}
	t.Fatal("manifest.yaml not found in bundle")
	return nil
}
