package download

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBatchDownloadsConcurrently(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(220 * time.Millisecond)
		_, _ = io.WriteString(w, r.URL.Path)
	}))
	defer server.Close()

	dir := t.TempDir()
	input := filepath.Join(dir, "downloads.txt")
	urls := []string{server.URL + "/a.bin", server.URL + "/b.bin", server.URL + "/c.bin"}
	if err := os.WriteFile(input, []byte(strings.Join(urls, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	if err := Batch(context.Background(), server.Client(), io.Discard, input, Options{OutputDir: dir}); err != nil {
		t.Fatalf("Batch() error = %v", err)
	}
	elapsed := time.Since(started)
	if elapsed > 550*time.Millisecond {
		t.Fatalf("batch appears sequential: %v", elapsed)
	}
	for _, name := range []string{"a.bin", "b.bin", "c.bin"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}

func TestBatchContinuesOtherDownloadsAfterFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad.bin" {
			http.Error(w, "bad", http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	dir := t.TempDir()
	input := filepath.Join(dir, "downloads.txt")
	content := server.URL + "/good.bin\n" + server.URL + "/bad.bin\n"
	if err := os.WriteFile(input, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Batch(context.Background(), server.Client(), io.Discard, input, Options{OutputDir: dir}); err == nil {
		t.Fatal("expected aggregate error")
	}
	if _, err := os.Stat(filepath.Join(dir, "good.bin")); err != nil {
		t.Fatalf("successful download was lost: %v", err)
	}
}

func TestBatchPrintsSizesFirstWithUnknownChunkedLength(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/known.bin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "3")
		_, _ = w.Write([]byte("abc"))
	})
	mux.HandleFunc("/chunked.bin", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = w.Write([]byte("chunked"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	dir := t.TempDir()
	input := filepath.Join(dir, "downloads.txt")
	if err := os.WriteFile(input, []byte(server.URL+"/known.bin\n"+server.URL+"/chunked.bin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Batch(context.Background(), server.Client(), &out, input, Options{OutputDir: dir}); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.HasPrefix(text, "content size: [3, unknown]\n") {
		t.Fatalf("unexpected output:\n%s", text)
	}
	if strings.Index(text, "content size:") > strings.Index(text, "finished ") {
		t.Fatalf("content sizes must be printed before finished lines:\n%s", text)
	}
}
