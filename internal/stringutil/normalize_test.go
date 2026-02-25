package stringutil

import "testing"

func TestNormalizeToken(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"add", "add"},
		{"--add", "add"},
		{"-add", "add"},
		{"  --ADD  ", "add"},
		{"Help", "help"},
		{"---version", "version"},
		{"", ""},
		{" ", ""},
		{"--", ""},
		{"ctrl+c", "ctrl+c"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeToken(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeToken(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
