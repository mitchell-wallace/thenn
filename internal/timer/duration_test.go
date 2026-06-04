package timer

import (
	"strings"
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input       string
		expected    time.Duration
		wantErr     bool
		errContains string
	}{
		// Standard Integer Durations
		{"10s", 10 * time.Second, false, ""},
		{"2h 13m 55s", 2*time.Hour + 13*time.Minute + 55*time.Second, false, ""},
		{"1d 2h", 26 * time.Hour, false, ""},
		{"100ms", 100 * time.Millisecond, false, ""},
		{"1h2m3s", 1*time.Hour + 2*time.Minute + 3*time.Second, false, ""},
		{"5s 10m", 10*time.Minute + 5*time.Second, false, ""},
		{"10S 2H", 2*time.Hour + 10*time.Second, false, ""},
		{"1d 2h 3m 4s 5ms", 24*time.Hour + 2*time.Hour + 3*time.Minute + 4*time.Second + 5*time.Millisecond, false, ""},

		// Float Durations
		{"1.5h", 1*time.Hour + 30*time.Minute, false, ""},
		{"0.5m", 30 * time.Second, false, ""},
		{"1.25s", 1*time.Second + 250*time.Millisecond, false, ""},
		{"0.005s", 5 * time.Millisecond, false, ""},
		{"1.5d 2.5h", 36*time.Hour + 2*time.Hour + 30*time.Minute, false, ""},

		// Errors & Empty Cases
		{"", 0, true, "empty duration"},
		{"invalid", 0, true, "invalid duration format"},
		{"10s 2h extra", 0, true, "unknown unit"},
		{"10s : 2h", 0, true, "invalid duration format"},
		{"10x", 0, true, "unknown unit \"x\""},

		// Standalone & Missing Unit Cases with suggestions
		{"10", 0, true, `Did you mean "10s", "10m", or "10h"?`},
		{"1.5", 0, true, `Did you mean "1.5s", "1.5m", or "1.5h"?`},
		{"1h 30", 0, true, `Did you mean "1h 30m"?`},
		{"5m 10", 0, true, `Did you mean "5m 10s"?`},
		{"2d 4", 0, true, `Did you mean "2d 4h"?`},
		{"1s 5", 0, true, `Did you mean "1s 5ms"?`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			res, err := ParseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ParseDuration(%q) error %v does not contain %q", tt.input, err, tt.errContains)
				}
			} else if res != tt.expected {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, res, tt.expected)
			}
		})
	}
}

func TestSuggestHint(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"10", `Did you mean "10s", "10m", or "10h"?`},
		{"1.5", `Did you mean "1.5s", "1.5m", or "1.5h"?`},
		{"1h 30", `Did you mean "1h 30m"?`},
		{"5m 10", `Did you mean "5m 10s"?`},
		{"2d 4", `Did you mean "2d 4h"?`},
		{"1s 5", `Did you mean "1s 5ms"?`},
		{"1h 30 10", `Did you mean "1h 30m 10s"?`},
		// Valid strings or symbols without clear hints
		{"10s", ""},
		{"10s 5m", ""},
		{"10s : 5m", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SuggestHint(tt.input)
			if got != tt.expected {
				t.Errorf("SuggestHint(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
