package mirror

import (
	"net/url"
	"path/filepath"
	"testing"
)

func TestExtensionlessPathsAreDeterministicAndDirectorySafe(t *testing.T) {
	tests := []struct {
		raw, contentType, want string
	}{
		{"https://example.com/data", "", "data.bin"},
		{"https://example.com/data", "application/x-custom", "data.bin"},
		{"https://example.com/data/x", "image/png", "data/x.png"},
		{"https://example.com/logo", "image/png", "logo.png"},
		{"https://example.com/api", "application/json", "api.json"},
		{"https://example.com/feed", "application/activity+json", "feed.json"},
		{"https://example.com/page", "text/html; charset=utf-8", "page.html"},
	}
	for _, tc := range tests {
		u, err := url.Parse(tc.raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := filepath.ToSlash(localPathFor(u, tc.contentType)); got != tc.want {
			t.Fatalf("localPathFor(%q, %q) = %q, want %q", tc.raw, tc.contentType, got, tc.want)
		}
	}
}
