package mirror

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMirrorDownloadsTreeFiltersAndConvertsLinks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<!doctype html><html><head><link rel="stylesheet" href="/css/style.css"><script src="/js/app.js"></script></head><body><img src="/img/photo.jpg"><img src="/img/anim.gif"><a href="/page/about.html">About</a><a href="/skip/hidden.html">Skip</a><a href="https://example.org/">External</a></body></html>`)
	})
	mux.HandleFunc("/css/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		_, _ = io.WriteString(w, `@import "/css/more.css"; body{background:url('../img/bg.png')}`)
	})
	mux.HandleFunc("/css/more.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		_, _ = io.WriteString(w, `.x{display:block}`)
	})
	mux.HandleFunc("/page/about.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<img src="../img/about.png"><a href="/">Home</a>`)
	})
	for _, item := range []string{"/img/photo.jpg", "/img/bg.png", "/img/about.png", "/img/anim.gif", "/js/app.js", "/skip/hidden.html"} {
		item := item
		mux.HandleFunc(item, func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, item)
		})
	}
	server := httptest.NewServer(mux)
	defer server.Close()

	base := t.TempDir()
	m := New(server.Client(), io.Discard, Options{
		Reject:       []string{"gif"},
		Exclude:      []string{"/skip"},
		ConvertLinks: true,
		BaseDir:      base,
	})
	if err := m.Run(context.Background(), server.URL+"/"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	rootDir := filepath.Join(base, sanitizeHost(strings.TrimPrefix(server.URL, "http://")))
	for _, rel := range []string{"index.html", "css/style.css", "css/more.css", "js/app.js", "img/photo.jpg", "img/bg.png", "img/about.png", "page/about.html"} {
		if _, err := os.Stat(filepath.Join(rootDir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing mirrored %s: %v", rel, err)
		}
	}
	for _, rel := range []string{"img/anim.gif", "skip/hidden.html"} {
		if _, err := os.Stat(filepath.Join(rootDir, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("filtered file %s unexpectedly exists", rel)
		}
	}

	index, err := os.ReadFile(filepath.Join(rootDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	indexText := string(index)
	for _, want := range []string{`href="css/style.css"`, `src="js/app.js"`, `src="img/photo.jpg"`, `href="page/about.html"`, `href="https://example.org/"`} {
		if !strings.Contains(indexText, want) {
			t.Fatalf("converted index missing %q:\n%s", want, indexText)
		}
	}
	if !strings.Contains(indexText, `/img/anim.gif`) || !strings.Contains(indexText, `/skip/hidden.html`) {
		t.Fatalf("filtered references should remain untouched:\n%s", indexText)
	}

	css, err := os.ReadFile(filepath.Join(rootDir, "css", "style.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), `@import "more.css"`) || !strings.Contains(string(css), `url("../img/bg.png")`) {
		t.Fatalf("CSS links were not converted correctly: %s", css)
	}
}

func TestExcludeMatchesDirectoryBoundary(t *testing.T) {
	m := New(nil, io.Discard, Options{Exclude: []string{"/img"}})
	for _, raw := range []string{"https://example.com/img", "https://example.com/img/a.png"} {
		u := mustURL(t, raw)
		if !m.shouldSkip(u) {
			t.Fatalf("expected %s to be excluded", raw)
		}
	}
	if m.shouldSkip(mustURL(t, "https://example.com/images/a.png")) {
		t.Fatal("/images must not match /img")
	}
}

func TestRejectIsCaseInsensitive(t *testing.T) {
	m := New(nil, io.Discard, Options{Reject: []string{"GIF"}})
	if !m.shouldSkip(mustURL(t, "https://example.com/image.GiF")) {
		t.Fatal("GIF suffix should be rejected case-insensitively")
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
