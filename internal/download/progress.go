package download

import (
	"fmt"
	"io"
	"strings"
	"time"
)

type progress struct {
	out       io.Writer
	total     int64
	started   time.Time
	lastPrint time.Time
}

func newProgress(out io.Writer, total int64) *progress {
	now := time.Now()
	return &progress{out: out, total: total, started: now, lastPrint: now.Add(-time.Second)}
}

func (p *progress) update(downloaded int64, force bool) {
	now := time.Now()
	if !force && now.Sub(p.lastPrint) < 120*time.Millisecond {
		return
	}
	p.lastPrint = now

	elapsed := now.Sub(p.started).Seconds()
	if elapsed <= 0 {
		elapsed = 0.001
	}
	speed := float64(downloaded) / elapsed
	speedText := formatRate(speed)

	if p.total > 0 {
		percent := float64(downloaded) * 100 / float64(p.total)
		if percent > 100 {
			percent = 100
		}
		remaining := time.Duration(0)
		if speed > 0 && downloaded < p.total {
			remaining = time.Duration(float64(p.total-downloaded) / speed * float64(time.Second))
		}
		width := 48
		filled := int(percent / 100 * float64(width))
		if filled > width {
			filled = width
		}
		bar := strings.Repeat("=", filled) + strings.Repeat(" ", width-filled)
		fmt.Fprintf(p.out, "\r %s / %s [%s] %6.2f%% %s %s",
			formatBinary(downloaded), formatBinary(p.total), bar, percent, speedText, formatETA(remaining))
	} else {
		fmt.Fprintf(p.out, "\r %s downloaded %s", formatBinary(downloaded), speedText)
	}
	if force {
		fmt.Fprintln(p.out)
	}
}

func formatBinary(bytes int64) string {
	const kib = 1024
	const mib = 1024 * kib
	if bytes >= mib {
		return fmt.Sprintf("%.2f MiB", float64(bytes)/mib)
	}
	return fmt.Sprintf("%.2f KiB", float64(bytes)/kib)
}

func formatRate(bytesPerSecond float64) string {
	const kib = 1024
	const mib = 1024 * kib
	if bytesPerSecond >= mib {
		return fmt.Sprintf("%.2f MiB/s", bytesPerSecond/mib)
	}
	return fmt.Sprintf("%.2f KiB/s", bytesPerSecond/kib)
}

func formatETA(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	seconds := int64(d.Round(time.Second) / time.Second)
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	seconds %= 60
	return fmt.Sprintf("%dm%02ds", minutes, seconds)
}
