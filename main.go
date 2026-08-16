package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
	"wget/internal/background"
	"wget/internal/cli"
	"wget/internal/download"
	"wget/internal/mirror"
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
	if opts.Background && !opts.BackgroundChild {
		if err := background.Start(args); err != nil {
			return err
		}
		fmt.Println("Output will be written to \"wget-log\".")
		return nil
	}
	rate, err := download.ParseRate(opts.RateLimit)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Minute}
	if opts.Mirror {
		m := mirror.New(client, os.Stdout, mirror.Options{Reject: opts.Reject, Exclude: opts.Exclude, ConvertLinks: opts.ConvertLinks, BaseDir: opts.OutputDir, RateLimit: rate})
		return m.Run(ctx, opts.URL)
	}
	downloadOpts := download.Options{OutputName: opts.OutputName, OutputDir: opts.OutputDir, RateLimit: rate, ShowProgress: !opts.BackgroundChild}
	if opts.InputFile != "" {
		return download.Batch(ctx, client, os.Stdout, opts.InputFile, downloadOpts)
	}
	d := download.New(client, os.Stdout)
	_, err = d.Fetch(ctx, opts.URL, downloadOpts)
	return err
}
