# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go build -o breeze .
go run .

# With version override:
go build -ldflags "-X breeze/internal/version.Version=$(git describe --tags 2>/dev/null || echo dev)" -o breeze .

# Run tests:
go test ./...
```

## Project Overview

Go module named `breeze` — a CLI tool for bundling Helm charts for air-gapped environments. It downloads charts with dependencies, extracts container image references, pulls all images, and packages everything into a compressed tar archive with a manifest. It can also compare two bundles and generate delta bundles containing only changed artifacts.

## Package Structure

```
cmd/                           # Cobra CLI commands (thin flag parsers)
  root.go                      # Root command, signal handling, --quiet/--verbose flags
  bundle.go                    # bundle subcommand, delegates to internal/bundler
  inspect.go                   # inspect subcommand
  diff.go                      # diff subcommand, compares bundles + generates delta bundles
  version.go                   # version subcommand

internal/bundler/              # Pipeline orchestration
  bundler.go                   # Bundler struct with injected dependencies, Run() pipeline

internal/chart/                # Helm chart operations
  fetch.go                     # Chart downloading (local/remote), dependency resolution
  images.go                    # Image extraction orchestration, template rendering
  normalize.go                 # Image reference normalization (NormalizeImageRef, splitImageNameTag)
  values.go                    # Values tree walking for image discovery (buildImageReference)
  interfaces.go                # Fetcher + ImageExtractor interfaces

internal/image/                # Container image operations
  pull.go                      # Concurrent image pulling with retry
  progress.go                  # Terminal progress display (multi-line, per-image bars)
  errors.go                    # Transient vs non-transient error classification
  interfaces.go                # Puller interface

internal/bundle/               # Bundle archive operations
  archive.go                   # tar.gz creation
  manifest.go                  # Manifest types + YAML serialization
  reader.go                    # Bundle reading for inspect command
  interfaces.go                # Creator interface

internal/diff/                 # Bundle comparison and delta generation
  compare.go                   # CompareManifests — classifies images as added/removed/changed/unchanged
  report.go                    # FormatText, FormatJSON, FormatYAML — diff report formatting
  delta.go                     # GenerateDeltaBundle — streams changed artifacts into a delta archive

internal/log/                  # Leveled logger (LevelQuiet, LevelNormal, LevelVerbose)
  logger.go

internal/version/              # Single version source, overridable via ldflags
  version.go
```

## Architecture

- **Interfaces at all boundaries**: `chart.Fetcher`, `chart.ImageExtractor`, `image.Puller`, `bundle.Creator` — all consumed by `bundler.Bundler` and fakeable in tests
- **Pipeline in `bundler.Run()`**: fetch chart -> extract images -> pull images -> create bundle
- **Cancellation**: Ctrl+C propagates via context through all steps; partial output files are cleaned up
- **Partial bundles**: If some images fail to pull, the bundle is still created with the successful images (warning printed)
- **Template rendering**: Tries full render first, falls back to per-template rendering on failure (handles charts using `lookup()`)
- **Diff and delta**: Compares two bundle manifests by image digest, outputs text/JSON/YAML reports, and can generate delta bundles containing only added/changed images streamed directly from the source archive (no disk extraction)

## Image Extraction Strategy

Three-layer approach to find container image references:

1. **Template rendering** — Renders chart templates via Helm SDK, falls back to per-template rendering if full render fails
2. **Regex scanning** — Scans rendered Kubernetes manifests for `image:` fields
3. **Values tree walking** — Walks merged values for `{repository, tag/version}` patterns (supports both `tag` and `version` as tag keys)

Values from override files (`-f`) are merged on top of defaults using `chartutil.CoalesceTables` so that subchart image overrides (e.g., private registries) are respected.

## Bundle Formats

### Regular Bundle (`kind: Bundle`)
Contains `manifest.yaml`, `chart/<name>.tgz`, and `images/*.tar`. Created by `breeze bundle`.

### Delta Bundle (`kind: DeltaBundle`)
Contains only added/changed images from a newer bundle compared to an older baseline. Created by `breeze diff --output`. Includes `delta.baselineVersion` and `delta.baselineDigest` for traceability. Images are streamed directly from the source bundle to avoid full extraction to disk.

## Key Dependencies

- `helm.sh/helm/v3` — Helm SDK for chart operations
- `github.com/google/go-containerregistry` — Docker image pulling without daemon (crane)
- `github.com/spf13/cobra` — CLI framework
- `gopkg.in/yaml.v3` — YAML marshaling for manifest

## Code Conventions

- No boolean parameters in functions — use typed constants (e.g., `log.Level`)
- No abbreviated variable names — use descriptive names (e.g., `helmChart` not `chrt`)
- Files should stay under ~200 lines; split by concern when they grow
- Methods should be short and focused; extract helpers for distinct steps
- All progress/status output goes to stderr via `log.Logger`; data output (inspect, version, diff) goes to stdout
- Exit codes: 0 = success/differences found, 1 = error, 2 = bundles identical (diff command only)
- `cmd/` files should be thin flag parsers that delegate to `internal/` packages

## Known Issues

Review files in `tmp/` track current bugs and architecture improvements:
- `tmp/bugs.md` — potential bugs including `os.Exit` in diff command, sort mutation, size formatting inconsistency
- `tmp/architecture-improvements.md` — structural improvements including shared tar utilities, diff orchestration extraction, unified size formatting
