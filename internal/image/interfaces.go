package image

import "context"

// Puller downloads container images and saves them as OCI tarballs.
type Puller interface {
	PullAll(ctx context.Context, references []string, platform string, concurrency int, destinationDir string) ([]PullResult, []error)
}

// DefaultPuller implements Puller using go-containerregistry/crane.
type DefaultPuller struct{}

func (puller *DefaultPuller) PullAll(ctx context.Context, references []string, platform string, concurrency int, destinationDir string) ([]PullResult, []error) {
	return PullAll(ctx, references, platform, concurrency, destinationDir)
}
