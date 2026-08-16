package download

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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
		url, path string
		size      int64
		err       error
	}
	results := make(chan result, len(urls))
	var wg sync.WaitGroup
	for _, rawURL := range urls {
		rawURL := rawURL
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := New(client, io.Discard)
			local := opts
			local.OutputName = ""
			local.ShowProgress = false
			local.Quiet = true
			r, err := d.Fetch(ctx, rawURL, local)
			results <- result{url: rawURL, path: r.Path, size: r.ContentLength, err: err}
		}()
	}
	go func() { wg.Wait(); close(results) }()

	sizes := make([]int64, 0, len(urls))
	finished := make([]string, 0, len(urls))
	var errs []string
	for r := range results {
		if r.err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.url, r.err))
			continue
		}
		sizes = append(sizes, r.size)
		fmt.Fprintf(out, "finished %s\n", filepath.Base(r.path))
		finished = append(finished, r.url)
	}
	if len(sizes) > 0 {
		fmt.Fprintf(out, "content size: %v\n", sizes)
	}
	sort.Strings(finished)
	fmt.Fprintf(out, "\nDownload finished:  [%s]\n", strings.Join(finished, " "))
	if len(errs) > 0 {
		return fmt.Errorf("%d download(s) failed: %s", len(errs), strings.Join(errs, "; "))
	}
	return nil
}
