package download

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBatchPrintsSizesBeforeSlowBodiesFinish(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "4")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		time.Sleep(300 * time.Millisecond)
		_, _ = io.WriteString(w, "data")
	}))
	defer server.Close()

	dir := t.TempDir()
	input := filepath.Join(dir, "downloads.txt")
	if err := os.WriteFile(input, []byte(server.URL+"/slow.bin\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- Batch(context.Background(), server.Client(), pw, input, Options{OutputDir: dir})
		_ = pw.Close()
	}()

	scanner := bufio.NewScanner(pr)
	lines := make(chan string, 4)
	go func() {
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	select {
	case line := <-lines:
		if line != "content size: [4]" {
			t.Fatalf("first line = %q", line)
		}
	case <-time.After(150 * time.Millisecond):
		t.Fatal("batch produced no size summary while transfer bodies were still running")
	}

	var rest []string
	for line := range lines {
		rest = append(rest, line)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(rest, "\n"), "finished slow.bin") {
		t.Fatalf("missing finish line: %v", rest)
	}
}

func TestBatchSerializesMetadataCallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "2")
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	dir := t.TempDir()
	input := filepath.Join(dir, "downloads.txt")
	urls := []string{
		server.URL + "/a.bin",
		server.URL + "/b.bin",
		server.URL + "/c.bin",
		server.URL + "/d.bin",
	}
	if err := os.WriteFile(input, []byte(strings.Join(urls, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var active atomic.Int32
	var maxActive atomic.Int32
	var calls atomic.Int32
	callback := func(md Metadata) {
		if md.ContentLength != 2 {
			t.Errorf("ContentLength = %d, want 2", md.ContentLength)
		}
		current := active.Add(1)
		calls.Add(1)
		for {
			seen := maxActive.Load()
			if current <= seen || maxActive.CompareAndSwap(seen, current) {
				break
			}
		}
		time.Sleep(40 * time.Millisecond)
		active.Add(-1)
	}

	if err := Batch(context.Background(), server.Client(), io.Discard, input, Options{
		OutputDir:  dir,
		OnMetadata: callback,
	}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != int32(len(urls)) {
		t.Fatalf("metadata callback calls = %d, want %d", calls.Load(), len(urls))
	}
	if maxActive.Load() != 1 {
		t.Fatalf("metadata callback ran concurrently; max active = %d", maxActive.Load())
	}
}
