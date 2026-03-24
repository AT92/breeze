package image

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// PullResult holds metadata about a successfully pulled image.
type PullResult struct {
	Reference string
	Digest    string
	File      string
	Size      int64
}

// PullError wraps an error with the image reference that caused it.
type PullError struct {
	Reference string
	Err       error
}

func (pullError *PullError) Error() string {
	return fmt.Sprintf("pulling %s: %v", pullError.Reference, pullError.Err)
}

// PullAll downloads all images concurrently and saves them as OCI tarballs.
func PullAll(ctx context.Context, references []string, platform string, concurrency int, destinationDir string) ([]PullResult, []error) {
	if err := os.MkdirAll(destinationDir, 0755); err != nil {
		return nil, []error{fmt.Errorf("creating image dir: %w", err)}
	}

	platformOption, err := parsePlatform(platform)
	if err != nil {
		return nil, []error{fmt.Errorf("parsing platform: %w", err)}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	display := newProgressDisplay(concurrency)
	display.start()
	defer display.stop()

	var (
		mutex      sync.Mutex
		results    []PullResult
		errors     []error
		waitGroup  sync.WaitGroup
		semaphore  = make(chan struct{}, concurrency)
	)

	for _, reference := range references {
		waitGroup.Add(1)

		select {
		case <-ctx.Done():
			waitGroup.Done()
			continue
		case semaphore <- struct{}{}:
		}

		go func(reference string) {
			defer waitGroup.Done()
			defer func() { <-semaphore }()

			result, err := pullWithRetry(ctx, reference, platformOption, destinationDir, 3, display)
			mutex.Lock()
			defer mutex.Unlock()
			if err != nil {
				errors = append(errors, &PullError{Reference: reference, Err: err})
			} else {
				results = append(results, *result)
			}
		}(reference)
	}

	waitGroup.Wait()
	return results, errors
}

func pullWithRetry(ctx context.Context, reference string, platformOption crane.Option, destinationDir string, maxRetries int, display *progressDisplay) (*PullResult, error) {
	var lastError error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			retryDelay := time.Duration(1<<uint(attempt)) * time.Second
			display.logMessage("  Retrying %s (attempt %d/%d) after %v...", reference, attempt+1, maxRetries, retryDelay)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDelay):
			}
		}

		result, err := pullAndSaveImage(ctx, reference, platformOption, destinationDir, display)
		if err == nil {
			return result, nil
		}
		lastError = err

		if ctx.Err() != nil || !isTransientError(err) {
			return nil, lastError
		}
	}
	return nil, lastError
}

func pullAndSaveImage(ctx context.Context, reference string, platformOption crane.Option, destinationDir string, display *progressDisplay) (*PullResult, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	slotIndex := display.allocSlot(reference)
	defer display.freeSlot(slotIndex)
	display.updateSlot(slotIndex, reference, "pulling...", 0, 0)

	image, err := crane.Pull(reference, platformOption, crane.WithAuthFromKeychain(authn.DefaultKeychain), crane.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("pulling image: %w", err)
	}

	digest, err := image.Digest()
	if err != nil {
		return nil, fmt.Errorf("getting digest: %w", err)
	}

	compressedSize := computeImageSize(image)
	display.updateSlot(slotIndex, reference, "saving...", 0, compressedSize)

	fileName := sanitizeReference(reference) + ".tar"
	filePath := filepath.Join(destinationDir, fileName)

	if err := saveImageToFile(image, reference, filePath, slotIndex, compressedSize, display); err != nil {
		return nil, fmt.Errorf("saving image: %w", err)
	}

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("stat image file: %w", err)
	}

	display.logMessage("  Done %s (%s)", reference, formatBytes(fileInfo.Size()))

	return &PullResult{
		Reference: reference,
		Digest:    digest.String(),
		File:      fileName,
		Size:      fileInfo.Size(),
	}, nil
}

func saveImageToFile(image v1.Image, reference string, filePath string, slotIndex int, totalSize int64, display *progressDisplay) error {
	outputFile, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	writer := &trackingWriter{
		destination: outputFile,
		slotIndex:   slotIndex,
		totalBytes:  totalSize,
		display:     display,
	}

	parsedReference, err := name.ParseReference(reference)
	if err != nil {
		return fmt.Errorf("parsing reference: %w", err)
	}

	return tarball.Write(parsedReference, image, writer)
}

func computeImageSize(image v1.Image) int64 {
	layers, err := image.Layers()
	if err != nil {
		return 0
	}
	var totalSize int64
	for _, layer := range layers {
		layerSize, err := layer.Size()
		if err != nil {
			return 0
		}
		totalSize += layerSize
	}
	return totalSize
}

func parsePlatform(platform string) (crane.Option, error) {
	parts := strings.SplitN(platform, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid platform format %q, expected os/arch (e.g., linux/amd64)", platform)
	}
	return crane.WithPlatform(&v1.Platform{
		OS:           parts[0],
		Architecture: parts[1],
	}), nil
}

func sanitizeReference(reference string) string {
	replacer := strings.NewReplacer("/", "_", ":", "_", "@", "_")
	return replacer.Replace(reference)
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	divisor, exponent := int64(unit), 0
	for remaining := bytes / unit; remaining >= unit; remaining /= unit {
		divisor *= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(divisor), "KMGTPE"[exponent])
}
