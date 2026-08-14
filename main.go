package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"wget/internal/cli"
	"wget/internal/download"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "wget: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	opts, err := cli.Parse(args)
	if err != nil {
		return err
	}
	if opts.Background || opts.Mirror {
		return fmt.Errorf("requested mode is not available in this build")
	}
	rate, err := download.ParseRate(opts.RateLimit)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Minute}
	downloadOpts := download.Options{OutputName: opts.OutputName, OutputDir: opts.OutputDir, RateLimit: rate, ShowProgress: true}
	if opts.InputFile != "" {
		return download.Batch(ctx, client, os.Stdout, opts.InputFile, downloadOpts)
	}
	d := download.New(client, os.Stdout)
	_, err = d.Fetch(ctx, opts.URL, downloadOpts)
	return err
}
