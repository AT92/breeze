package chart

import (
	"sort"
	"testing"

	helmchart "helm.sh/helm/v3/pkg/chart"
)

func TestSplitImageNameTag(t *testing.T) {
	tests := []struct {
		input   string
		name    string
		tag     string
	}{
		{"nginx", "nginx", ""},
		{"nginx:1.25", "nginx", "1.25"},
		{"nginx:latest", "nginx", "latest"},
		{"bitnami/nginx:1.25", "bitnami/nginx", "1.25"},
		{"bitnami/nginx", "bitnami/nginx", ""},
		{"docker.io/library/nginx:1.25.3", "docker.io/library/nginx", "1.25.3"},
		{"docker.io/library/nginx", "docker.io/library/nginx", ""},
		{"registry.local:5000/myimage:v1", "registry.local:5000/myimage", "v1"},
		{"registry.local:5000/org/myimage:v1", "registry.local:5000/org/myimage", "v1"},
		{"registry.local:5000/org/myimage", "registry.local:5000/org/myimage", ""},
		{"gcr.io/my-project/app:sha-abc123", "gcr.io/my-project/app", "sha-abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, tag := splitImageNameTag(tt.input)
			if name != tt.name {
				t.Errorf("splitImageNameTag(%q) name = %q, want %q", tt.input, name, tt.name)
			}
			if tag != tt.tag {
				t.Errorf("splitImageNameTag(%q) tag = %q, want %q", tt.input, tag, tt.tag)
			}
		})
	}
}

func TestNormalizeImageRef(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"nginx", "docker.io/library/nginx:latest"},
		{"nginx:1.25", "docker.io/library/nginx:1.25"},
		{"bitnami/nginx", "docker.io/bitnami/nginx:latest"},
		{"bitnami/nginx:1.25", "docker.io/bitnami/nginx:1.25"},
		{"docker.io/library/nginx:1.25", "docker.io/library/nginx:1.25"},
		{"docker.io/library/nginx", "docker.io/library/nginx:latest"},
		{"registry.local:5000/myimage:v1", "registry.local:5000/myimage:v1"},
		{"registry.local:5000/myimage", "registry.local:5000/myimage:latest"},
		{"gcr.io/my-project/app:v2.0", "gcr.io/my-project/app:v2.0"},
		{"quay.io/org/image", "quay.io/org/image:latest"},
		// Digest refs should pass through without adding :latest
		{"nginx@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890", "docker.io/library/nginx@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"},
		{"docker.io/library/nginx@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890", "docker.io/library/nginx@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeImageRef(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeImageRef(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidImageRef(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"nginx:1.25", true},
		{"docker.io/library/nginx:1.25", true},
		{"bitnami/redis:7.2", true},
		{"my-image", true},
		// Invalid cases
		{"", false},
		{"null", false},
		{"true", false},
		{"false", false},
		{"{{ .Values.image }}", false},
		{"has spaces", false},
		{"123456", false},
		// Bare hostnames should be rejected
		{"registry-1.docker.io", false},
		{"docker.io", false},
		{"gcr.io", false},
		// But hostnames with paths are valid
		{"docker.io/library/nginx", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isValidImageRef(tt.input)
			if got != tt.valid {
				t.Errorf("isValidImageRef(%q) = %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}

func TestBuildImageRef(t *testing.T) {
	tests := []struct {
		name   string
		input  map[string]interface{}
		want   string
	}{
		{
			name:  "full bitnami style",
			input: map[string]interface{}{"registry": "docker.io", "repository": "bitnami/nginx", "tag": "1.25.3"},
			want:  "docker.io/bitnami/nginx:1.25.3",
		},
		{
			name:  "no registry",
			input: map[string]interface{}{"repository": "nginx", "tag": "1.25"},
			want:  "nginx:1.25",
		},
		{
			name:  "no tag",
			input: map[string]interface{}{"registry": "docker.io", "repository": "bitnami/nginx"},
			want:  "docker.io/bitnami/nginx",
		},
		{
			name:  "digest",
			input: map[string]interface{}{"registry": "docker.io", "repository": "library/nginx", "digest": "sha256:abc123"},
			want:  "docker.io/library/nginx@sha256:abc123",
		},
		{
			name:  "numeric tag",
			input: map[string]interface{}{"registry": "docker.io", "repository": "bitnami/redis", "tag": 7.2},
			want:  "docker.io/bitnami/redis:7.2",
		},
		{
			name:  "empty repository",
			input: map[string]interface{}{"registry": "docker.io", "repository": ""},
			want:  "",
		},
		{
			name:  "version key as tag fallback",
			input: map[string]interface{}{"repository": "akcp/web-service", "version": "1.20260307.0"},
			want:  "akcp/web-service:1.20260307.0",
		},
		{
			name:  "tag takes precedence over version",
			input: map[string]interface{}{"repository": "nginx", "tag": "1.25", "version": "should-be-ignored"},
			want:  "nginx:1.25",
		},
		{
			name:  "version with registry",
			input: map[string]interface{}{"registry": "docker.io", "repository": "akcp/sp-service", "version": "0.20260223.1"},
			want:  "docker.io/akcp/sp-service:0.20260223.1",
		},
		{
			name:  "digest takes precedence over tag",
			input: map[string]interface{}{"repository": "nginx", "tag": "1.25", "digest": "sha256:abc"},
			want:  "nginx@sha256:abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildImageReference(tt.input)
			if got != tt.want {
				t.Errorf("buildImageReference() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractImagesFromRendered(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name: "qualified image in deployment",
			content: `
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: nginx
          image: docker.io/library/nginx:1.25.3
`,
			want: []string{"docker.io/library/nginx:1.25.3"},
		},
		{
			name: "simple image in container spec",
			content: `
      containers:
        - image: redis:7.2
          name: redis
`,
			want: []string{"redis:7.2"},
		},
		{
			name: "multiple images",
			content: `
      containers:
        - image: registry.example.com/app/frontend:v2.1
          name: frontend
      initContainers:
        - image: registry.example.com/app/migrator:v2.1
          name: migrator
`,
			want: []string{"registry.example.com/app/frontend:v2.1", "registry.example.com/app/migrator:v2.1"},
		},
		{
			name: "image with registry port",
			content: `
      containers:
        - image: myregistry.local:5000/org/myapp:v1
`,
			want: []string{"myregistry.local:5000/org/myapp:v1"},
		},
		{
			name:    "no images in content",
			content: "apiVersion: v1\nkind: ConfigMap\n",
			want:    nil,
		},
		{
			name: "template directive not matched",
			content: `
        - image: {{ .Values.image.repository }}:{{ .Values.image.tag }}
`,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			images := make(map[string]struct{})
			scanRenderedContentForImages(tt.content, images)

			got := make([]string, 0, len(images))
			for img := range images {
				got = append(got, img)
			}
			sort.Strings(got)
			sort.Strings(tt.want)

			if len(got) != len(tt.want) {
				t.Fatalf("got %d images %v, want %d images %v", len(got), got, len(tt.want), tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("image[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestDeduplicateImages(t *testing.T) {
	tests := []struct {
		name           string
		renderedImages map[string]struct{}
		valuesImages   map[string]struct{}
		want           []string
	}{
		{
			name:           "rendered wins over values latest",
			renderedImages: map[string]struct{}{"docker.io/bitnami/nginx:1.25.3": {}},
			valuesImages:   map[string]struct{}{"docker.io/bitnami/nginx": {}},
			want:           []string{"docker.io/bitnami/nginx:1.25.3"},
		},
		{
			name:           "values adds image not in rendered",
			renderedImages: map[string]struct{}{"docker.io/library/nginx:1.25": {}},
			valuesImages:   map[string]struct{}{"docker.io/bitnami/redis:7.2": {}},
			want:           []string{"docker.io/bitnami/redis:7.2", "docker.io/library/nginx:1.25"},
		},
		{
			name:           "values specific tag wins over rendered latest",
			renderedImages: map[string]struct{}{"bitnami/nginx": {}},
			valuesImages:   map[string]struct{}{"docker.io/bitnami/nginx:1.25.3": {}},
			want:           []string{"docker.io/bitnami/nginx:1.25.3"},
		},
		{
			name:           "empty inputs",
			renderedImages: map[string]struct{}{},
			valuesImages:   map[string]struct{}{},
			want:           []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deduplicateImages(tt.renderedImages, tt.valuesImages)
			sort.Strings(got)
			sort.Strings(tt.want)

			if len(got) != len(tt.want) {
				t.Fatalf("got %d images %v, want %d images %v", len(got), got, len(tt.want), tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("image[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestImageNameWithoutTag(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"docker.io/library/nginx:1.25", "docker.io/library/nginx"},
		{"docker.io/library/nginx:latest", "docker.io/library/nginx"},
		{"docker.io/library/nginx", "docker.io/library/nginx"},
		{"registry.local:5000/app:v1", "registry.local:5000/app"},
		{"registry.local:5000/app", "registry.local:5000/app"},
		{"nginx@sha256:abc123", "nginx"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := imageNameWithoutTag(tt.input)
			if got != tt.want {
				t.Errorf("imageNameWithoutTag(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWalkValues(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]interface{}
		want   []string
	}{
		{
			name: "bitnami style nested image",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"registry":   "docker.io",
					"repository": "bitnami/nginx",
					"tag":        "1.25.3",
				},
			},
			want: []string{"docker.io/bitnami/nginx:1.25.3"},
		},
		{
			name: "string image key",
			values: map[string]interface{}{
				"image": "myapp:v1",
			},
			want: []string{"myapp:v1"},
		},
		{
			name: "deeply nested",
			values: map[string]interface{}{
				"subchart": map[string]interface{}{
					"image": map[string]interface{}{
						"repository": "bitnami/redis",
						"tag":        "7.2",
					},
				},
			},
			want: []string{"bitnami/redis:7.2"},
		},
		{
			name: "repository without tag",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"registry":   "docker.io",
					"repository": "library/nginx",
				},
			},
			want: []string{"docker.io/library/nginx"},
		},
		{
			name:   "empty values",
			values: map[string]interface{}{},
			want:   nil,
		},
		{
			name: "repository with version key (not tag)",
			values: map[string]interface{}{
				"webService": map[string]interface{}{
					"repository": "akcp/web-service",
					"version":    "1.20260307.0",
				},
			},
			want: []string{"akcp/web-service:1.20260307.0"},
		},
		{
			name: "non-image repository key ignored",
			values: map[string]interface{}{
				"helm": map[string]interface{}{
					"repository": "",
				},
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			images := make(map[string]struct{})
			walkValuesTree(tt.values, images)

			got := make([]string, 0, len(images))
			for img := range images {
				got = append(got, img)
			}
			sort.Strings(got)
			sort.Strings(tt.want)

			if len(got) != len(tt.want) {
				t.Fatalf("got %d images %v, want %d images %v", len(got), got, len(tt.want), tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("image[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestExtractImages_OverrideTagsMergedWithDefaults(t *testing.T) {
	// Simulate: defaults have repository with empty tag, overrides provide the tag.
	chrt := &helmchart.Chart{
		Metadata: &helmchart.Metadata{Name: "test", Version: "1.0.0"},
		Values: map[string]interface{}{
			"webService": map[string]interface{}{
				"image": map[string]interface{}{
					"registry":   "docker.io",
					"repository": "akcp/web-service",
					"tag":        "",
				},
			},
			"spService": map[string]interface{}{
				"image": map[string]interface{}{
					"registry":   "docker.io",
					"repository": "akcp/sp-service",
					"tag":        "",
				},
			},
		},
	}

	overrides := map[string]interface{}{
		"webService": map[string]interface{}{
			"image": map[string]interface{}{"tag": "2.0.0"},
		},
		"spService": map[string]interface{}{
			"image": map[string]interface{}{"tag": "0.20260223.1"},
		},
	}

	images, err := ExtractImages(chrt, overrides)
	if err != nil {
		t.Fatalf("ExtractImages error: %v", err)
	}

	found := make(map[string]bool)
	for _, img := range images {
		found[img] = true
	}

	if found["docker.io/akcp/web-service:latest"] {
		t.Error("web-service should not be :latest, override provides tag 2.0.0")
	}
	if !found["docker.io/akcp/web-service:2.0.0"] {
		t.Errorf("expected docker.io/akcp/web-service:2.0.0, got: %v", images)
	}
	if found["docker.io/akcp/sp-service:latest"] {
		t.Error("sp-service should not be :latest, override provides tag 0.20260223.1")
	}
	if !found["docker.io/akcp/sp-service:0.20260223.1"] {
		t.Errorf("expected docker.io/akcp/sp-service:0.20260223.1, got: %v", images)
	}
}
