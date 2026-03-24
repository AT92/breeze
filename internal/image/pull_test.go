package image

import (
	"fmt"
	"testing"
)

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name      string
		errMsg    string
		transient bool
	}{
		// Non-transient errors — should not retry
		{"nil error", "", false},
		{"unauthorized", "UNAUTHORIZED: authentication required", false},
		{"denied", "DENIED: access forbidden", false},
		{"401", "pulling image: GET https://registry/v2/: 401 Unauthorized", false},
		{"403", "pulling image: 403 Forbidden", false},
		{"404 not found", "pulling image: 404 Not Found", false},
		{"name unknown", "pulling image: NAME_UNKNOWN: repository not found", false},
		{"manifest unknown", "MANIFEST_UNKNOWN: manifest not found", false},

		// Transient errors — should retry
		{"timeout", "pulling image: context deadline exceeded (timeout)", true},
		{"429 rate limit", "pulling image: 429 Too Many Requests", true},
		{"500 server error", "pulling image: 500 Internal Server Error", true},
		{"502 bad gateway", "pulling image: 502 Bad Gateway", true},
		{"503 unavailable", "pulling image: 503 Service Unavailable", true},
		{"504 gateway timeout", "pulling image: 504 Gateway Timeout", true},
		{"connection reset", "pulling image: connection reset by peer", true},
		{"connection refused", "dial tcp: connection refused", true},
		{"eof", "pulling image: unexpected EOF", true},
		{"tls handshake", "pulling image: TLS handshake timeout", true},
		{"broken pipe", "write: broken pipe", true},

		// Unknown errors default to transient
		{"unknown error", "something unexpected happened", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.errMsg == "" {
				got := isTransientError(nil)
				if got != false {
					t.Errorf("isTransientError(nil) = %v, want false", got)
				}
				return
			}

			innerErr := fmt.Errorf("%s", tt.errMsg)
			got := isTransientError(innerErr)
			if got != tt.transient {
				t.Errorf("isTransientError(%q) = %v, want %v", tt.errMsg, got, tt.transient)
			}
		})
	}
}

func TestSanitizeRef(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"docker.io/library/nginx:1.25", "docker.io_library_nginx_1.25"},
		{"registry.local:5000/org/app:v1", "registry.local_5000_org_app_v1"},
		{"nginx@sha256:abc123", "nginx_sha256_abc123"},
		{"simple", "simple"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeReference(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeReference(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{54321000, "51.8 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatBytes(tt.input)
			if got != tt.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParsePlatform(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"linux/amd64", false},
		{"linux/arm64", false},
		{"darwin/arm64", false},
		{"invalid", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := parsePlatform(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parsePlatform(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
