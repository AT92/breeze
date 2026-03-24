# Breeze

Getting Helm charts into air-gapped environments is tedious. You end up writing fragile shell scripts that call `helm template`, pipe through `grep`, run `docker pull` in a loop, and hope you didn't miss an init container buried in a subchart. When something breaks at the customer site, you get to debug it over a phone call with someone who can't reach the internet.

Breeze packages a Helm chart — with every dependency and every container image — into a single `.tar.gz` you can carry across the air gap.

```bash
breeze bundle ./my-chart/ -f environments/production.yaml -o my-chart-bundle.tar.gz
```

One command. One file. Every image included, every subchart resolved, every private registry override respected.

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Breeze Cloud](https://img.shields.io/badge/Breeze_Cloud-breeze.dev-8B5CF6)](https://breeze.dev)

---

## Why Breeze?

**Instead of bash scripts with `docker save`** — Those scripts break every time a chart adds a conditional image, changes a subchart, or uses a non-standard values structure. Breeze renders Helm templates the same way Helm does, walks the full values tree (including subcharts), and handles edge cases like `lookup()` failures, Bitnami-style registry overrides, and charts that use `version` instead of `tag`. You stop maintaining scripts and start maintaining a single command.

**Instead of Zarf** — Zarf is an air-gap platform that packages entire clusters, including OS-level components, init systems, and its own GitOps agent. If you just need to get a Helm chart and its images across the gap, that's a lot of machinery. Breeze does one thing: it produces a standard tar archive with a Helm chart and OCI image tarballs. No agent, no runtime, no opinions about how you deploy.

**Instead of Hauler** — Hauler requires you to declare your images upfront in a manifest file. Breeze discovers them automatically from your chart templates and values, including subcharts and overrides. If your chart changes, Breeze adapts. You don't maintain a separate image list.

---

## Quick Start

### 1. Bundle your chart

```bash
breeze bundle ./my-chart/ -f environments/production.yaml -o my-chart-bundle.tar.gz
```

Breeze fetches the chart, resolves subchart dependencies, discovers all container images, pulls them (with progress bars), and packages everything into a single archive.

### 2. Transfer and deploy in the air-gapped environment

```bash
# Extract the bundle
tar xzf my-chart-bundle.tar.gz

# Load images into a local Docker daemon
for img in images/*.tar; do
  docker load -i "$img"
done

# Or push to a private registry
for img in images/*.tar; do
  crane push "$img" my-registry.local/$(basename "$img" .tar)
done

# Install the chart
helm install my-release chart/my-chart-1.0.0.tgz -f my-values.yaml
```

### 3. Inspect what's inside

```bash
breeze inspect my-chart-bundle.tar.gz
```

```yaml
apiVersion: breeze/v1
kind: Bundle
metadata:
    name: my-chart
    version: 1.0.0
    breezeVersion: 0.5.0
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

### 4. Compare bundle versions

```bash
breeze diff old-bundle.tar.gz new-bundle.tar.gz
```

See what changed between releases — added, removed, and updated images — and generate a delta bundle containing only the differences.

---

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

### Cross-compile for Linux

```bash
GOOS=linux GOARCH=amd64 go build -o breeze-linux .
```

### Build with version info

```bash
go build -ldflags "-X breeze/internal/version.Version=$(git describe --tags)" -o breeze .
```

---

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

### `breeze diff <old-bundle> <new-bundle>`

Compares two bundle archives and reports image differences. Optionally generates a delta bundle.

**Flags:**

| Flag | Short | Default | Description |
|---|---|---|---|
| `--output` | `-o` | | Generate a delta bundle at this path (only added/changed images) |
| `--output-format` | | `text` | Report format: `text`, `json`, or `yaml` |

**Exit codes:** `0` bundles differ, `1` error, `2` bundles are identical.

### `breeze inspect <bundle.tar.gz>`

Reads a bundle archive and prints its `manifest.yaml` to stdout.

### `breeze version`

Prints the version of breeze.

**Global flags** (available on all commands):

| Flag | Short | Description |
|---|---|---|
| `--quiet` | `-q` | Suppress progress output |
| `--verbose` | `-v` | Show detailed debug output |

---

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

### Compare two bundle versions

```bash
breeze diff v1.0-bundle.tar.gz v1.1-bundle.tar.gz
```

```
Bundle Diff: my-chart 1.0.0 → 1.1.0

Chart:
  version: 1.0.0 → 1.1.0
  appVersion: 1.0.0 → 1.1.0

Images added (1):
  + docker.io/library/redis:7.2                                      (34.2 MB)

Images changed (1):
  ~ docker.io/akcp/web-service:0.20260323.0                          (45.2 MB → 48.1 MB)

Images unchanged (10):
  = 10 images identical (total 312.5 MB)

Summary:
  Total size: 380.0 MB → 394.8 MB (+14.8 MB)
  Delta size: 82.3 MB (images that actually changed)
```

### Generate a delta bundle (only changed images)

```bash
breeze diff v1.0-bundle.tar.gz v1.1-bundle.tar.gz -o delta-v1.0-to-v1.1.tar.gz
```

### Bundle chart only (no images)

```bash
breeze bundle nginx --repo https://charts.bitnami.com/bitnami --skip-images
```

### Bundle for a different platform

```bash
breeze bundle nginx --repo https://charts.bitnami.com/bitnami --platform linux/arm64
```

---

## How Image Discovery Works

Breeze finds images that scripts miss. It doesn't just grep for `image:` fields — it renders templates with your values, walks subchart defaults, handles private registry overrides, and catches non-standard patterns like `version` used as an image tag key.

Three strategies run in sequence:

1. **Template rendering** — Renders chart templates via the Helm SDK with default values plus any `--values`/`--set` overrides. If the full render fails (e.g., templates use `lookup()` for cluster-dependent logic), Breeze falls back to rendering each template individually, skipping only the ones that fail.

2. **Regex scanning** — Scans the rendered Kubernetes manifests for `image:` fields in container and initContainer specs.

3. **Values tree walking** — Recursively walks the merged values tree looking for common image patterns:
   - Bitnami-style: `image.registry` + `image.repository` + `image.tag`
   - Version-style: `repository` + `version` (used by some charts as the tag key)
   - Direct string: `image: "nginx:1.25"`

Values from override files are merged on top of chart defaults before walking, so subchart image overrides (e.g., pointing to a private registry instead of Docker Hub) are correctly resolved. Subchart defaults are merged underneath the parent's overrides, ensuring the parent always wins.

---

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

---

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

---

## Managing bundles across customers?

If you're shipping air-gapped bundles to multiple customers or environments, the CLI handles the packaging — but tracking who has which version, coordinating upgrades, and maintaining audit history across deployments is a different problem. [Breeze Cloud](https://breeze.dev) provides a central dashboard for air-gapped application lifecycle management across your customer fleet.

---

## Contributing

Contributions are welcome. Please open an issue to discuss larger changes before submitting a PR.

## License

Apache License 2.0
