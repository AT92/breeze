package bundle

// Creator assembles a compressed tar archive from chart and image artifacts.
type Creator interface {
	Create(outputPath string, manifest *Manifest, chartArchive string, imageTarballs map[string]string) error
}

// DefaultCreator implements Creator using archive/tar + compress/gzip.
type DefaultCreator struct{}

func (creator *DefaultCreator) Create(outputPath string, manifest *Manifest, chartArchive string, imageTarballs map[string]string) error {
	return Create(outputPath, manifest, chartArchive, imageTarballs)
}
