package download

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func Batch(ctx context.Context, client *http.Client, out io.Writer, inputFile string, opts Options) error {
	file, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("open input file: %w", err)
	}
	defer file.Close()
	var urls []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		urls = append(urls, line)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read input file: %w", err)
	}
	if len(urls) == 0 {
		return fmt.Errorf("input file contains no URLs")
	}
	type result struct {
		index     int
		url, path string
		size      int64
		err       error
	}
	results := make(chan result, len(urls))
	var wg sync.WaitGroup
	for index, rawURL := range urls {
		index, rawURL := index, rawURL
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := New(client, io.Discard)
			local := opts
			local.OutputName = ""
			local.ShowProgress = false
			local.Quiet = true
			r, err := d.Fetch(ctx, rawURL, local)
			results <- result{index: index, url: rawURL, path: r.Path, size: r.ContentLength, err: err}
		}()
	}
	go func() { wg.Wait(); close(results) }()
	completed := make([]result, 0, len(urls))
	sizes := make([]string, len(urls))
	for i := range sizes {
		sizes[i] = "unknown"
	}
	var errs []string
	for r := range results {
		if r.err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.url, r.err))
			continue
		}
		if r.size >= 0 {
			sizes[r.index] = fmt.Sprintf("%d", r.size)
		}
		completed = append(completed, r)
	}
	fmt.Fprintf(out, "content size: [%s]\n", strings.Join(sizes, ", "))
	for _, r := range completed {
		fmt.Fprintf(out, "finished %s\n", filepath.Base(r.path))
	}
	finished := make([]string, 0, len(completed))
	for _, r := range completed {
		finished = append(finished, r.url)
	}
	fmt.Fprintf(out, "\nDownload finished:  [%s]\n", strings.Join(finished, " "))
	if len(errs) > 0 {
		return fmt.Errorf("%d download(s) failed: %s", len(errs), strings.Join(errs, "; "))
	}
	return nil
}
