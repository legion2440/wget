package main

import (
	"bytes"
	"fmt"
	"io"
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
	for _, want := range []string{"start at ", "sending request, awaiting response... status 200 OK", fmt.Sprintf("content size: %d", len(payload)), "saving file to:", "100.00%", "Downloaded [", "finished at "} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q:\n%s", want, output)
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
func TestAuditChunkedLengthIsUnknown(t *testing.T) {
	server, _ := newAuditServer(t)
	output, err := runAuditCLI(t.TempDir(), server.URL+"/chunked.bin")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "content size: unknown") || strings.Contains(output, "content size: -1") {
		t.Fatalf("bad chunked output:\n%s", output)
	}
}
func TestAuditRateLimit(t *testing.T) {
	server, _ := newAuditServer(t)
	started := time.Now()
	output, err := runAuditCLI(t.TempDir(), "--rate-limit=300k", server.URL+"/rate.bin")
	if err != nil {
		t.Fatalf("wget failed: %v\n%s", err, output)
	}
	if time.Since(started) < 800*time.Millisecond {
		t.Fatalf("rate limit ignored")
	}
}
func TestAuditBatchDownloadsConcurrentlyAndCleanly(t *testing.T) {
	server, _ := newAuditServer(t)
	dir := t.TempDir()
	input := strings.Join([]string{server.URL + "/batch-a.zip", server.URL + "/batch-b.zip", server.URL + "/batch-c.zip"}, "\n") + "\n"
	_ = os.WriteFile(filepath.Join(dir, "downloads.txt"), []byte(input), 0o644)
	started := time.Now()
	output, err := runAuditCLI(dir, "-i=downloads.txt")
	if err != nil {
		t.Fatalf("wget failed: %v\n%s", err, output)
	}
	if time.Since(started) >= 800*time.Millisecond {
		t.Fatalf("batch appears sequential")
	}
	if strings.Contains(output, "sending request") || strings.Contains(output, "start at ") {
		t.Fatalf("batch output is noisy/interleaved:\n%s", output)
	}
	for _, name := range []string{"batch-a.zip", "batch-b.zip", "batch-c.zip"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output, "finished "+name) {
			t.Fatalf("missing finish for %s:\n%s", name, output)
		}
	}
}
func TestAuditBackgroundDownloadAndExactLogShape(t *testing.T) {
	server, payload := newAuditServer(t)
	dir := t.TempDir()
	output, err := runAuditCLI(dir, "-B", server.URL+"/background.bin")
	if err != nil {
		t.Fatalf("launcher failed: %v\n%s", err, output)
	}
	if strings.TrimSpace(output) != `Output will be written to "wget-log".` {
		t.Fatalf("unexpected launcher output %q", output)
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
		t.Fatal("background did not finish")
	}
	logText := string(logData)
	if strings.Contains(logText, "\n\nDownloaded [") {
		t.Fatalf("wget-log contains extra blank line:\n%s", logText)
	}
	for _, want := range []string{"start at ", "status 200 OK", "content size:", "saving file to:", "Downloaded [", "finished at "} {
		if !strings.Contains(logText, want) {
			t.Fatalf("log missing %q:\n%s", want, logText)
		}
	}
}
func TestAuditHTTPErrorStopsWithoutFile(t *testing.T) {
	server, _ := newAuditServer(t)
	dir := t.TempDir()
	output, err := runAuditCLI(dir, server.URL+"/missing.bin")
	if err == nil {
		t.Fatalf("expected error:\n%s", output)
	}
	if !strings.Contains(output, "status 404 Not Found") {
		t.Fatalf("missing status:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(dir, "missing.bin")); !os.IsNotExist(err) {
		t.Fatalf("failed download left file: %v", err)
	}
}
func TestAuditMirrorUsesDirectoryPrefixAndRequiredTagsOnly(t *testing.T) {
	server, _ := newAuditServer(t)
	dir := t.TempDir()
	output, err := runAuditCLI(dir, "--mirror", "--convert-links", "-P=mirror-out", server.URL+"/")
	if err != nil {
		t.Fatalf("mirror failed: %v\n%s", err, output)
	}
	root := mirrorRoot(filepath.Join(dir, "mirror-out"), server.URL)
	for _, rel := range []string{"index.html", "css/style.css", "img/photo.jpg", "img/anim.gif", "img/bg.png", "img/inline.png", "about.html", "about/team.png"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing %s: %v\n%s", rel, err, output)
		}
	}
	for _, rel := range []string{"js/app.js", "img/fake.png"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("script resource %s unexpectedly fetched: %v", rel, err)
		}
	}
	index, _ := os.ReadFile(filepath.Join(root, "index.html"))
	for _, want := range []string{"css/style.css", "img/photo.jpg", "img/inline.png", "about.html"} {
		if !strings.Contains(string(index), want) {
			t.Fatalf("converted index missing %q:\n%s", want, index)
		}
	}
}
func TestAuditMirrorRejectAndExclude(t *testing.T) {
	server, _ := newAuditServer(t)
	for _, tc := range []struct {
		args    []string
		absent  string
		present string
	}{{[]string{"--mirror", "--reject=gif", server.URL + "/"}, "img/anim.gif", "img/photo.jpg"}, {[]string{"--mirror", "-X=/img", server.URL + "/"}, "img", "css/style.css"}} {
		dir := t.TempDir()
		output, err := runAuditCLI(dir, tc.args...)
		if err != nil {
			t.Fatalf("mirror failed: %v\n%s", err, output)
		}
		root := mirrorRoot(dir, server.URL)
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(tc.present))); err != nil {
			t.Fatalf("expected %s: %v", tc.present, err)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(tc.absent))); !os.IsNotExist(err) {
			t.Fatalf("expected %s absent: %v", tc.absent, err)
		}
	}
}
func TestAuditRejectsIgnoredMirrorFlags(t *testing.T) {
	server, _ := newAuditServer(t)
	for _, args := range [][]string{{"--mirror", "-O=x", server.URL + "/"}, {"--mirror", "--rate-limit=300k", server.URL + "/"}} {
		output, err := runAuditCLI(t.TempDir(), args...)
		if err == nil {
			t.Fatalf("expected invalid combination for %v:\n%s", args, output)
		}
	}
}

func newAuditServer(t *testing.T) (*httptest.Server, []byte) {
	t.Helper()
	filePayload := bytes.Repeat([]byte("audit-payload-"), 4096)
	ratePayload := bytes.Repeat([]byte("r"), 300*1024)
	batchPayload := bytes.Repeat([]byte("b"), 4096)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!doctype html><html><head><link rel="stylesheet" href="/css/style.css"><style>.x{background:url('/img/inline.png')}</style><script src="/js/app.js">document.write("<img src='/img/fake.png'>")</script></head><body><a href="/about">About</a><img src="/img/photo.jpg"><img src="/img/anim.gif"></body></html>`)
	})
	mux.HandleFunc("/file.jpg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(filePayload)))
		_, _ = w.Write(filePayload)
	})
	mux.HandleFunc("/background.bin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(filePayload)))
		_, _ = w.Write(filePayload)
	})
	mux.HandleFunc("/chunked.bin", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "one")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = io.WriteString(w, "two")
	})
	mux.HandleFunc("/rate.bin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(ratePayload)))
		_, _ = w.Write(ratePayload)
	})
	for _, p := range []string{"/batch-a.zip", "/batch-b.zip", "/batch-c.zip"} {
		p := p
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(300 * time.Millisecond)
			w.Header().Set("Content-Length", fmt.Sprint(len(batchPayload)))
			_, _ = w.Write(batchPayload)
		})
	}
	mux.HandleFunc("/css/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		_, _ = io.WriteString(w, `body{background:url('../img/bg.png')}`)
	})
	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<img src="/about/team.png"><a href="/">home</a>`)
	})
	for _, p := range []string{"/img/photo.jpg", "/img/anim.gif", "/img/bg.png", "/img/inline.png", "/about/team.png", "/js/app.js", "/img/fake.png"} {
		p := p
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, p) })
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, filePayload
}
func runAuditCLI(dir string, args ...string) (string, error) {
	cmd := exec.Command(auditBinary, args...)
	cmd.Dir = dir
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
		t.Fatalf("content differs")
	}
}
func assertTimestampLine(t *testing.T, output, prefix string) {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			if _, err := time.Parse("2006-01-02 15:04:05", strings.TrimPrefix(line, prefix)); err != nil {
				t.Fatalf("wrong timestamp %q", line)
			}
			return
		}
	}
	t.Fatalf("missing timestamp %s", prefix)
}
func mirrorRoot(dir, rawURL string) string {
	host := strings.TrimPrefix(strings.TrimPrefix(rawURL, "http://"), "https://")
	host = strings.NewReplacer(":", "_", "/", "_", `\`, "_").Replace(host)
	return filepath.Join(dir, host)
}
