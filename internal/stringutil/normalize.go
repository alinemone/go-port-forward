package stringutil

import "strings"

func NormalizeToken(value string) string {
	normalized := strings.TrimLeft(strings.TrimSpace(value), "-")
	return strings.ToLower(normalized)
}
