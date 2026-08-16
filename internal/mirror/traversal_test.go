package mirror

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMirrorChildReadFailureDoesNotAbort(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<img src="/broken.bin"><img src="/ok.png">`)
	})
	mux.HandleFunc("/broken.bin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "short")
	})
	mux.HandleFunc("/ok.png", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") })
	server := httptest.NewServer(mux)
	defer server.Close()

	base := t.TempDir()
	var output strings.Builder
	m := New(server.Client(), &output, Options{BaseDir: base})
	if err := m.Run(context.Background(), server.URL+"/"); err != nil {
		t.Fatalf("child failure aborted mirror: %v\n%s", err, output.String())
	}
	root := filepath.Join(base, sanitizeHost(strings.TrimPrefix(server.URL, "http://")))
	if _, err := os.Stat(filepath.Join(root, "ok.png")); err != nil {
		t.Fatalf("healthy sibling missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "broken.bin")); !os.IsNotExist(err) {
		t.Fatalf("partial broken resource left behind: %v", err)
	}
	if !strings.Contains(output.String(), "skip ") {
		t.Fatalf("child error was not reported as skip:\n%s", output.String())
	}
}

func TestMirrorQueryPathsDoNotOverwrite(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<a href="/?page=1">one</a><a href="/?page=2">two</a>`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	base := t.TempDir()
	m := New(server.Client(), io.Discard, Options{BaseDir: base})
	if err := m.Run(context.Background(), server.URL+"/"); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, sanitizeHost(strings.TrimPrefix(server.URL, "http://")))
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var queryFiles int
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "index__q_") && strings.HasSuffix(entry.Name(), ".html") {
			queryFiles++
		}
	}
	if queryFiles != 2 {
		t.Fatalf("query variants = %d, want 2; entries=%v", queryFiles, entries)
	}
}

func TestMirrorRunsConcurrentlyAndDeduplicates(t *testing.T) {
	var duplicateHits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<img src="/r0.png"><img src="/r0.png"><img src="/r1.png"><img src="/r2.png"><img src="/r3.png"><img src="/r4.png"><img src="/r5.png">`)
	})
	for i := 0; i < 6; i++ {
		p := fmt.Sprintf("/r%d.png", i)
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/r0.png" {
				duplicateHits.Add(1)
			}
			time.Sleep(200 * time.Millisecond)
			_, _ = io.WriteString(w, "ok")
		})
	}
	server := httptest.NewServer(mux)
	defer server.Close()
	started := time.Now()
	m := New(server.Client(), io.Discard, Options{BaseDir: t.TempDir(), Workers: 3})
	if err := m.Run(context.Background(), server.URL+"/"); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed >= 900*time.Millisecond {
		t.Fatalf("mirror appears sequential: %v", elapsed)
	}
	if duplicateHits.Load() != 1 {
		t.Fatalf("duplicate URL requested %d times, want 1", duplicateHits.Load())
	}
}

func TestExcludeMatchesDirectoryBoundary(t *testing.T) {
	m := New(nil, io.Discard, Options{Exclude: []string{"/img"}})
	for _, raw := range []string{"https://example.com/img", "https://example.com/img/a.png"} {
		if !m.shouldSkip(mustURL(t, raw)) {
			t.Fatalf("expected %s excluded", raw)
		}
	}
	if m.shouldSkip(mustURL(t, "https://example.com/images/a.png")) {
		t.Fatal("/images must not match /img")
	}
}

func TestRejectIsCaseInsensitive(t *testing.T) {
	m := New(nil, io.Discard, Options{Reject: []string{"GIF"}})
	if !m.shouldSkip(mustURL(t, "https://example.com/image.GiF")) {
		t.Fatal("GIF suffix should be case-insensitive")
	}
}

func TestLocalPathUsesContentTypeForExtensionlessResources(t *testing.T) {
	if got := filepath.ToSlash(localPathFor(mustURL(t, "https://example.com/about"), "text/html; charset=utf-8")); got != "about.html" {
		t.Fatalf("HTML path = %q", got)
	}
	if got := filepath.ToSlash(localPathFor(mustURL(t, "https://example.com/logo"), "image/png")); got != "logo.png" {
		t.Fatalf("PNG path = %q", got)
	}
	if got := filepath.ToSlash(localPathFor(mustURL(t, "https://example.com/api/data"), "application/json")); got != "api/data.json" {
		t.Fatalf("JSON path = %q", got)
	}
}

func TestASCIILowerPreservesHTMLByteOffsets(t *testing.T) {
	input := "İẞK<style>body{background:url('/img/x')}</style><script>document.write(\"<img src='/fake'>\")</script>"
	lowered := asciiLower(input)
	if len(lowered) != len(input) {
		t.Fatalf("asciiLower changed byte length: %d -> %d", len(input), len(lowered))
	}
	m := New(nil, io.Discard, Options{ConvertLinks: true})
	m.allowedHosts["example.com"] = struct{}{}
	img := mustURL(t, "https://example.com/img/x")
	m.paths[canonicalKey(img)] = "img/x.png"
	processed, links, err := m.processHTML([]byte(input), mustURL(t, "https://example.com/"), "index.html", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].Path != "/img/x" {
		t.Fatalf("unexpected links: %#v", links)
	}
	if !strings.Contains(string(processed), `url("img/x.png")`) {
		t.Fatalf("style was not converted correctly: %s", processed)
	}
}

func TestMirrorConvertsExtensionlessImageUsingMIME(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<img src="/logo">`)
	})
	mux.HandleFunc("/logo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	base := t.TempDir()
	m := New(server.Client(), io.Discard, Options{ConvertLinks: true, BaseDir: base, Workers: 2})
	if err := m.Run(context.Background(), server.URL+"/"); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, sanitizeHost(strings.TrimPrefix(server.URL, "http://")))
	index, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), `src="logo.png"`) {
		t.Fatalf("index not converted: %s", index)
	}
	if _, err := os.Stat(filepath.Join(root, "logo.png")); err != nil {
		t.Fatalf("logo.png missing: %v", err)
	}
}

func TestMirrorRateLimitIsSharedAcrossWorkers(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 64*1024)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<img src="/a"><img src="/b"><img src="/c"><img src="/d">`)
	})
	for _, p := range []string{"/a", "/b", "/c", "/d"} {
		p := p
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
			_, _ = w.Write(payload)
		})
	}
	server := httptest.NewServer(mux)
	defer server.Close()
	started := time.Now()
	m := New(server.Client(), io.Discard, Options{BaseDir: t.TempDir(), Workers: 4, RateLimit: 256 * 1024})
	if err := m.Run(context.Background(), server.URL+"/"); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 800*time.Millisecond {
		t.Fatalf("shared mirror rate limit too fast: %v", elapsed)
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
