package bundle

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
)

// ReadManifest extracts and parses the manifest.yaml from a breeze bundle archive.
func ReadManifest(bundlePath string) (*Manifest, error) {
	bundleFile, err := os.Open(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("opening bundle: %w", err)
	}
	defer bundleFile.Close()

	gzipReader, err := gzip.NewReader(bundleFile)
	if err != nil {
		return nil, fmt.Errorf("reading gzip: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar: %w", err)
		}
		if header.Name == "manifest.yaml" {
			const maxManifestSize = 10 * 1024 * 1024
			data, err := io.ReadAll(io.LimitReader(tarReader, maxManifestSize))
			if err != nil {
				return nil, fmt.Errorf("reading manifest: %w", err)
			}
			return UnmarshalManifest(data)
		}
	}
	return nil, fmt.Errorf("manifest.yaml not found in bundle")
}
