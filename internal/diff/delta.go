package diff

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"breeze/internal/bundle"
	"breeze/internal/version"

	"gopkg.in/yaml.v3"
)

// DeltaManifest describes a delta bundle containing only changed/added artifacts.
type DeltaManifest struct {
	APIVersion string              `yaml:"apiVersion"`
	Kind       string              `yaml:"kind"`
	Metadata   bundle.ManifestMetadata `yaml:"metadata"`
	Delta      DeltaInfo           `yaml:"delta"`
	Chart      bundle.ChartInfo    `yaml:"chart"`
	Images     []DeltaImageInfo    `yaml:"images"`
}

// DeltaInfo holds delta-specific metadata.
type DeltaInfo struct {
	BaselineVersion string       `yaml:"baselineVersion"`
	BaselineDigest  string       `yaml:"baselineDigest"`
	Summary         DeltaSummary `yaml:"summary"`
}

// DeltaSummary holds counts and sizes for the delta.
type DeltaSummary struct {
	Added           int   `yaml:"added"`
	Removed         int   `yaml:"removed"`
	Changed         int   `yaml:"changed"`
	Unchanged       int   `yaml:"unchanged"`
	FullBundleSize  int64 `yaml:"fullBundleSize"`
	DeltaBundleSize int64 `yaml:"deltaBundleSize"`
}

// DeltaImageInfo describes an image in a delta bundle.
type DeltaImageInfo struct {
	Reference      string `yaml:"reference"`
	Digest         string `yaml:"digest"`
	Status         string `yaml:"status"`
	PreviousDigest string `yaml:"previousDigest,omitempty"`
	File           string `yaml:"file"`
	Size           int64  `yaml:"size"`
}

// GenerateDeltaBundle creates a delta bundle containing only added/changed images
// streamed directly from the new bundle archive.
func GenerateDeltaBundle(outputPath string, oldBundlePath string, newBundlePath string, diffResult *DiffResult, newManifest *bundle.Manifest) error {
	includedFiles := buildIncludedFileSet(diffResult)

	baselineDigest, err := computeFileDigest(oldBundlePath)
	if err != nil {
		return fmt.Errorf("computing baseline digest: %w", err)
	}

	deltaManifest := buildDeltaManifest(diffResult, newManifest, baselineDigest, includedFiles)

	return writeDeltaBundle(outputPath, newBundlePath, deltaManifest, includedFiles)
}

func buildIncludedFileSet(diffResult *DiffResult) map[string]ImageStatus {
	includedFiles := make(map[string]ImageStatus)
	for _, image := range diffResult.Added {
		includedFiles[image.File] = ImageAdded
	}
	for _, image := range diffResult.Changed {
		includedFiles[image.File] = ImageChanged
	}
	return includedFiles
}

func buildDeltaManifest(diffResult *DiffResult, newManifest *bundle.Manifest, baselineDigest string, includedFiles map[string]ImageStatus) *DeltaManifest {
	deltaManifest := &DeltaManifest{
		APIVersion: "breeze/v1",
		Kind:       "DeltaBundle",
		Metadata: bundle.ManifestMetadata{
			Name:          newManifest.Metadata.Name,
			Version:       newManifest.Metadata.Version,
			CreatedAt:     time.Now().UTC(),
			BreezeVersion: version.Version,
		},
		Delta: DeltaInfo{
			BaselineVersion: diffResult.Chart.OldVersion,
			BaselineDigest:  baselineDigest,
			Summary: DeltaSummary{
				Added:          diffResult.Summary.AddedCount,
				Removed:        diffResult.Summary.RemovedCount,
				Changed:        diffResult.Summary.ChangedCount,
				Unchanged:      diffResult.Summary.UnchangedCount,
				FullBundleSize: diffResult.Summary.NewTotalSize,
			},
		},
		Chart: newManifest.Chart,
	}

	oldDigestByReference := buildOldDigestMap(diffResult)

	for _, newImage := range newManifest.Images {
		status, isIncluded := includedFiles[newImage.File]
		if !isIncluded {
			continue
		}
		deltaImage := DeltaImageInfo{
			Reference: newImage.Reference,
			Digest:    newImage.Digest,
			Status:    string(status),
			File:      newImage.File,
			Size:      newImage.Size,
		}
		if status == ImageChanged {
			deltaImage.PreviousDigest = oldDigestByReference[newImage.Reference]
		}
		deltaManifest.Images = append(deltaManifest.Images, deltaImage)
		deltaManifest.Delta.Summary.DeltaBundleSize += newImage.Size
	}

	return deltaManifest
}

func buildOldDigestMap(diffResult *DiffResult) map[string]string {
	oldDigestByReference := make(map[string]string)
	for _, image := range diffResult.Changed {
		oldDigestByReference[image.Reference] = image.OldDigest
	}
	return oldDigestByReference
}

func writeDeltaBundle(outputPath string, newBundlePath string, deltaManifest *DeltaManifest, includedFiles map[string]ImageStatus) (returnError error) {
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating delta bundle: %w", err)
	}
	defer func() {
		if closeError := outputFile.Close(); closeError != nil && returnError == nil {
			returnError = fmt.Errorf("closing delta bundle: %w", closeError)
		}
	}()

	gzipWriter := gzip.NewWriter(outputFile)
	defer func() {
		if closeError := gzipWriter.Close(); closeError != nil && returnError == nil {
			returnError = fmt.Errorf("closing gzip writer: %w", closeError)
		}
	}()

	tarWriter := tar.NewWriter(gzipWriter)
	defer func() {
		if closeError := tarWriter.Close(); closeError != nil && returnError == nil {
			returnError = fmt.Errorf("closing tar writer: %w", closeError)
		}
	}()

	if err := writeManifestEntry(tarWriter, deltaManifest); err != nil {
		return err
	}

	return streamEntriesFromBundle(tarWriter, newBundlePath, includedFiles)
}

func writeManifestEntry(tarWriter *tar.Writer, deltaManifest *DeltaManifest) error {
	manifestData, err := marshalDeltaManifest(deltaManifest)
	if err != nil {
		return fmt.Errorf("marshaling delta manifest: %w", err)
	}
	header := &tar.Header{
		Name: "manifest.yaml",
		Mode: 0644,
		Size: int64(len(manifestData)),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		return fmt.Errorf("writing manifest header: %w", err)
	}
	if _, err := tarWriter.Write(manifestData); err != nil {
		return fmt.Errorf("writing manifest data: %w", err)
	}
	return nil
}

func streamEntriesFromBundle(tarWriter *tar.Writer, bundlePath string, includedFiles map[string]ImageStatus) error {
	bundleFile, err := os.Open(bundlePath)
	if err != nil {
		return fmt.Errorf("opening source bundle: %w", err)
	}
	defer bundleFile.Close()

	gzipReader, err := gzip.NewReader(bundleFile)
	if err != nil {
		return fmt.Errorf("reading gzip: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar entry: %w", err)
		}

		if shouldIncludeEntry(header.Name, includedFiles) {
			if err := tarWriter.WriteHeader(header); err != nil {
				return fmt.Errorf("writing entry header %s: %w", header.Name, err)
			}
			if _, err := io.Copy(tarWriter, tarReader); err != nil {
				return fmt.Errorf("streaming entry %s: %w", header.Name, err)
			}
		}
	}

	return nil
}

func shouldIncludeEntry(entryName string, includedFiles map[string]ImageStatus) bool {
	if strings.HasPrefix(entryName, "chart/") {
		return true
	}
	_, isIncluded := includedFiles[entryName]
	return isIncluded
}

func computeFileDigest(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("sha256:%x", hasher.Sum(nil)), nil
}

func marshalDeltaManifest(manifest *DeltaManifest) ([]byte, error) {
	return yaml.Marshal(manifest)
}
