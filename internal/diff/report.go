package diff

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// FormatText produces a human-readable diff report.
func FormatText(result *DiffResult) string {
	if result.IsIdentical() {
		return "Bundles are identical.\n"
	}

	var output strings.Builder

	writeHeader(&output, result)
	writeChartChanges(&output, result)
	writeAddedImages(&output, result.Added)
	writeRemovedImages(&output, result.Removed)
	writeChangedImages(&output, result.Changed)
	writeUnchangedImages(&output, result.Unchanged)
	writeSummary(&output, result.Summary)

	return output.String()
}

// FormatJSON produces a JSON diff report.
func FormatJSON(result *DiffResult) (string, error) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling JSON: %w", err)
	}
	return string(data) + "\n", nil
}

// FormatYAML produces a YAML diff report.
func FormatYAML(result *DiffResult) (string, error) {
	data, err := yaml.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshaling YAML: %w", err)
	}
	return string(data), nil
}

func writeHeader(output *strings.Builder, result *DiffResult) {
	fmt.Fprintf(output, "Bundle Diff: %s %s → %s\n\n",
		result.ChartName, result.Chart.OldVersion, result.Chart.NewVersion)
}

func writeChartChanges(output *strings.Builder, result *DiffResult) {
	hasVersionChange := result.Chart.OldVersion != result.Chart.NewVersion
	hasAppVersionChange := result.Chart.OldAppVersion != result.Chart.NewAppVersion

	if !hasVersionChange && !hasAppVersionChange {
		return
	}

	fmt.Fprintln(output, "Chart:")
	if hasVersionChange {
		fmt.Fprintf(output, "  version: %s → %s\n", result.Chart.OldVersion, result.Chart.NewVersion)
	}
	if hasAppVersionChange {
		fmt.Fprintf(output, "  appVersion: %s → %s\n", result.Chart.OldAppVersion, result.Chart.NewAppVersion)
	}
	fmt.Fprintln(output)
}

func writeAddedImages(output *strings.Builder, images []ImageDiff) {
	if len(images) == 0 {
		return
	}
	sortImageDiffsByReference(images)
	fmt.Fprintf(output, "Images added (%d):\n", len(images))
	for _, image := range images {
		fmt.Fprintf(output, "  + %-60s (%s)\n", image.Reference, formatMegabytes(image.NewSize))
	}
	fmt.Fprintln(output)
}

func writeRemovedImages(output *strings.Builder, images []ImageDiff) {
	if len(images) == 0 {
		return
	}
	sortImageDiffsByReference(images)
	fmt.Fprintf(output, "Images removed (%d):\n", len(images))
	for _, image := range images {
		fmt.Fprintf(output, "  - %-60s (%s)\n", image.Reference, formatMegabytes(image.OldSize))
	}
	fmt.Fprintln(output)
}

func writeChangedImages(output *strings.Builder, images []ImageDiff) {
	if len(images) == 0 {
		return
	}
	sortImageDiffsByReference(images)
	fmt.Fprintf(output, "Images changed (%d):\n", len(images))
	for _, image := range images {
		fmt.Fprintf(output, "  ~ %-60s (%s → %s)\n",
			image.Reference, formatMegabytes(image.OldSize), formatMegabytes(image.NewSize))
	}
	fmt.Fprintln(output)
}

func writeUnchangedImages(output *strings.Builder, images []ImageDiff) {
	if len(images) == 0 {
		return
	}
	var totalSize int64
	for _, image := range images {
		totalSize += image.NewSize
	}
	fmt.Fprintf(output, "Images unchanged (%d):\n", len(images))
	fmt.Fprintf(output, "  = %d images identical (total %s)\n\n", len(images), formatMegabytes(totalSize))
}

func writeSummary(output *strings.Builder, summary DiffSummary) {
	sizeDifference := summary.NewTotalSize - summary.OldTotalSize
	sizeSign := "+"
	if sizeDifference < 0 {
		sizeSign = "-"
		sizeDifference = -sizeDifference
	}

	fmt.Fprintln(output, "Summary:")
	fmt.Fprintf(output, "  Total size: %s → %s (%s%s)\n",
		formatMegabytes(summary.OldTotalSize), formatMegabytes(summary.NewTotalSize),
		sizeSign, formatMegabytes(sizeDifference))
	fmt.Fprintf(output, "  Delta size: %s (images that actually changed)\n",
		formatMegabytes(summary.DeltaSize))
}

func formatMegabytes(bytes int64) string {
	megabytes := float64(bytes) / 1024 / 1024
	return fmt.Sprintf("%.1f MB", megabytes)
}

func sortImageDiffsByReference(images []ImageDiff) {
	sort.Slice(images, func(i, j int) bool {
		return images[i].Reference < images[j].Reference
	})
}
