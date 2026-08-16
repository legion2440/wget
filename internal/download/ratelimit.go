package download

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

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

type RateLimiter struct {
	mu      sync.Mutex
	rate    int64
	started time.Time
	total   int64
}

func NewRateLimiter(rate int64) *RateLimiter {
	if rate <= 0 {
		return nil
	}
	return &RateLimiter{rate: rate}
}

func (l *RateLimiter) Wrap(r io.Reader) io.Reader {
	if l == nil || l.rate <= 0 {
		return r
	}
	return &sharedLimitedReader{r: r, limiter: l}
}

func (l *RateLimiter) maxChunk() int {
	maxChunk := int(l.rate / 20)
	if maxChunk < 1024 {
		maxChunk = 1024
	}
	if maxChunk > 32*1024 {
		maxChunk = 32 * 1024
	}
	return maxChunk
}

func (l *RateLimiter) wait(n int) {
	if l == nil || n <= 0 {
		return
	}
	now := time.Now()
	l.mu.Lock()
	if l.started.IsZero() {
		l.started = now
	}
	l.total += int64(n)
	expected := time.Duration(float64(l.total) / float64(l.rate) * float64(time.Second))
	wait := expected - now.Sub(l.started)
	l.mu.Unlock()
	if wait > 0 {
		time.Sleep(wait)
	}
}

type sharedLimitedReader struct {
	r       io.Reader
	limiter *RateLimiter
}

func (r *sharedLimitedReader) Read(p []byte) (int, error) {
	if max := r.limiter.maxChunk(); len(p) > max {
		p = p[:max]
	}
	n, err := r.r.Read(p)
	if n > 0 {
		r.limiter.wait(n)
	}
	return n, err
}

func newRateLimitedReader(r io.Reader, rate int64) io.Reader { return NewRateLimiter(rate).Wrap(r) }
