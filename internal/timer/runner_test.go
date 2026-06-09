package timer

import (
	"strings"
	"testing"
	"time"
)

func TestFormatRemaining(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{0, "0s"},
		{-5 * time.Second, "0s"},
		{5 * time.Second, "5s"},
		{65 * time.Second, "1m 5s"},
		{3600 * time.Second, "1h"},
		{3665 * time.Second, "1h 1m 5s"},
		{25 * time.Hour, "1d 1h"},
		{25*time.Hour + 5*time.Second, "1d 1h 5s"},
		{25*time.Hour + 2*time.Minute + 5*time.Second, "1d 1h 2m 5s"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := FormatRemaining(tt.d)
			if got != tt.expected {
				t.Errorf("FormatRemaining(%v) = %q, want %q", tt.d, got, tt.expected)
			}
		})
	}
}

func TestGetDayString(t *testing.T) {
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.Local)

	tests := []struct {
		target   time.Time
		expected string
	}{
		{time.Date(2026, 6, 4, 15, 0, 0, 0, time.Local), "today"},
		{time.Date(2026, 6, 5, 2, 0, 0, 0, time.Local), "tomorrow"},
		{time.Date(2026, 6, 6, 12, 0, 0, 0, time.Local), "2026.06.06"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := GetDayString(tt.target, now)
			if got != tt.expected {
				t.Errorf("GetDayString(%v, %v) = %q, want %q", tt.target, now, got, tt.expected)
			}
		})
	}
}

func TestFormatEndTime(t *testing.T) {
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.Local)

	tests := []struct {
		target   time.Time
		expected string
	}{
		{time.Date(2026, 6, 4, 19, 12, 0, 0, time.Local), "7:12p today"},
		{time.Date(2026, 6, 4, 7, 12, 0, 0, time.Local), "7:12a today"},
		{time.Date(2026, 6, 5, 11, 5, 0, 0, time.Local), "11:05a tomorrow"},
		{time.Date(2026, 6, 6, 23, 59, 0, 0, time.Local), "11:59p 2026.06.06"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := FormatEndTime(tt.target, now)
			if got != tt.expected {
				t.Errorf("FormatEndTime(%v, %v) = %q, want %q", tt.target, now, got, tt.expected)
			}
		})
	}
}

func TestFormatCommand(t *testing.T) {
	tests := []struct {
		cmd      []string
		expected string
	}{
		{[]string{}, ""},
		{[]string{"echo", "hello"}, "echo hello"},
		{[]string{"echo", "hello world"}, "echo \"hello world\""},
		{[]string{"sh", "-c", "echo 'done'"}, "echo 'done'"},
		{[]string{"/bin/bash", "-c", "ls -la"}, "ls -la"},
		{[]string{"cmd.exe", "/c", "dir"}, "dir"},
		{[]string{"powershell", "-c", "Get-Process"}, "Get-Process"},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.cmd, " "), func(t *testing.T) {
			got := FormatCommand(tt.cmd)
			if got != tt.expected {
				t.Errorf("FormatCommand(%v) = %q, want %q", tt.cmd, got, tt.expected)
			}
		})
	}
}

