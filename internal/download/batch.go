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

type batchEventKind uint8

const (
	batchMetadata batchEventKind = iota
	batchFinished
)

type batchEvent struct {
	kind      batchEventKind
	index     int
	url, path string
	size      int64
	err       error
}

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

	events := make(chan batchEvent, len(urls)*2)
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
			parentMetadata := local.OnMetadata
			local.OnMetadata = func(md Metadata) {
				if parentMetadata != nil {
					parentMetadata(md)
				}
				events <- batchEvent{kind: batchMetadata, index: index, size: md.ContentLength}
			}
			r, err := d.Fetch(ctx, rawURL, local)
			events <- batchEvent{kind: batchFinished, index: index, url: rawURL, path: r.Path, err: err}
		}()
	}
	go func() {
		wg.Wait()
		close(events)
	}()

	sizes := make([]string, len(urls))
	metadataSeen := make([]bool, len(urls))
	for i := range sizes {
		sizes[i] = "unknown"
	}
	resolvedMetadata := 0
	sizesPrinted := false
	pendingFinished := make([]batchEvent, 0, len(urls))
	finishedURLs := make([]string, 0, len(urls))
	var errs []string

	printFinished := func(event batchEvent) {
		if event.err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", event.url, event.err))
			return
		}
		fmt.Fprintf(out, "finished %s\n", filepath.Base(event.path))
		finishedURLs = append(finishedURLs, event.url)
	}
	printSizes := func() {
		if sizesPrinted {
			return
		}
		fmt.Fprintf(out, "content size: [%s]\n", strings.Join(sizes, ", "))
		sizesPrinted = true
		for _, event := range pendingFinished {
			printFinished(event)
		}
		pendingFinished = nil
	}

	for event := range events {
		switch event.kind {
		case batchMetadata:
			if !metadataSeen[event.index] {
				metadataSeen[event.index] = true
				resolvedMetadata++
			}
			if event.size >= 0 {
				sizes[event.index] = fmt.Sprintf("%d", event.size)
			}
		case batchFinished:
			if event.err != nil && !metadataSeen[event.index] {
				metadataSeen[event.index] = true
				resolvedMetadata++
			}
			if sizesPrinted {
				printFinished(event)
			} else {
				pendingFinished = append(pendingFinished, event)
			}
		}
		if resolvedMetadata == len(urls) {
			printSizes()
		}
	}
	printSizes()

	fmt.Fprintf(out, "\nDownload finished:  [%s]\n", strings.Join(finishedURLs, " "))
	if len(errs) > 0 {
		return fmt.Errorf("%d download(s) failed: %s", len(errs), strings.Join(errs, "; "))
	}
	return nil
}
