package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var auditBinary string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "wget-audit-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "audit setup:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)

	name := "wget"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	auditBinary = filepath.Join(tmp, name)
	cmd := exec.Command("go", "build", "-o", auditBinary, ".")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "audit setup: go build failed: %v\n%s", err, output)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestAuditSingleDownloadOutput(t *testing.T) {
	server, payload := newAuditServer(t)
	dir := t.TempDir()
	output, err := runAuditCLI(dir, server.URL+"/file.jpg")
	if err != nil {
		t.Fatalf("wget failed: %v\n%s", err, output)
	}

	assertFileEquals(t, filepath.Join(dir, "file.jpg"), payload)
	for _, want := range []string{
		"start at ",
		"sending request, awaiting response... status 200 OK",
		fmt.Sprintf("content size: %d", len(payload)),
		"saving file to:",
		"100.00%",
		"Downloaded [",
		"finished at ",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output does not contain %q:\n%s", want, output)
		}
	}
	assertTimestampLine(t, output, "start at ")
	assertTimestampLine(t, output, "finished at ")
}

func TestAuditOutputNameAndDirectory(t *testing.T) {
	server, payload := newAuditServer(t)
	dir := t.TempDir()
	output, err := runAuditCLI(dir, "-O=test_20MB.zip", "-P=downloads", server.URL+"/file.jpg")
	if err != nil {
		t.Fatalf("wget failed: %v\n%s", err, output)
	}
	assertFileEquals(t, filepath.Join(dir, "downloads", "test_20MB.zip"), payload)
}

func TestAuditRateLimit(t *testing.T) {
	server, _ := newAuditServer(t)
	dir := t.TempDir()
	started := time.Now()
	output, err := runAuditCLI(dir, "--rate-limit=300k", server.URL+"/rate.bin")
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("wget failed: %v\n%s", err, output)
	}
	// /rate.bin is 300 KiB. At 300 KiB/s the transfer should take about one second.
	// A generous lower bound catches an ignored/broken limiter without making the
	// test sensitive to scheduler jitter.
	if elapsed < 800*time.Millisecond {
		t.Fatalf("300k rate limit was not respected: transfer finished in %v", elapsed)
	}
}

func TestAuditBatchDownloadsConcurrently(t *testing.T) {
	server, _ := newAuditServer(t)
	dir := t.TempDir()
	input := strings.Join([]string{
		server.URL + "/batch-a.zip",
		server.URL + "/batch-b.zip",
		server.URL + "/batch-c.zip",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "downloads.txt"), []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	output, err := runAuditCLI(dir, "-i=downloads.txt")
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("wget failed: %v\n%s", err, output)
	}
	for _, name := range []string{"batch-a.zip", "batch-b.zip", "batch-c.zip"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing batch download %s: %v", name, err)
		}
	}
	// Each endpoint sleeps for 300 ms before responding. Sequential downloads
	// therefore take at least ~900 ms; concurrent downloads should stay well below it.
	if elapsed >= 800*time.Millisecond {
		t.Fatalf("batch downloads do not appear concurrent: %v", elapsed)
	}
}

func TestAuditBackgroundDownloadAndLog(t *testing.T) {
	server, payload := newAuditServer(t)
	dir := t.TempDir()
	output, err := runAuditCLI(dir, "-B", server.URL+"/background.bin")
	if err != nil {
		t.Fatalf("background launcher failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, `Output will be written to "wget-log".`) {
		t.Fatalf("unexpected background launcher output:\n%s", output)
	}

	deadline := time.Now().Add(5 * time.Second)
	var logData []byte
	for time.Now().Before(deadline) {
		data, logErr := os.ReadFile(filepath.Join(dir, "wget-log"))
		fileData, fileErr := os.ReadFile(filepath.Join(dir, "background.bin"))
		if logErr == nil && fileErr == nil && bytes.Equal(fileData, payload) && strings.Contains(string(data), "finished at ") {
			logData = data
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if len(logData) == 0 {
		t.Fatal("background download did not finish with wget-log within 5 seconds")
	}
	logText := string(logData)
	for _, want := range []string{"start at ", "status 200 OK", "content size:", "saving file to:", "Downloaded [", "finished at "} {
		if !strings.Contains(logText, want) {
			t.Fatalf("wget-log does not contain %q:\n%s", want, logText)
		}
	}
}

func TestAuditHTTPErrorStopsWithoutFile(t *testing.T) {
	server, _ := newAuditServer(t)
	dir := t.TempDir()
	output, err := runAuditCLI(dir, server.URL+"/missing.bin")
	if err == nil {
		t.Fatalf("expected non-zero exit status:\n%s", output)
	}
	if !strings.Contains(output, "status 404 Not Found") {
		t.Fatalf("missing HTTP status in output:\n%s", output)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "missing.bin")); !os.IsNotExist(statErr) {
		t.Fatalf("failed HTTP download left a file behind: %v", statErr)
	}
}

func TestAuditMirrorAndConvertLinks(t *testing.T) {
	server, _ := newAuditServer(t)
	dir := t.TempDir()
	output, err := runAuditCLI(dir, "--mirror", "--convert-links", server.URL+"/")
	if err != nil {
		t.Fatalf("mirror failed: %v\n%s", err, output)
	}
	root := mirrorRoot(dir, server.URL)
	for _, rel := range []string{
		"index.html",
		"css/style.css",
		"img/photo.jpg",
		"img/anim.gif",
		"img/bg.png",
		"page/about.html",
		"js/app.js",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("mirror missing %s: %v\n%s", rel, err, output)
		}
	}

	index, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(index), server.URL) {
		t.Fatalf("--convert-links left absolute site URLs in index.html:\n%s", index)
	}
	for _, want := range []string{"css/style.css", "img/photo.jpg", "page/about.html", "js/app.js"} {
		if !strings.Contains(string(index), want) {
			t.Fatalf("converted index.html does not contain %q:\n%s", want, index)
		}
	}
}

func TestAuditMirrorReject(t *testing.T) {
	server, _ := newAuditServer(t)
	dir := t.TempDir()
	output, err := runAuditCLI(dir, "--mirror", "--reject=gif", server.URL+"/")
	if err != nil {
		t.Fatalf("mirror failed: %v\n%s", err, output)
	}
	root := mirrorRoot(dir, server.URL)
	if _, err := os.Stat(filepath.Join(root, "img", "photo.jpg")); err != nil {
		t.Fatalf("expected JPG was not mirrored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "img", "anim.gif")); !os.IsNotExist(err) {
		t.Fatalf("rejected GIF exists or returned unexpected error: %v", err)
	}
}

func TestAuditMirrorExclude(t *testing.T) {
	server, _ := newAuditServer(t)
	dir := t.TempDir()
	output, err := runAuditCLI(dir, "--mirror", "-X=/img", server.URL+"/")
	if err != nil {
		t.Fatalf("mirror failed: %v\n%s", err, output)
	}
	root := mirrorRoot(dir, server.URL)
	if _, err := os.Stat(filepath.Join(root, "css", "style.css")); err != nil {
		t.Fatalf("expected CSS was not mirrored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "img")); !os.IsNotExist(err) {
		t.Fatalf("excluded /img exists or returned unexpected error: %v", err)
	}
}

func newAuditServer(t *testing.T) (*httptest.Server, []byte) {
	t.Helper()
	filePayload := bytes.Repeat([]byte("audit-payload-"), 4096)
	ratePayload := bytes.Repeat([]byte("r"), 300*1024)
	batchPayload := bytes.Repeat([]byte("b"), 4096)

	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><html><head>
<link rel="stylesheet" href="%s/css/style.css">
<script src="%s/js/app.js"></script>
</head><body>
<a href="%s/page/about.html">About</a>
<img src="%s/img/photo.jpg">
<img src="%s/img/anim.gif">
</body></html>`, baseURL, baseURL, baseURL, baseURL, baseURL)
	})
	mux.HandleFunc("/file.jpg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(filePayload)))
		_, _ = w.Write(filePayload)
	})
	mux.HandleFunc("/background.bin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(filePayload)))
		_, _ = w.Write(filePayload)
	})
	mux.HandleFunc("/rate.bin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(ratePayload)))
		_, _ = w.Write(ratePayload)
	})
	for _, path := range []string{"/batch-a.zip", "/batch-b.zip", "/batch-c.zip"} {
		path := path
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(300 * time.Millisecond)
			w.Header().Set("Content-Length", fmt.Sprint(len(batchPayload)))
			_, _ = w.Write(batchPayload)
		})
	}
	mux.HandleFunc("/css/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		fmt.Fprintf(w, `body{background:url("%s/img/bg.png")}`, baseURL)
	})
	mux.HandleFunc("/img/photo.jpg", writeBytes([]byte("jpg"), "image/jpeg"))
	mux.HandleFunc("/img/anim.gif", writeBytes([]byte("gif"), "image/gif"))
	mux.HandleFunc("/img/bg.png", writeBytes([]byte("png"), "image/png"))
	mux.HandleFunc("/page/about.html", writeBytes([]byte(`<html><a href="/">home</a></html>`), "text/html"))
	mux.HandleFunc("/js/app.js", writeBytes([]byte(`console.log("audit")`), "application/javascript"))

	server := httptest.NewServer(mux)
	baseURL = server.URL
	t.Cleanup(server.Close)
	return server, filePayload
}

func writeBytes(data []byte, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", fmt.Sprint(len(data)))
		_, _ = w.Write(data)
	}
}

func runAuditCLI(dir string, args ...string) (string, error) {
	cmd := exec.Command(auditBinary, args...)
	cmd.Dir = dir
	cmd.Stdin = nil
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return output.String(), err
}

func assertFileEquals(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("file %s content differs: got %d bytes, want %d", path, len(got), len(want))
	}
}

func assertTimestampLine(t *testing.T, output, prefix string) {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		value := strings.TrimPrefix(line, prefix)
		if _, err := time.Parse("2006-01-02 15:04:05", value); err != nil {
			t.Fatalf("%s has wrong timestamp format: %q", prefix, line)
		}
		return
	}
	t.Fatalf("missing %s timestamp in output:\n%s", prefix, output)
}

func mirrorRoot(dir, rawURL string) string {
	host := strings.TrimPrefix(rawURL, "http://")
	host = strings.TrimPrefix(host, "https://")
	host = strings.NewReplacer(":", "_", "/", "_", `\\`, "_").Replace(host)
	return filepath.Join(dir, host)
}
