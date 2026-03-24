package diff

import "breeze/internal/bundle"

// ImageStatus classifies how an image changed between two bundles.
type ImageStatus string

const (
	ImageAdded     ImageStatus = "added"
	ImageRemoved   ImageStatus = "removed"
	ImageChanged   ImageStatus = "changed"
	ImageUnchanged ImageStatus = "unchanged"
)

// ImageDiff describes a single image's change between old and new bundles.
type ImageDiff struct {
	Reference      string      `json:"reference" yaml:"reference"`
	Status         ImageStatus `json:"status" yaml:"status"`
	OldDigest      string      `json:"oldDigest,omitempty" yaml:"oldDigest,omitempty"`
	NewDigest      string      `json:"newDigest,omitempty" yaml:"newDigest,omitempty"`
	OldSize        int64       `json:"oldSize,omitempty" yaml:"oldSize,omitempty"`
	NewSize        int64       `json:"newSize,omitempty" yaml:"newSize,omitempty"`
	File           string      `json:"file,omitempty" yaml:"file,omitempty"`
}

// ChartDiff describes how the chart metadata changed.
type ChartDiff struct {
	OldVersion    string `json:"oldVersion" yaml:"oldVersion"`
	NewVersion    string `json:"newVersion" yaml:"newVersion"`
	OldAppVersion string `json:"oldAppVersion" yaml:"oldAppVersion"`
	NewAppVersion string `json:"newAppVersion" yaml:"newAppVersion"`
}

// DiffSummary holds aggregate statistics about the diff.
type DiffSummary struct {
	AddedCount     int   `json:"addedCount" yaml:"addedCount"`
	RemovedCount   int   `json:"removedCount" yaml:"removedCount"`
	ChangedCount   int   `json:"changedCount" yaml:"changedCount"`
	UnchangedCount int   `json:"unchangedCount" yaml:"unchangedCount"`
	OldTotalSize   int64 `json:"oldTotalSize" yaml:"oldTotalSize"`
	NewTotalSize   int64 `json:"newTotalSize" yaml:"newTotalSize"`
	DeltaSize      int64 `json:"deltaSize" yaml:"deltaSize"`
}

// DiffResult holds the complete comparison between two bundles.
type DiffResult struct {
	ChartName string      `json:"chartName" yaml:"chartName"`
	Chart     ChartDiff   `json:"chart" yaml:"chart"`
	Added     []ImageDiff `json:"added" yaml:"added"`
	Removed   []ImageDiff `json:"removed" yaml:"removed"`
	Changed   []ImageDiff `json:"changed" yaml:"changed"`
	Unchanged []ImageDiff `json:"unchanged" yaml:"unchanged"`
	Summary   DiffSummary `json:"summary" yaml:"summary"`
}

// IsIdentical returns true if the two bundles have no differences.
func (result *DiffResult) IsIdentical() bool {
	return len(result.Added) == 0 &&
		len(result.Removed) == 0 &&
		len(result.Changed) == 0 &&
		result.Chart.OldVersion == result.Chart.NewVersion &&
		result.Chart.OldAppVersion == result.Chart.NewAppVersion
}

// CompareManifests compares two bundle manifests and produces a DiffResult.
func CompareManifests(oldManifest, newManifest *bundle.Manifest) *DiffResult {
	oldImagesByReference := indexImagesByReference(oldManifest.Images)
	newImagesByReference := indexImagesByReference(newManifest.Images)

	result := &DiffResult{
		ChartName: newManifest.Chart.Name,
		Chart: ChartDiff{
			OldVersion:    oldManifest.Chart.Version,
			NewVersion:    newManifest.Chart.Version,
			OldAppVersion: oldManifest.Chart.AppVersion,
			NewAppVersion: newManifest.Chart.AppVersion,
		},
	}

	classifyNewImages(result, newImagesByReference, oldImagesByReference)
	classifyRemovedImages(result, oldImagesByReference, newImagesByReference)
	computeSummary(result, oldManifest, newManifest)

	return result
}

func classifyNewImages(result *DiffResult, newImages, oldImages map[string]bundle.ImageInfo) {
	for reference, newImage := range newImages {
		oldImage, existsInOld := oldImages[reference]
		if !existsInOld {
			result.Added = append(result.Added, ImageDiff{
				Reference: reference,
				Status:    ImageAdded,
				NewDigest: newImage.Digest,
				NewSize:   newImage.Size,
				File:      newImage.File,
			})
		} else if oldImage.Digest != newImage.Digest {
			result.Changed = append(result.Changed, ImageDiff{
				Reference: reference,
				Status:    ImageChanged,
				OldDigest: oldImage.Digest,
				NewDigest: newImage.Digest,
				OldSize:   oldImage.Size,
				NewSize:   newImage.Size,
				File:      newImage.File,
			})
		} else {
			result.Unchanged = append(result.Unchanged, ImageDiff{
				Reference: reference,
				Status:    ImageUnchanged,
				NewDigest: newImage.Digest,
				NewSize:   newImage.Size,
			})
		}
	}
}

func classifyRemovedImages(result *DiffResult, oldImages, newImages map[string]bundle.ImageInfo) {
	for reference, oldImage := range oldImages {
		if _, existsInNew := newImages[reference]; !existsInNew {
			result.Removed = append(result.Removed, ImageDiff{
				Reference: reference,
				Status:    ImageRemoved,
				OldDigest: oldImage.Digest,
				OldSize:   oldImage.Size,
			})
		}
	}
}

func computeSummary(result *DiffResult, oldManifest, newManifest *bundle.Manifest) {
	result.Summary.AddedCount = len(result.Added)
	result.Summary.RemovedCount = len(result.Removed)
	result.Summary.ChangedCount = len(result.Changed)
	result.Summary.UnchangedCount = len(result.Unchanged)

	for _, image := range oldManifest.Images {
		result.Summary.OldTotalSize += image.Size
	}
	for _, image := range newManifest.Images {
		result.Summary.NewTotalSize += image.Size
	}

	for _, image := range result.Added {
		result.Summary.DeltaSize += image.NewSize
	}
	for _, image := range result.Changed {
		result.Summary.DeltaSize += image.NewSize
	}
}

func indexImagesByReference(images []bundle.ImageInfo) map[string]bundle.ImageInfo {
	index := make(map[string]bundle.ImageInfo, len(images))
	for _, image := range images {
		index[image.Reference] = image
	}
	return index
}
