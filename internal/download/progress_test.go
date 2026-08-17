package download

import (
	"bytes"
	"testing"
)

func TestProgressClearsPreviousLineTail(t *testing.T) {
	var out bytes.Buffer
	p := &progress{out: &out}

	p.writeLine("1234567890", false)
	p.writeLine("short", true)

	const want = "\r1234567890\rshort     \n"
	if got := out.String(); got != want {
		t.Fatalf("progress output = %q, want %q", got, want)
	}
}

func TestProgressFinalLineAddsSingleNewline(t *testing.T) {
	var out bytes.Buffer
	p := &progress{out: &out}

	p.writeLine("done", true)

	const want = "\rdone\n"
	if got := out.String(); got != want {
		t.Fatalf("progress output = %q, want %q", got, want)
	}
}
