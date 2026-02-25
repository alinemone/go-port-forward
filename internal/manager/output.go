package manager

import (
	"os"
	"strings"
)

type lineKind int

const (
	lineKindInfo lineKind = iota
	lineKindHealthy
	lineKindTransientError
	lineKindFatalError
)

func classifyOutputLine(line string, isError bool) lineKind {
	if indicatesHealthyPortForward(line) {
		return lineKindHealthy
	}

	if !isError {
		return lineKindInfo
	}

	if isTransientPortForwardError(line) {
		return lineKindTransientError
	}

	if looksLikeError(line) {
		return lineKindFatalError
	}

	return lineKindInfo
}

func indicatesHealthyPortForward(line string) bool {
	return strings.Contains(line, "Forwarding from") ||
		strings.Contains(line, "Handling connection for")
}

// تشخیص ساده خطا از روی متن خروجی
func looksLikeError(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "error") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "unable to") ||
		strings.Contains(lower, "cannot") ||
		strings.Contains(lower, "denied") ||
		strings.Contains(lower, "refused") ||
		strings.Contains(lower, "not found") ||
		strings.Contains(lower, "lost connection")
}

// خطاهای گذرای kubectl port-forward که معمولاً به معنی خرابی تونل نیستند
func isTransientPortForwardError(line string) bool {
	lower := strings.ToLower(line)

	return strings.Contains(lower, "an existing connection was forcibly closed by the remote host") ||
		strings.Contains(lower, "connection reset by peer") ||
		strings.Contains(lower, "broken pipe") ||
		strings.Contains(lower, "use of closed network connection") ||
		(strings.Contains(lower, "unhandled error") &&
			strings.Contains(lower, "error copying from remote stream to local connection")) ||
		(strings.Contains(lower, "unhandled error") &&
			strings.Contains(lower, "error copying from local connection to remote stream"))
}

// کوتاه‌سازی پیام خطا برای نمایش
func normalizeErrorLine(line string) string {
	if len(line) > 150 {
		line = line[:147] + "..."
	}
	return strings.Join(strings.Fields(line), " ")
}

// فعال بودن لاگ خطا در ترمینال بر اساس متغیر محیطی
func isStderrLoggingEnabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("PF_STDERR")))
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}
