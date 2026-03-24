package bundle

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManifestRoundTrip(t *testing.T) {
	original := &Manifest{
		APIVersion: "breeze/v1",
		Kind:       "Bundle",
		Metadata: ManifestMetadata{
			Name:          "my-chart",
			Version:       "1.2.3",
			CreatedAt:     time.Date(2026, 3, 23, 10, 0, 0, 0, time.UTC),
			BreezeVersion: "0.1.0",
		},
		Chart: ChartInfo{
			Name:       "my-chart",
			Version:    "1.2.3",
			AppVersion: "1.0.0",
			File:       "chart/my-chart-1.2.3.tgz",
			Dependencies: []DependencyInfo{
				{Name: "redis", Version: "17.0.0", Repository: "https://charts.bitnami.com/bitnami"},
			},
		},
		Images: []ImageInfo{
			{
				Reference: "docker.io/library/nginx:1.25.3",
				Digest:    "sha256:abc123",
				Platform:  "linux/amd64",
				File:      "images/docker.io_library_nginx_1.25.3.tar",
				Size:      54321000,
			},
		},
	}

	data, err := original.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	restored, err := UnmarshalManifest(data)
	if err != nil {
		t.Fatalf("UnmarshalManifest() error = %v", err)
	}

	// Verify key fields
	if restored.APIVersion != original.APIVersion {
		t.Errorf("APIVersion = %q, want %q", restored.APIVersion, original.APIVersion)
	}
	if restored.Kind != original.Kind {
		t.Errorf("Kind = %q, want %q", restored.Kind, original.Kind)
	}
	if restored.Metadata.Name != original.Metadata.Name {
		t.Errorf("Metadata.Name = %q, want %q", restored.Metadata.Name, original.Metadata.Name)
	}
	if restored.Metadata.Version != original.Metadata.Version {
		t.Errorf("Metadata.Version = %q, want %q", restored.Metadata.Version, original.Metadata.Version)
	}
	if restored.Chart.Name != original.Chart.Name {
		t.Errorf("Chart.Name = %q, want %q", restored.Chart.Name, original.Chart.Name)
	}
	if restored.Chart.AppVersion != original.Chart.AppVersion {
		t.Errorf("Chart.AppVersion = %q, want %q", restored.Chart.AppVersion, original.Chart.AppVersion)
	}
	if len(restored.Chart.Dependencies) != 1 {
		t.Fatalf("Chart.Dependencies count = %d, want 1", len(restored.Chart.Dependencies))
	}
	if restored.Chart.Dependencies[0].Name != "redis" {
		t.Errorf("Dependency name = %q, want %q", restored.Chart.Dependencies[0].Name, "redis")
	}
	if len(restored.Images) != 1 {
		t.Fatalf("Images count = %d, want 1", len(restored.Images))
	}
	if restored.Images[0].Reference != original.Images[0].Reference {
		t.Errorf("Image reference = %q, want %q", restored.Images[0].Reference, original.Images[0].Reference)
	}
	if restored.Images[0].Size != original.Images[0].Size {
		t.Errorf("Image size = %d, want %d", restored.Images[0].Size, original.Images[0].Size)
	}
}

func TestNewManifest(t *testing.T) {
	m := NewManifest("test-chart", "2.0.0")

	if m.APIVersion != "breeze/v1" {
		t.Errorf("APIVersion = %q, want %q", m.APIVersion, "breeze/v1")
	}
	if m.Kind != "Bundle" {
		t.Errorf("Kind = %q, want %q", m.Kind, "Bundle")
	}
	if m.Metadata.Name != "test-chart" {
		t.Errorf("Name = %q, want %q", m.Metadata.Name, "test-chart")
	}
	if m.Metadata.Version != "2.0.0" {
		t.Errorf("Version = %q, want %q", m.Metadata.Version, "2.0.0")
	}
	if m.Metadata.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestCreateAndReadManifest(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fake chart archive
	chartContent := []byte("fake chart content")
	chartPath := filepath.Join(tmpDir, "test-chart-1.0.0.tgz")
	if err := os.WriteFile(chartPath, chartContent, 0644); err != nil {
		t.Fatalf("writing fake chart: %v", err)
	}

	// Create a fake image tarball
	imageContent := []byte("fake image content")
	imagePath := filepath.Join(tmpDir, "fake-image.tar")
	if err := os.WriteFile(imagePath, imageContent, 0644); err != nil {
		t.Fatalf("writing fake image: %v", err)
	}

	manifest := NewManifest("test-chart", "1.0.0")
	manifest.Chart = ChartInfo{
		Name:    "test-chart",
		Version: "1.0.0",
		File:    "chart/test-chart-1.0.0.tgz",
	}
	manifest.Images = []ImageInfo{
		{
			Reference: "docker.io/library/nginx:1.25",
			Digest:    "sha256:deadbeef",
			Platform:  "linux/amd64",
			File:      "images/fake-image.tar",
			Size:      int64(len(imageContent)),
		},
	}

	bundlePath := filepath.Join(tmpDir, "test-bundle.tar.gz")
	imageTarballs := map[string]string{
		"fake-image.tar": imagePath,
	}

	// Create the bundle
	if err := Create(bundlePath, manifest, chartPath, imageTarballs); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify bundle file exists
	stat, err := os.Stat(bundlePath)
	if err != nil {
		t.Fatalf("bundle file not found: %v", err)
	}
	if stat.Size() == 0 {
		t.Error("bundle file is empty")
	}

	// Read manifest back
	restored, err := ReadManifest(bundlePath)
	if err != nil {
		t.Fatalf("ReadManifest() error = %v", err)
	}

	if restored.Metadata.Name != "test-chart" {
		t.Errorf("Name = %q, want %q", restored.Metadata.Name, "test-chart")
	}
	if restored.Metadata.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", restored.Metadata.Version, "1.0.0")
	}
	if len(restored.Images) != 1 {
		t.Fatalf("Images count = %d, want 1", len(restored.Images))
	}
	if restored.Images[0].Reference != "docker.io/library/nginx:1.25" {
		t.Errorf("Image reference = %q, want %q", restored.Images[0].Reference, "docker.io/library/nginx:1.25")
	}
}

func TestReadManifestMissingFile(t *testing.T) {
	_, err := ReadManifest("/nonexistent/path.tar.gz")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestUnmarshalInvalidYAML(t *testing.T) {
	_, err := UnmarshalManifest([]byte("{{invalid yaml"))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}
