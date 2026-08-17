package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const timestampLayout = "2006-01-02 15:04:05"

type Metadata struct {
	URL           string
	Path          string
	ContentLength int64
}

type Options struct {
	OutputName   string
	OutputDir    string
	RateLimit    int64
	ShowProgress bool
	Quiet        bool
	OnMetadata   func(Metadata)
}

type Result struct {
	URL           string
	Path          string
	Bytes         int64
	ContentLength int64
}

type Downloader struct {
	client *http.Client
	out    io.Writer
}

func New(client *http.Client, out io.Writer) *Downloader {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Minute}
	}
	if out == nil {
		out = io.Discard
	}
	return &Downloader{client: client, out: out}
}

func (d *Downloader) Fetch(ctx context.Context, rawURL string, opts Options) (Result, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return Result{}, fmt.Errorf("invalid URL %q", rawURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Result{}, fmt.Errorf("unsupported URL scheme %q", parsed.Scheme)
	}

	if !opts.Quiet {
		fmt.Fprintf(d.out, "start at %s\n", time.Now().Format(timestampLayout))
		fmt.Fprint(d.out, "sending request, awaiting response... ")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Result{}, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		if !opts.Quiet {
			fmt.Fprintln(d.out, "request failed")
		}
		return Result{}, fmt.Errorf("request %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if !opts.Quiet {
		fmt.Fprintf(d.out, "status %s\n", resp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("download failed with status %s", resp.Status)
	}

	if !opts.Quiet {
		if resp.ContentLength >= 0 {
			fmt.Fprintf(d.out, "content size: %d [%s]\n", resp.ContentLength, roundedSize(resp.ContentLength))
		} else {
			fmt.Fprintln(d.out, "content size: unknown")
		}
	}

	filename := opts.OutputName
	if filename == "" {
		filename = filenameFromURL(resp.Request.URL)
	}
	dir, err := expandHome(opts.OutputDir)
	if err != nil {
		return Result{}, err
	}
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{}, fmt.Errorf("create output directory: %w", err)
	}
	destination := filepath.Join(dir, filename)
	if !opts.Quiet {
		fmt.Fprintf(d.out, "saving file to: %s\n", displayPath(destination))
	}

	file, err := os.Create(destination)
	if err != nil {
		return Result{}, fmt.Errorf("create %s: %w", destination, err)
	}
	completed := false
	defer func() {
		_ = file.Close()
		if !completed {
			_ = os.Remove(destination)
		}
	}()

	if opts.OnMetadata != nil {
		opts.OnMetadata(Metadata{URL: rawURL, Path: destination, ContentLength: resp.ContentLength})
	}

	reader := newRateLimitedReader(resp.Body, opts.RateLimit)
	buffer := make([]byte, 32*1024)
	var downloaded int64
	var prog *progress
	if opts.ShowProgress && !opts.Quiet {
		prog = newProgress(d.out, resp.ContentLength)
	}

	for {
		n, readErr := reader.Read(buffer)
		if n > 0 {
			written, writeErr := file.Write(buffer[:n])
			downloaded += int64(written)
			if prog != nil {
				prog.update(downloaded, false)
			}
			if writeErr != nil {
				return Result{}, fmt.Errorf("write %s: %w", destination, writeErr)
			}
			if written != n {
				return Result{}, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return Result{}, fmt.Errorf("download %s: %w", rawURL, readErr)
		}
	}

	if err := file.Close(); err != nil {
		return Result{}, fmt.Errorf("close %s: %w", destination, err)
	}
	completed = true
	if prog != nil {
		prog.update(downloaded, true)
		fmt.Fprintln(d.out)
	}
	if !opts.Quiet {
		fmt.Fprintf(d.out, "Downloaded [%s]\n", rawURL)
		fmt.Fprintf(d.out, "finished at %s\n", time.Now().Format(timestampLayout))
	}
	return Result{URL: rawURL, Path: destination, Bytes: downloaded, ContentLength: resp.ContentLength}, nil
}

func filenameFromURL(u *url.URL) string {
	name := path.Base(u.Path)
	if name == "." || name == "/" || name == "" {
		return "index.html"
	}
	if decoded, err := url.PathUnescape(name); err == nil && decoded != "" {
		name = decoded
	}
	return sanitizeFilename(name)
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, string(filepath.Separator), "_")
	if filepath.Separator != '/' {
		name = strings.ReplaceAll(name, "/", "_")
	}
	if name == "" || name == "." || name == ".." {
		return "index.html"
	}
	return name
}

func expandHome(dir string) (string, error) {
	if dir == "" || dir == "~" {
		if dir == "" {
			return "", nil
		}
		return os.UserHomeDir()
	}
	if strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return filepath.Join(home, dir[2:]), nil
	}
	return dir, nil
}

func roundedSize(bytes int64) string {
	if bytes < 0 {
		return "unknown"
	}
	const gb = 1_000_000_000
	const mb = 1_000_000
	if bytes >= gb {
		return fmt.Sprintf("~%.2fGB", float64(bytes)/gb)
	}
	return fmt.Sprintf("~%.2fMB", float64(bytes)/mb)
}

func displayPath(p string) string {
	if !filepath.IsAbs(p) && !strings.HasPrefix(p, "."+string(filepath.Separator)) {
		return "." + string(filepath.Separator) + p
	}
	return p
}
