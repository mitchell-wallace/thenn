package job

import (
	"strings"
	"testing"
	"time"
)

func fixedNow() time.Time {
	return time.Date(2026, 7, 5, 12, 0, 0, 0, time.Local)
}

func TestParseScheduleEvery(t *testing.T) {
	now := fixedNow()
	tests := []struct {
		name            string
		tokens          []string
		wantInterval    time.Duration
		wantActiveSec   string
		wantUntil       *time.Time
		wantErrContains string
	}{
		{
			name:          "minutes",
			tokens:        []string{"every", "15m"},
			wantInterval:  15 * time.Minute,
			wantActiveSec: "15m",
		},
		{
			name:          "space separated duration parts",
			tokens:        []string{"every", "1h", "30m"},
			wantInterval:  90 * time.Minute,
			wantActiveSec: "90m",
		},
		{
			name:          "days",
			tokens:        []string{"every", "2d"},
			wantInterval:  48 * time.Hour,
			wantActiveSec: "2d",
		},
		{
			name:          "go duration unit",
			tokens:        []string{"every", "500ms"},
			wantInterval:  500 * time.Millisecond,
			wantActiveSec: "500ms",
		},
		{
			name:          "microseconds",
			tokens:        []string{"every", "1500us"},
			wantInterval:  1500 * time.Microsecond,
			wantActiveSec: "1500us",
		},
		{
			name:          "until time today",
			tokens:        []string{"every", "1h", "until", "9pm"},
			wantInterval:  time.Hour,
			wantActiveSec: "1h",
			wantUntil:     timePtr(time.Date(2026, 7, 5, 21, 0, 0, 0, time.Local)),
		},
		{
			name:          "until date uses end of day",
			tokens:        []string{"every", "1d", "until", "2026/07/06"},
			wantInterval:  24 * time.Hour,
			wantActiveSec: "1d",
			wantUntil:     timePtr(time.Date(2026, 7, 6, 23, 59, 59, 0, time.Local)),
		},
		{
			name:            "missing duration",
			tokens:          []string{"every"},
			wantErrContains: "requires a duration",
		},
		{
			name:            "zero duration",
			tokens:          []string{"every", "0m"},
			wantErrContains: "greater than zero",
		},
		{
			name:            "bad duration",
			tokens:          []string{"every", "ten"},
			wantErrContains: "invalid duration",
		},
		{
			name:            "unsupported nanoseconds",
			tokens:          []string{"every", "1ns"},
			wantErrContains: "invalid duration",
		},
		{
			name:            "below systemd precision",
			tokens:          []string{"every", "0.5us"},
			wantErrContains: "below systemd timer precision",
		},
		{
			name:            "fractional microsecond does not truncate",
			tokens:          []string{"every", "1.0000001us"},
			wantErrContains: "below nanosecond precision",
		},
		{
			name:            "huge duration",
			tokens:          []string{"every", "999999999999999999999h"},
			wantErrContains: "too large",
		},
		{
			name:            "until without value",
			tokens:          []string{"every", "1h", "until"},
			wantErrContains: "until must be followed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSchedule(tt.tokens, WithNow(now))
			if tt.wantErrContains != "" {
				assertErrContains(t, err, tt.wantErrContains)
				return
			}
			if err != nil {
				t.Fatalf("ParseSchedule() error = %v", err)
			}
			if got.Kind != ScheduleEvery {
				t.Fatalf("Kind = %q, want %q", got.Kind, ScheduleEvery)
			}
			if got.Interval != tt.wantInterval {
				t.Errorf("Interval = %v, want %v", got.Interval, tt.wantInterval)
			}
			if got.OnUnitActiveSec != tt.wantActiveSec {
				t.Errorf("OnUnitActiveSec = %q, want %q", got.OnUnitActiveSec, tt.wantActiveSec)
			}
			assertTimePtr(t, got.Until, tt.wantUntil)
			if got.OnCalendar != "" {
				t.Errorf("OnCalendar = %q, want empty", got.OnCalendar)
			}
		})
	}
}

func TestParseScheduleCalendar(t *testing.T) {
	now := fixedNow()
	tests := []struct {
		name            string
		input           string
		wantKind        ScheduleKind
		wantOnCalendar  string
		wantUntil       *time.Time
		wantErrContains string
	}{
		{
			name:           "daily 12 hour",
			input:          "daily at 9:30pm",
			wantKind:       ScheduleDaily,
			wantOnCalendar: "*-*-* 21:30:00",
		},
		{
			name:           "daily 24 hour with until",
			input:          "daily at 09:30 until 2026-07-10",
			wantKind:       ScheduleDaily,
			wantOnCalendar: "*-*-* 09:30:00",
			wantUntil:      timePtr(time.Date(2026, 7, 10, 23, 59, 59, 0, time.Local)),
		},
		{
			name:           "weekdays",
			input:          "weekdays at 21:00",
			wantKind:       ScheduleWeekdays,
			wantOnCalendar: "Mon..Fri *-*-* 21:00:00",
		},
		{
			name:           "weekdays with short am",
			input:          "weekdays at 9a until 6pm",
			wantKind:       ScheduleWeekdays,
			wantOnCalendar: "Mon..Fri *-*-* 09:00:00",
			wantUntil:      timePtr(time.Date(2026, 7, 5, 18, 0, 0, 0, time.Local)),
		},
		{
			name:           "weekly long weekday",
			input:          "weekly Monday at 9pm",
			wantKind:       ScheduleWeekly,
			wantOnCalendar: "Mon *-*-* 21:00:00",
		},
		{
			name:           "weekly short weekday",
			input:          "weekly thu at 09:30 until 2026.07.08",
			wantKind:       ScheduleWeekly,
			wantOnCalendar: "Thu *-*-* 09:30:00",
			wantUntil:      timePtr(time.Date(2026, 7, 8, 23, 59, 59, 0, time.Local)),
		},
		{
			name:            "daily missing at",
			input:           "daily 9pm",
			wantErrContains: "daily at <time>",
		},
		{
			name:            "weekdays extra tokens",
			input:           "weekdays at 9pm tomorrow",
			wantErrContains: "weekdays at <time>",
		},
		{
			name:            "weekly invalid weekday",
			input:           "weekly someday at 9pm",
			wantErrContains: "invalid weekday",
		},
		{
			name:            "weekly missing at",
			input:           "weekly Monday 9pm",
			wantErrContains: "weekly <weekday> at <time>",
		},
		{
			name:            "invalid time",
			input:           "daily at 25:00",
			wantErrContains: "invalid time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseScheduleString(tt.input, WithNow(now))
			if tt.wantErrContains != "" {
				assertErrContains(t, err, tt.wantErrContains)
				return
			}
			if err != nil {
				t.Fatalf("ParseScheduleString() error = %v", err)
			}
			if got.Kind != tt.wantKind {
				t.Fatalf("Kind = %q, want %q", got.Kind, tt.wantKind)
			}
			if got.OnCalendar != tt.wantOnCalendar {
				t.Errorf("OnCalendar = %q, want %q", got.OnCalendar, tt.wantOnCalendar)
			}
			assertTimePtr(t, got.Until, tt.wantUntil)
			if got.Interval != 0 || got.OnUnitActiveSec != "" {
				t.Errorf("unexpected interval fields: %v %q", got.Interval, got.OnUnitActiveSec)
			}
		})
	}
}

func TestParseScheduleOnce(t *testing.T) {
	now := fixedNow()
	tests := []struct {
		name            string
		input           string
		wantOnCalendar  string
		wantErrContains string
	}{
		{
			name:           "time only today",
			input:          "once at 9pm",
			wantOnCalendar: "2026-07-05 21:00:00",
		},
		{
			name:           "date with dash",
			input:          "once at 2026-07-06",
			wantOnCalendar: "2026-07-06 00:00:00",
		},
		{
			name:           "date with slash",
			input:          "once at 2026/07/06",
			wantOnCalendar: "2026-07-06 00:00:00",
		},
		{
			name:           "date with dot",
			input:          "once at 2026.07.06",
			wantOnCalendar: "2026-07-06 00:00:00",
		},
		{
			name:           "date and time",
			input:          "once at 2026-07-06 9:30pm",
			wantOnCalendar: "2026-07-06 21:30:00",
		},
		{
			name:           "time already passed uses tomorrow",
			input:          "once at 9am",
			wantOnCalendar: "2026-07-06 09:00:00",
		},
		{
			name:            "past date",
			input:           "once at 2026-07-04",
			wantErrContains: "already passed",
		},
		{
			name:            "bad grammar",
			input:           "once 9pm",
			wantErrContains: "once at <time-or-date>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseScheduleString(tt.input, WithNow(now))
			if tt.wantErrContains != "" {
				assertErrContains(t, err, tt.wantErrContains)
				return
			}
			if err != nil {
				t.Fatalf("ParseScheduleString() error = %v", err)
			}
			if got.Kind != ScheduleOnce {
				t.Fatalf("Kind = %q, want %q", got.Kind, ScheduleOnce)
			}
			if got.OnCalendar != tt.wantOnCalendar {
				t.Errorf("OnCalendar = %q, want %q", got.OnCalendar, tt.wantOnCalendar)
			}
		})
	}
}

func TestParseScheduleRejectsAmbiguousDates(t *testing.T) {
	now := fixedNow()
	tests := []struct {
		input           string
		wantErrContains string
	}{
		{"daily at 9pm until 07-06-2026", "ambiguous date"},
		{"once at 06/07/2026", "ambiguous date"},
		{"once at 2026-13-01", "real date"},
		{"once at 2026-02-30", "real date"},
		{"once at 2026-07/06", "one separator consistently"},
		{"daily at 9pm until 11am", "already passed today"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := ParseScheduleString(tt.input, WithNow(now))
			assertErrContains(t, err, tt.wantErrContains)
		})
	}
}

func TestParseScheduleUnknownVerb(t *testing.T) {
	_, err := ParseScheduleString("monthly at 9pm", WithNow(fixedNow()))
	assertErrContains(t, err, "unknown schedule")
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func assertErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want substring %q", err.Error(), want)
	}
}

func assertTimePtr(t *testing.T, got, want *time.Time) {
	t.Helper()
	if got == nil || want == nil {
		if got != want {
			t.Fatalf("time pointer = %v, want %v", got, want)
		}
		return
	}
	if !got.Equal(*want) {
		t.Fatalf("time = %v, want %v", *got, *want)
	}
}
