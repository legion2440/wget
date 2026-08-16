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
	"sync"
	"time"

	"wget/internal/download"
)

const defaultWorkers = 4

type Options struct {
	Reject       []string
	Exclude      []string
	ConvertLinks bool
	BaseDir      string
	Workers      int
	RateLimit    int64
}

type resourceKind uint8

const (
	resourceBinary resourceKind = iota
	resourceHTML
	resourceCSS
)

type savedResource struct {
	key      string
	localRel string
	baseURL  *url.URL
	kind     resourceKind
}
type fetchResult struct {
	links []*url.URL
	saved *savedResource
}
type crawlResult struct {
	target *url.URL
	result fetchResult
	err    error
}

type Mirrorer struct {
	client       *http.Client
	out          io.Writer
	opts         Options
	allowedHosts map[string]struct{}
	reject       map[string]struct{}
	exclude      []string
	rootDir      string
	rootKey      string
	outMu        sync.Mutex
	paths        map[string]string
	saved        []savedResource
	limiter      *download.RateLimiter
}

func New(client *http.Client, out io.Writer, opts Options) *Mirrorer {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if out == nil {
		out = io.Discard
	}
	m := &Mirrorer{client: client, out: out, opts: opts, allowedHosts: make(map[string]struct{}), reject: make(map[string]struct{}), paths: make(map[string]string), limiter: download.NewRateLimiter(opts.RateLimit)}
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

func (m *Mirrorer) Run(ctx context.Context, rawURL string) error {
	root, err := url.Parse(rawURL)
	if err != nil || root.Scheme == "" || root.Host == "" {
		return fmt.Errorf("invalid mirror URL %q", rawURL)
	}
	if root.Scheme != "http" && root.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme %q", root.Scheme)
	}
	root.Fragment = ""
	m.rootKey = canonicalKey(root)
	m.allowedHosts[strings.ToLower(root.Host)] = struct{}{}
	base, err := expandBaseDir(m.opts.BaseDir)
	if err != nil {
		return err
	}
	if base == "" {
		base = "."
	}
	m.rootDir = filepath.Join(base, sanitizeHost(root.Host))
	if err := os.MkdirAll(m.rootDir, 0o755); err != nil {
		return fmt.Errorf("create mirror directory: %w", err)
	}
	m.logf("mirroring %s to %s\n", rawURL, m.rootDir)
	first, err := m.fetchOne(ctx, root, true)
	if err != nil {
		return err
	}
	m.registerSaved(first.saved)
	if err := m.crawlConcurrent(ctx, root, first.links); err != nil {
		return err
	}
	if m.opts.ConvertLinks {
		if err := m.convertSavedResources(); err != nil {
			return err
		}
	}
	m.logf("mirror finished: %s\n", m.rootDir)
	return nil
}

func (m *Mirrorer) crawlConcurrent(ctx context.Context, root *url.URL, initial []*url.URL) error {
	workers := m.opts.Workers
	if workers <= 0 {
		workers = defaultWorkers
	}
	jobs := make(chan *url.URL)
	results := make(chan crawlResult, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range jobs {
				r, err := m.fetchOne(ctx, target, false)
				results <- crawlResult{target: target, result: r, err: err}
			}
		}()
	}
	scheduled := map[string]struct{}{canonicalKey(root): {}}
	queue := make([]*url.URL, 0, len(initial))
	enqueue := func(items []*url.URL) {
		for _, u := range items {
			if u == nil || !m.isAllowed(u) || m.shouldSkip(u) {
				continue
			}
			key := canonicalKey(u)
			if _, ok := scheduled[key]; ok {
				continue
			}
			scheduled[key] = struct{}{}
			queue = append(queue, u)
		}
	}
	enqueue(initial)
	active := 0
	for len(queue) > 0 || active > 0 {
		var send chan *url.URL
		var next *url.URL
		if len(queue) > 0 && active < workers {
			send = jobs
			next = queue[0]
		}
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		case send <- next:
			queue = queue[1:]
			active++
		case result := <-results:
			active--
			if result.err != nil {
				m.logf("skip %s: %v\n", result.target, result.err)
				continue
			}
			m.registerSaved(result.result.saved)
			enqueue(result.result.links)
		}
	}
	close(jobs)
	wg.Wait()
	return nil
}

func (m *Mirrorer) fetchOne(ctx context.Context, target *url.URL, root bool) (fetchResult, error) {
	if target == nil {
		return fetchResult{}, nil
	}
	target = cloneURL(target)
	target.Fragment = ""
	if !m.isAllowed(target) || m.shouldSkip(target) {
		return fetchResult{}, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return fetchResult{}, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return fetchResult{}, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.Request != nil && resp.Request.URL != nil {
		finalHost := strings.ToLower(resp.Request.URL.Host)
		if root {
			m.allowedHosts[finalHost] = struct{}{}
		} else {
			if _, ok := m.allowedHosts[finalHost]; !ok {
				return fetchResult{}, fmt.Errorf("redirected outside mirrored host to %s", resp.Request.URL.Host)
			}
		}
	}
	if resp.StatusCode != http.StatusOK {
		return fetchResult{}, fmt.Errorf("status %s", resp.Status)
	}
	contentType := resp.Header.Get("Content-Type")
	kind := detectResourceKind(contentType, target.Path, root)
	pathType := contentType
	if normalizedMediaType(pathType) == "" && kind == resourceHTML {
		pathType = "text/html"
	}
	localRel := localPathFor(target, pathType)
	localAbs := filepath.Join(m.rootDir, localRel)
	if err := os.MkdirAll(filepath.Dir(localAbs), 0o755); err != nil {
		return fetchResult{}, fmt.Errorf("create path %s: %w", filepath.Dir(localAbs), err)
	}
	baseURL := target
	if resp.Request != nil && resp.Request.URL != nil {
		baseURL = resp.Request.URL
	}
	body := io.Reader(resp.Body)
	if m.limiter != nil {
		body = m.limiter.Wrap(body)
	}
	var links []*url.URL
	switch kind {
	case resourceHTML:
		data, err := io.ReadAll(body)
		if err != nil {
			return fetchResult{}, fmt.Errorf("read body: %w", err)
		}
		processed, found, err := m.processHTML(data, baseURL, localRel, false)
		if err != nil {
			return fetchResult{}, fmt.Errorf("parse HTML: %w", err)
		}
		links = found
		if err := writeFileAtomic(localAbs, processed); err != nil {
			return fetchResult{}, fmt.Errorf("save %s: %w", localAbs, err)
		}
	case resourceCSS:
		data, err := io.ReadAll(body)
		if err != nil {
			return fetchResult{}, fmt.Errorf("read body: %w", err)
		}
		processed, found := m.processCSS(data, baseURL, localRel, false)
		links = found
		if err := writeFileAtomic(localAbs, processed); err != nil {
			return fetchResult{}, fmt.Errorf("save %s: %w", localAbs, err)
		}
	default:
		if err := writeStreamAtomic(localAbs, body); err != nil {
			return fetchResult{}, fmt.Errorf("save %s: %w", localAbs, err)
		}
	}
	saved := &savedResource{key: canonicalKey(target), localRel: localRel, baseURL: cloneURL(baseURL), kind: kind}
	m.logf("saved %s\n", localAbs)
	return fetchResult{links: links, saved: saved}, nil
}

func detectResourceKind(contentType, urlPath string, root bool) resourceKind {
	mediaType := normalizedMediaType(contentType)
	ext := strings.ToLower(path.Ext(urlPath))
	if mediaType == "text/html" || mediaType == "application/xhtml+xml" || ext == ".html" || ext == ".htm" || (root && mediaType == "" && ext == "") {
		return resourceHTML
	}
	if mediaType == "text/css" || ext == ".css" {
		return resourceCSS
	}
	return resourceBinary
}
func (m *Mirrorer) registerSaved(saved *savedResource) {
	if saved == nil {
		return
	}
	m.paths[saved.key] = saved.localRel
	m.saved = append(m.saved, *saved)
}
func (m *Mirrorer) convertSavedResources() error {
	for _, saved := range m.saved {
		if saved.kind == resourceBinary {
			continue
		}
		abs := filepath.Join(m.rootDir, saved.localRel)
		data, err := os.ReadFile(abs)
		if err != nil {
			if saved.key == m.rootKey {
				return fmt.Errorf("convert root %s: %w", abs, err)
			}
			m.logf("skip convert %s: %v\n", abs, err)
			continue
		}
		var processed []byte
		switch saved.kind {
		case resourceHTML:
			processed, _, err = m.processHTML(data, saved.baseURL, saved.localRel, true)
		case resourceCSS:
			processed, _ = m.processCSS(data, saved.baseURL, saved.localRel, true)
		}
		if err != nil {
			if saved.key == m.rootKey {
				return fmt.Errorf("convert root %s: %w", abs, err)
			}
			m.logf("skip convert %s: %v\n", abs, err)
			continue
		}
		if string(processed) == string(data) {
			continue
		}
		if err := writeFileAtomic(abs, processed); err != nil {
			if saved.key == m.rootKey {
				return fmt.Errorf("convert root %s: %w", abs, err)
			}
			m.logf("skip convert %s: %v\n", abs, err)
		}
	}
	return nil
}

func writeFileAtomic(destination string, data []byte) error {
	dir := filepath.Dir(destination)
	tmp, err := os.CreateTemp(dir, ".wget-mirror-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, destination); err != nil {
		return err
	}
	ok = true
	return nil
}
func writeStreamAtomic(destination string, r io.Reader) error {
	dir := filepath.Dir(destination)
	tmp, err := os.CreateTemp(dir, ".wget-mirror-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if _, err := io.Copy(tmp, r); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, destination); err != nil {
		return err
	}
	ok = true
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
	if _, ok := m.reject[ext]; ok && ext != "" {
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
func cloneURL(u *url.URL) *url.URL { copy := *u; return &copy }
func sanitizeHost(host string) string {
	return strings.NewReplacer(":", "_", "/", "_", `\`, `_`).Replace(host)
}
func (m *Mirrorer) logf(format string, args ...any) {
	m.outMu.Lock()
	defer m.outMu.Unlock()
	fmt.Fprintf(m.out, format, args...)
}
func expandBaseDir(dir string) (string, error) {
	if dir == "" {
		return "", nil
	}
	if dir == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return home, nil
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
