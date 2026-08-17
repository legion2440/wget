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
