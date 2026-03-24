package bundle

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Create assembles a compressed tar.gz bundle from a manifest, chart archive, and image tarballs.
func Create(outputPath string, manifest *Manifest, chartArchive string, imageTarballs map[string]string) (returnError error) {
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer func() {
		if closeError := outputFile.Close(); closeError != nil && returnError == nil {
			returnError = fmt.Errorf("closing output file: %w", closeError)
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

	manifestData, err := manifest.Marshal()
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	if err := writeDataToTar(tarWriter, "manifest.yaml", manifestData); err != nil {
		return fmt.Errorf("writing manifest to archive: %w", err)
	}

	chartFileName := filepath.Base(chartArchive)
	if err := addFileToTar(tarWriter, "chart/"+chartFileName, chartArchive); err != nil {
		return fmt.Errorf("adding chart to archive: %w", err)
	}

	for archiveName, localPath := range imageTarballs {
		if err := addFileToTar(tarWriter, "images/"+archiveName, localPath); err != nil {
			return fmt.Errorf("adding image %s to archive: %w", archiveName, err)
		}
	}

	return nil
}

func writeDataToTar(tarWriter *tar.Writer, name string, data []byte) error {
	header := &tar.Header{
		Name: name,
		Mode: 0644,
		Size: int64(len(data)),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}
	_, err := tarWriter.Write(data)
	return err
}

func addFileToTar(tarWriter *tar.Writer, name string, sourcePath string) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	fileInfo, err := sourceFile.Stat()
	if err != nil {
		return err
	}

	header := &tar.Header{
		Name: name,
		Mode: 0644,
		Size: fileInfo.Size(),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	_, err = io.Copy(tarWriter, sourceFile)
	return err
}
