package cli

import (
	"reflect"
	"testing"
)

func TestParseAssignmentFlags(t *testing.T) {
	opts, err := Parse([]string{"--mirror", "--convert-links", "-R=jpg,gif", "-X=/assets,/img", "https://example.com"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !opts.Mirror || !opts.ConvertLinks || opts.URL != "https://example.com" {
		t.Fatalf("unexpected options: %+v", opts)
	}
	if !reflect.DeepEqual(opts.Reject, []string{"jpg", "gif"}) {
		t.Fatalf("Reject = %#v", opts.Reject)
	}
	if !reflect.DeepEqual(opts.Exclude, []string{"/assets", "/img"}) {
		t.Fatalf("Exclude = %#v", opts.Exclude)
	}
}
func TestParseSupportsSeparatedAndAttachedShortValues(t *testing.T) {
	opts, err := Parse([]string{"-O", "renamed.zip", "-Pdownloads", "--rate-limit", "300k", "https://example.com/file.zip"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.OutputName != "renamed.zip" || opts.OutputDir != "downloads" || opts.RateLimit != "300k" {
		t.Fatalf("unexpected options: %+v", opts)
	}
}
func TestParseRejectsMirrorOnlyFlagsWithoutMirror(t *testing.T) {
	if _, err := Parse([]string{"-R=gif", "https://example.com"}); err == nil {
		t.Fatal("expected an error")
	}
}
func TestParseInputFileDoesNotRequireURL(t *testing.T) {
	opts, err := Parse([]string{"-i=downloads.txt"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.InputFile != "downloads.txt" {
		t.Fatalf("InputFile = %q", opts.InputFile)
	}
}
func TestParseAllowsRateLimitWithMirror(t *testing.T) {
	opts, err := Parse([]string{"--mirror", "--rate-limit=300k", "https://example.com"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !opts.Mirror || opts.RateLimit != "300k" {
		t.Fatalf("unexpected options: %+v", opts)
	}
}
