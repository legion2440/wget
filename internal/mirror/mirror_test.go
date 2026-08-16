package mirror

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMirrorDownloadsRequiredResourcesAndInlineCSS(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<!doctype html><html><head>
<link rel="stylesheet" href="/css/style.css">
<style>.hero{background:url('/img/inline-style.png')}</style>
<script src="/js/not-required.js">document.write("<img src='/img/fake.png'>")</script>
</head><body style="background-image:url('/img/inline-attr.png')">
<img src="/img/photo.jpg"><a href="/about">About</a>
</body></html>`)
	})
	mux.HandleFunc("/css/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		_, _ = io.WriteString(w, `body{background:url('../img/bg.png')}`)
	})
	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<img src="/about/team.png"><a href="/">Home</a>`)
	})
	for _, item := range []string{"/img/photo.jpg", "/img/bg.png", "/img/inline-style.png", "/img/inline-attr.png", "/about/team.png", "/js/not-required.js", "/img/fake.png"} {
		item := item
		mux.HandleFunc(item, func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, item) })
	}
	server := httptest.NewServer(mux)
	defer server.Close()

	base := t.TempDir()
	m := New(server.Client(), io.Discard, Options{ConvertLinks: true, BaseDir: base})
	if err := m.Run(context.Background(), server.URL+"/"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	root := filepath.Join(base, sanitizeHost(strings.TrimPrefix(server.URL, "http://")))
	for _, rel := range []string{"index.html", "css/style.css", "img/photo.jpg", "img/bg.png", "img/inline-style.png", "img/inline-attr.png", "about.html", "about/team.png"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	for _, rel := range []string{"js/not-required.js", "img/fake.png"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("out-of-scope/script-body resource %s unexpectedly exists: %v", rel, err)
		}
	}
	index, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(index)
	for _, want := range []string{"css/style.css", "img/inline-style.png", "img/inline-attr.png", "img/photo.jpg", "about.html"} {
		if !strings.Contains(text, want) {
			t.Fatalf("converted index missing %q:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "js/not-required.js") {
		t.Fatalf("script src should remain in HTML even though it is outside crawl scope:\n%s", text)
	}
}
