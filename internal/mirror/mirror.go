package mirror

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

// Options controls website mirroring.
type Options struct {
	Reject       []string
	Exclude      []string
	ConvertLinks bool
	BaseDir      string
}

// Mirrorer recursively stores same-site pages and resources.
type Mirrorer struct {
	client       *http.Client
	out          io.Writer
	opts         Options
	visited      map[string]struct{}
	allowedHosts map[string]struct{}
	reject       map[string]struct{}
	exclude      []string
	rootDir      string
}

func New(client *http.Client, out io.Writer, opts Options) *Mirrorer {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if out == nil {
		out = io.Discard
	}
	m := &Mirrorer{
		client:       client,
		out:          out,
		opts:         opts,
		visited:      make(map[string]struct{}),
		allowedHosts: make(map[string]struct{}),
		reject:       make(map[string]struct{}),
	}
	for _, suffix := range opts.Reject {
		suffix = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(suffix, ".")))
		if suffix != "" {
			m.reject[suffix] = struct{}{}
		}
	}
	for _, item := range opts.Exclude {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if !strings.HasPrefix(item, "/") {
			item = "/" + item
		}
		m.exclude = append(m.exclude, path.Clean(item))
	}
	return m
}

// Run mirrors one website into a folder named after the requested host.
func (m *Mirrorer) Run(ctx context.Context, rawURL string) error {
	root, err := url.Parse(rawURL)
	if err != nil || root.Scheme == "" || root.Host == "" {
		return fmt.Errorf("invalid mirror URL %q", rawURL)
	}
	if root.Scheme != "http" && root.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme %q", root.Scheme)
	}
	root.Fragment = ""
	m.allowedHosts[strings.ToLower(root.Host)] = struct{}{}

	base := m.opts.BaseDir
	if base == "" {
		base = "."
	}
	m.rootDir = filepath.Join(base, sanitizeHost(root.Host))
	if err := os.MkdirAll(m.rootDir, 0o755); err != nil {
		return fmt.Errorf("create mirror directory: %w", err)
	}
	fmt.Fprintf(m.out, "mirroring %s to %s\n", rawURL, m.rootDir)
	if err := m.crawl(ctx, root, true); err != nil {
		return err
	}
	fmt.Fprintf(m.out, "mirror finished: %s\n", m.rootDir)
	return nil
}

func (m *Mirrorer) crawl(ctx context.Context, target *url.URL, root bool) error {
	if target == nil {
		return nil
	}
	target = cloneURL(target)
	target.Fragment = ""
	if !m.isAllowed(target) || m.shouldSkip(target) {
		return nil
	}
	key := canonicalKey(target)
	if _, exists := m.visited[key]; exists {
		return nil
	}
	m.visited[key] = struct{}{}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		if root {
			return err
		}
		fmt.Fprintf(m.out, "skip %s: %v\n", target, err)
		return nil
	}
	resp, err := m.client.Do(req)
	if err != nil {
		if root {
			return fmt.Errorf("mirror request %s: %w", target, err)
		}
		fmt.Fprintf(m.out, "skip %s: %v\n", target, err)
		return nil
	}
	defer resp.Body.Close()

	if root && resp.Request != nil && resp.Request.URL != nil {
		m.allowedHosts[strings.ToLower(resp.Request.URL.Host)] = struct{}{}
	}
	if resp.StatusCode != http.StatusOK {
		if root {
			return fmt.Errorf("mirror root returned %s", resp.Status)
		}
		fmt.Fprintf(m.out, "skip %s: status %s\n", target, resp.Status)
		return nil
	}

	localRel := localPathFor(target)
	localAbs := filepath.Join(m.rootDir, localRel)
	if err := os.MkdirAll(filepath.Dir(localAbs), 0o755); err != nil {
		return fmt.Errorf("create mirror path: %w", err)
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	ext := strings.ToLower(path.Ext(target.Path))
	baseURL := target
	if resp.Request != nil && resp.Request.URL != nil {
		baseURL = resp.Request.URL
	}

	var links []*url.URL
	switch {
	case strings.Contains(contentType, "text/html") || ext == ".html" || ext == ".htm" || (ext == "" && root):
		data, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", target, readErr)
		}
		processed, found, parseErr := m.processHTML(data, baseURL, localRel)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", target, parseErr)
		}
		links = found
		if err := os.WriteFile(localAbs, processed, 0o644); err != nil {
			return fmt.Errorf("save %s: %w", localAbs, err)
		}
	case strings.Contains(contentType, "text/css") || ext == ".css":
		data, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", target, readErr)
		}
		processed, found := m.processCSS(data, baseURL, localRel)
		links = found
		if err := os.WriteFile(localAbs, processed, 0o644); err != nil {
			return fmt.Errorf("save %s: %w", localAbs, err)
		}
	default:
		file, createErr := os.Create(localAbs)
		if createErr != nil {
			return fmt.Errorf("save %s: %w", localAbs, createErr)
		}
		_, copyErr := io.Copy(file, resp.Body)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("save %s: %w", localAbs, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close %s: %w", localAbs, closeErr)
		}
	}

	fmt.Fprintf(m.out, "saved %s\n", localAbs)
	for _, link := range links {
		if err := m.crawl(ctx, link, false); err != nil {
			return err
		}
	}
	return nil
}

func (m *Mirrorer) isAllowed(u *url.URL) bool {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	_, ok := m.allowedHosts[strings.ToLower(u.Host)]
	return ok
}

func (m *Mirrorer) shouldSkip(u *url.URL) bool {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(u.Path), "."))
	if _, rejected := m.reject[ext]; rejected && ext != "" {
		return true
	}
	cleanPath := path.Clean("/" + u.Path)
	for _, excluded := range m.exclude {
		if excluded == "/" || cleanPath == excluded || strings.HasPrefix(cleanPath, excluded+"/") {
			return true
		}
	}
	return false
}

func canonicalKey(u *url.URL) string {
	copy := cloneURL(u)
	copy.Fragment = ""
	copy.Host = strings.ToLower(copy.Host)
	return copy.String()
}

func cloneURL(u *url.URL) *url.URL {
	copy := *u
	return &copy
}

func sanitizeHost(host string) string {
	replacer := strings.NewReplacer(":", "_", "/", "_", `\`, "_")
	return replacer.Replace(host)
}
