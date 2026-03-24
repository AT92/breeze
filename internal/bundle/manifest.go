package bundle

import (
	"time"

	"breeze/internal/version"

	"gopkg.in/yaml.v3"
)

// Manifest describes the contents of a breeze bundle archive.
type Manifest struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   ManifestMetadata `yaml:"metadata"`
	Chart      ChartInfo        `yaml:"chart"`
	Images     []ImageInfo      `yaml:"images"`
}

// ManifestMetadata holds bundle-level metadata.
type ManifestMetadata struct {
	Name          string    `yaml:"name"`
	Version       string    `yaml:"version"`
	CreatedAt     time.Time `yaml:"createdAt"`
	BreezeVersion string    `yaml:"breezeVersion"`
}

// ChartInfo describes the bundled Helm chart.
type ChartInfo struct {
	Name         string           `yaml:"name"`
	Version      string           `yaml:"version"`
	AppVersion   string           `yaml:"appVersion,omitempty"`
	File         string           `yaml:"file"`
	Dependencies []DependencyInfo `yaml:"dependencies,omitempty"`
}

// DependencyInfo describes a subchart dependency.
type DependencyInfo struct {
	Name       string `yaml:"name"`
	Version    string `yaml:"version"`
	Repository string `yaml:"repository,omitempty"`
}

// ImageInfo describes a bundled container image.
type ImageInfo struct {
	Reference string `yaml:"reference"`
	Digest    string `yaml:"digest"`
	Platform  string `yaml:"platform"`
	File      string `yaml:"file"`
	Size      int64  `yaml:"size"`
}

// NewManifest creates a new manifest with the given chart name and version.
func NewManifest(chartName, chartVersion string) *Manifest {
	return &Manifest{
		APIVersion: "breeze/v1",
		Kind:       "Bundle",
		Metadata: ManifestMetadata{
			Name:          chartName,
			Version:       chartVersion,
			CreatedAt:     time.Now().UTC(),
			BreezeVersion: version.Version,
		},
	}
}

// Marshal serializes the manifest to YAML.
func (manifest *Manifest) Marshal() ([]byte, error) {
	return yaml.Marshal(manifest)
}

// UnmarshalManifest parses YAML data into a Manifest.
func UnmarshalManifest(data []byte) (*Manifest, error) {
	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}
