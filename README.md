# Breeze

Breeze is a CLI tool that bundles Helm charts for air-gapped environments. It downloads a Helm chart with all its dependencies, pulls all referenced container images, and packages everything into a single compressed `.tar.gz` archive with a `manifest.yaml` describing the contents.

No Docker daemon required — Breeze pulls images directly from container registries using your existing Docker credentials.

## Installation

### Build from source

Requires Go 1.25+.

```bash
git clone <repo-url>
cd breeze
go build -o breeze .
```

Optionally move the binary to your PATH:

```bash
cp breeze /usr/local/bin/
```

### Build with version info

```bash
go build -ldflags "-X breeze/internal/version.Version=$(git describe --tags)" -o breeze .
```

## Quick Start

Bundle a remote chart:

```bash
breeze bundle nginx --repo https://charts.bitnami.com/bitnami -o nginx-bundle.tar.gz
```

Bundle a local chart with environment-specific values:

```bash
breeze bundle ./my-chart/ -f environments/production.yaml
```

Preview which images would be downloaded (no download, no bundle):

```bash
breeze bundle ./my-chart/ -f environments/production.yaml --dry-run
```

Inspect an existing bundle:

```bash
breeze inspect nginx-bundle.tar.gz
```

## Commands

### `breeze bundle <chart-ref>`

Downloads a Helm chart, resolves all subchart dependencies, extracts container image references, pulls all images, and creates a compressed bundle.

The chart-ref can be:
- A local directory: `breeze bundle ./my-chart/`
- A local `.tgz` file: `breeze bundle my-chart-1.0.0.tgz`
- A remote chart: `breeze bundle nginx --repo https://charts.bitnami.com/bitnami`

**Flags:**

| Flag | Short | Default | Description |
|---|---|---|---|
| `--repo` | | | Helm repository URL (required for remote charts) |
| `--version` | | latest | Chart version to download |
| `--values` | `-f` | | Values file(s) for template rendering (can be repeated) |
| `--set` | | | Set values on the command line (`key=value`, can be repeated) |
| `--output` | `-o` | `<chart>-<version>-bundle.tar.gz` | Output file path |
| `--platform` | | `linux/amd64` | Target platform for container images |
| `--skip-images` | | `false` | Bundle the chart only, skip image downloading |
| `--dry-run` | | `false` | List discovered images without downloading or creating a bundle |
| `--concurrency` | | `4` | Number of parallel image downloads |

**Global flags** (available on all commands):

| Flag | Short | Description |
|---|---|---|
| `--quiet` | `-q` | Suppress progress output |
| `--verbose` | `-v` | Show detailed debug output |

### `breeze inspect <bundle.tar.gz>`

Reads a bundle archive and prints its `manifest.yaml` to stdout.

### `breeze version`

Prints the version of breeze.

## Examples

### Bundle a remote chart with a specific version

```bash
breeze bundle nginx \
  --repo https://charts.bitnami.com/bitnami \
  --version 18.3.1 \
  -o nginx-18.3.1-bundle.tar.gz
```

### Bundle a local chart with environment overrides

Values files override image tags, enable optional components, and configure subchart images. Breeze merges them with chart defaults so that overridden image references (e.g., private registries) are correctly resolved:

```bash
breeze bundle ./my-chart/ \
  -f environments/production.yaml \
  --set monitoring.enabled=true
```

### Preview images without downloading (dry run)

```bash
breeze bundle ./my-chart/ -f environments/production.yaml --dry-run
```

Output:

```
Fetching chart...
Fetched my-chart (version 1.0.0)
Extracting image references...
Found 12 image(s):
  - docker.io/akcp/web-service:0.20260323.0
  - docker.io/akcp/sp-service:1.20260307.0
  - docker.io/bitnami/kafka:3.9.0-debian-12-r12
  ...
Dry run: skipping image download and bundle creation
```

### Bundle chart only (no images)

```bash
breeze bundle nginx \
  --repo https://charts.bitnami.com/bitnami \
  --skip-images
```

### Bundle for a different platform

```bash
breeze bundle nginx \
  --repo https://charts.bitnami.com/bitnami \
  --platform linux/arm64
```

### Inspect a bundle

```bash
breeze inspect my-chart-1.0.0-bundle.tar.gz
```

Output:

```yaml
apiVersion: breeze/v1
kind: Bundle
metadata:
    name: my-chart
    version: 1.0.0
    createdAt: 2026-03-24T10:00:00Z
    breezeVersion: 0.4.0
chart:
    name: my-chart
    version: 1.0.0
    appVersion: 2.0.0
    file: chart/my-chart-1.0.0.tgz
    dependencies:
        - name: kafka
          version: 31.5.0
          repository: oci://registry-1.docker.io/bitnamicharts
images:
    - reference: docker.io/akcp/web-service:0.20260323.0
      digest: sha256:72bbb777e394036ad2e765e15845df3...
      platform: linux/amd64
      file: images/docker.io_akcp_web-service_0.20260323.0.tar
      size: 43615232
```

### Use a bundle in an air-gapped environment

On the target machine, extract the bundle and load the images:

```bash
# Extract the bundle
tar xzf my-chart-1.0.0-bundle.tar.gz

# Load images into a local Docker daemon
for img in images/*.tar; do
  docker load -i "$img"
done

# Or push to a private registry using crane
for img in images/*.tar; do
  crane push "$img" my-registry.local/$(basename "$img" .tar)
done

# Install the chart
helm install my-release chart/my-chart-1.0.0.tgz -f my-values.yaml
```

## Bundle Structure

```
my-chart-1.0.0-bundle.tar.gz
├── manifest.yaml              # Bundle metadata and artifact inventory
├── chart/
│   └── my-chart-1.0.0.tgz    # Packaged Helm chart (with dependencies)
└── images/
    ├── docker.io_akcp_web-service_0.20260323.0.tar
    └── docker.io_bitnami_kafka_3.9.0-debian-12-r12.tar
```

## How Image Discovery Works

Breeze uses three strategies to find all container images referenced in a chart:

1. **Template rendering** — Renders chart templates via the Helm SDK with default values plus any `--values`/`--set` overrides. If the full render fails (e.g., templates use `lookup()` for cluster-dependent logic), Breeze falls back to rendering each template individually, skipping the ones that fail.

2. **Regex scanning** — Scans the rendered Kubernetes manifests for `image:` fields in container and initContainer specs.

3. **Values tree walking** — Recursively walks the merged values tree looking for common image patterns:
   - Bitnami-style: `image.registry` + `image.repository` + `image.tag`
   - Version-style: `repository` + `version` (used by some charts as the tag key)
   - Direct string: `image: "nginx:1.25"`

Values from override files are merged on top of chart defaults before walking, so subchart image overrides (e.g., pointing to a private registry) are correctly resolved. Subchart defaults are merged underneath the parent's overrides, ensuring the parent always wins.

## Behavior Details

### Partial bundles

If some images fail to download (e.g., a tag no longer exists in the registry), Breeze still creates the bundle with the images that succeeded. A warning is printed listing the failed images. The manifest only includes successfully pulled images.

### Cancellation

Pressing Ctrl+C at any point aborts the operation cleanly. In-flight image downloads are cancelled, and any partial output files are deleted. The temp working directory is always cleaned up.

### Authentication

Breeze uses your existing Docker credentials (`~/.docker/config.json`) for pulling images from private registries. If you can `docker pull` an image, Breeze can pull it too. Credential helpers (ECR, GCR, ACR) are supported via the default Docker keychain.

### Progress display

During image downloads, Breeze shows a multi-line progress display with one line per concurrent download:

```
  docker.io/akcp/web-service:0.20260323.0             [========>           ] 45.2 MB / 98.3 MB (46%)
  docker.io/bitnami/kafka:3.9.0-debian-12-r12         [====>               ] 12.1 MB / 55.0 MB (22%)
  docker.io/akcp/debug-utils:1.0.1                    [======>             ]  3.2 MB / 10.5 MB (30%)
  docker.io/akcp/sp-service:1.20260307.0              [===============>    ] 88.0 MB / 120.0 MB (73%)
```

Use `--quiet` to suppress all progress output (useful in CI/CD pipelines).
