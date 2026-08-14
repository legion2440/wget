package download

import (
	"bytes"
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

func TestFetchDownloadsAndRenamesFile(t *testing.T) {
	payload := bytes.Repeat([]byte("abc123"), 1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "6144")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	dir := t.TempDir()
	var output bytes.Buffer
	d := New(server.Client(), &output)
	result, err := d.Fetch(context.Background(), server.URL+"/original.bin", Options{
		OutputName:   "renamed.bin",
		OutputDir:    dir,
		ShowProgress: true,
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Bytes != int64(len(payload)) {
		t.Fatalf("Bytes = %d, want %d", result.Bytes, len(payload))
	}
	got, err := os.ReadFile(filepath.Join(dir, "renamed.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("downloaded content differs")
	}
	for _, want := range []string{"start at ", "status 200 OK", "content size: 6144", "saving file to:", "100.00%", "Downloaded [", "finished at "} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestFetchFollowsRedirectAndUsesFinalFilename(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final.txt", http.StatusFound)
	})
	mux.HandleFunc("/final.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "done")
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	dir := t.TempDir()
	d := New(server.Client(), io.Discard)
	result, err := d.Fetch(context.Background(), server.URL+"/start", Options{OutputDir: dir})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if filepath.Base(result.Path) != "final.txt" {
		t.Fatalf("saved %q, want final.txt", result.Path)
	}
}

func TestFetchRejectsHTTPErrorWithoutCreatingFile(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	dir := t.TempDir()

	d := New(server.Client(), io.Discard)
	if _, err := d.Fetch(context.Background(), server.URL+"/missing.bin", Options{OutputDir: dir}); err == nil {
		t.Fatal("expected HTTP error")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("unexpected files after failed download: %v", entries)
	}
}

func TestRateLimitParser(t *testing.T) {
	tests := map[string]int64{
		"300k": 300 * 1024,
		"700K": 700 * 1024,
		"2M":   2 * 1024 * 1024,
		"1.5M": int64(1.5 * 1024 * 1024),
	}
	for input, want := range tests {
		got, err := ParseRate(input)
		if err != nil {
			t.Fatalf("ParseRate(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseRate(%q) = %d, want %d", input, got, want)
		}
	}
	for _, input := range []string{"0", "-1k", "abc"} {
		if _, err := ParseRate(input); err == nil {
			t.Fatalf("ParseRate(%q) expected error", input)
		}
	}
}

func TestRateLimitedReaderPacesReturnedBytes(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 8*1024)
	reader := newRateLimitedReader(bytes.NewReader(payload), 64*1024)
	started := time.Now()
	if _, err := io.Copy(io.Discard, reader); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(started)
	if elapsed < 100*time.Millisecond {
		t.Fatalf("rate limiter completed too quickly: %v", elapsed)
	}
}
