package download

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// ParseRate converts assignment values such as 300k or 2M into bytes per second.
func ParseRate(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}

	multiplier := int64(1)
	number := value
	last := value[len(value)-1]
	switch last {
	case 'k', 'K':
		multiplier = 1024
		number = value[:len(value)-1]
	case 'm', 'M':
		multiplier = 1024 * 1024
		number = value[:len(value)-1]
	}

	n, err := strconv.ParseFloat(number, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid rate limit %q", value)
	}
	rate := int64(n * float64(multiplier))
	if rate <= 0 {
		return 0, fmt.Errorf("invalid rate limit %q", value)
	}
	return rate, nil
}

type rateLimitedReader struct {
	r       io.Reader
	rate    int64
	started time.Time
	total   int64
}

func newRateLimitedReader(r io.Reader, rate int64) io.Reader {
	if rate <= 0 {
		return r
	}
	return &rateLimitedReader{r: r, rate: rate, started: time.Now()}
}

func (r *rateLimitedReader) Read(p []byte) (int, error) {
	// Keep each returned chunk small enough for regular progress updates while
	// still avoiding excessive syscall overhead.
	maxChunk := int(r.rate / 20)
	if maxChunk < 1024 {
		maxChunk = 1024
	}
	if maxChunk > 32*1024 {
		maxChunk = 32 * 1024
	}
	if len(p) > maxChunk {
		p = p[:maxChunk]
	}

	n, err := r.r.Read(p)
	if n > 0 {
		r.total += int64(n)
		expected := time.Duration(float64(r.total) / float64(r.rate) * float64(time.Second))
		if wait := expected - time.Since(r.started); wait > 0 {
			time.Sleep(wait)
		}
	}
	return n, err
}
