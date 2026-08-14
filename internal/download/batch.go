package download

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
)

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}

// Batch downloads every URL from a text file concurrently.
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

	lw := &lockedWriter{w: out}
	var wg sync.WaitGroup
	errCh := make(chan error, len(urls))
	completed := make(chan string, len(urls))

	for _, rawURL := range urls {
		rawURL := rawURL
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := New(client, lw)
			localOpts := opts
			localOpts.OutputName = ""
			localOpts.ShowProgress = false
			result, err := d.Fetch(ctx, rawURL, localOpts)
			if err != nil {
				errCh <- fmt.Errorf("%s: %w", rawURL, err)
				return
			}
			fmt.Fprintf(lw, "finished %s\n", result.Path)
			completed <- rawURL
		}()
	}

	wg.Wait()
	close(errCh)
	close(completed)

	finished := make([]string, 0, len(urls))
	for rawURL := range completed {
		finished = append(finished, rawURL)
	}
	fmt.Fprintf(lw, "\nDownload finished:  [%s]\n", strings.Join(finished, " "))

	var errs []string
	for err := range errCh {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d download(s) failed: %s", len(errs), strings.Join(errs, "; "))
	}
	return nil
}
