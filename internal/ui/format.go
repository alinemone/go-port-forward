package ui

import (
	"fmt"
	"time"
)

// formatRate renders a throughput compactly for the chart's live value:
// "", 840B, 12K, 3.4M (per second). Empty when idle so a quiet chart stays quiet.
func formatRate(bytesPerSec float64) string {
	switch {
	case bytesPerSec < 1:
		return ""
	case bytesPerSec < 1024:
		return fmt.Sprintf("%.0fB", bytesPerSec)
	case bytesPerSec < 1024*1024:
		return fmt.Sprintf("%.0fK", bytesPerSec/1024)
	case bytesPerSec < 10*1024*1024:
		return fmt.Sprintf("%.1fM", bytesPerSec/(1024*1024))
	default:
		return fmt.Sprintf("%.0fM", bytesPerSec/(1024*1024))
	}
}

// formatDuration renders an elapsed time compactly: 42s, 7m 03s, 2h 14m, 3d 4h.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}
