package stringutil

import "strings"

// NormalizeToken یکسان‌سازی نام فرمان با حذف خط تیره‌های ابتدای آن
func NormalizeToken(value string) string {
	normalized := strings.TrimLeft(strings.TrimSpace(value), "-")
	return strings.ToLower(normalized)
}
