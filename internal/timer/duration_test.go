package timer

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"10s", 10 * time.Second, false},
		{"2h 13m 55s", 2*time.Hour + 13*time.Minute + 55*time.Second, false},
		{"1d 2h", 26 * time.Hour, false},
		{"100ms", 100 * time.Millisecond, false},
		{"1h2m3s", 1*time.Hour + 2*time.Minute + 3*time.Second, false},
		{"5s 10m", 10*time.Minute + 5*time.Second, false},
		{"10S 2H", 2*time.Hour + 10*time.Second, false},
		{"1d 2h 3m 4s 5ms", 24*time.Hour + 2*time.Hour + 3*time.Minute + 4*time.Second + 5*time.Millisecond, false},
		{"", 0, true},
		{"invalid", 0, true},
		{"10", 0, true},
		{"10s 2h extra", 0, true},
		{"10s : 2h", 0, true},
		{"10x", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			res, err := ParseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && res != tt.expected {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, res, tt.expected)
			}
		})
	}
}
