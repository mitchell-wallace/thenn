package job

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ScheduleKind identifies the supported systemd-backed job schedule forms.
type ScheduleKind string

const (
	ScheduleEvery    ScheduleKind = "every"
	ScheduleDaily    ScheduleKind = "daily"
	ScheduleWeekdays ScheduleKind = "weekdays"
	ScheduleWeekly   ScheduleKind = "weekly"
	ScheduleOnce     ScheduleKind = "once"
)

// Schedule is the parsed representation needed to construct a systemd timer.
type Schedule struct {
	Kind ScheduleKind `json:"kind,omitempty"`

	// Interval and OnUnitActiveSec are set for "every" schedules.
	Interval        time.Duration `json:"interval,omitempty"`
	OnUnitActiveSec string        `json:"on_unit_active_sec,omitempty"`

	// OnCalendar is set for calendar schedules: daily, weekdays, weekly, and once.
	OnCalendar string `json:"on_calendar,omitempty"`

	// Until is set when the schedule includes an "until" limit.
	Until *time.Time `json:"until,omitempty"`
}

type parseConfig struct {
	now time.Time
}

// ParseOption customizes schedule parsing.
type ParseOption func(*parseConfig)

// WithNow sets the reference time used for time-only dates such as "until 9pm".
func WithNow(now time.Time) ParseOption {
	return func(cfg *parseConfig) {
		cfg.now = now
	}
}

var (
	durationPartRe  = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)(us|ms|s|m|h|d)`)
	time12Re        = regexp.MustCompile(`(?i)^(1[0-2]|[1-9])(?::([0-5]\d))?(am|pm|a|p)$`)
	time24Re        = regexp.MustCompile(`^(?:([01]?\d|2[0-3]):([0-5]\d))$`)
	yearFirstRe     = regexp.MustCompile(`^(\d{4})([-/.])(\d{1,2})([-/.])(\d{1,2})$`)
	startsYearRe    = regexp.MustCompile(`^\d{4}[-/.]`)
	ambiguousDateRe = regexp.MustCompile(`^\d{1,2}[-/.]\d{1,2}[-/.]\d{2,4}$`)
)

// ParseScheduleString splits input on whitespace and parses a schedule.
func ParseScheduleString(input string, opts ...ParseOption) (Schedule, error) {
	return ParseSchedule(strings.Fields(input), opts...)
}

// ParseSchedule parses verb-first schedule tokens into a systemd timer shape.
func ParseSchedule(tokens []string, opts ...ParseOption) (Schedule, error) {
	cfg := parseConfig{now: time.Now()}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.now.IsZero() {
		cfg.now = time.Now()
	}

	if len(tokens) == 0 {
		return Schedule{}, fmt.Errorf("empty schedule")
	}

	switch strings.ToLower(tokens[0]) {
	case "every":
		return parseEvery(tokens, cfg.now)
	case "daily":
		return parseDaily(tokens, cfg.now)
	case "weekdays":
		return parseWeekdays(tokens, cfg.now)
	case "weekly":
		return parseWeekly(tokens, cfg.now)
	case "once":
		return parseOnce(tokens, cfg.now)
	default:
		return Schedule{}, fmt.Errorf("unknown schedule %q; expected every, daily, weekdays, weekly, or once", tokens[0])
	}
}

func parseEvery(tokens []string, now time.Time) (Schedule, error) {
	parts, untilToken, err := splitUntil(tokens[1:])
	if err != nil {
		return Schedule{}, err
	}
	if len(parts) == 0 {
		return Schedule{}, fmt.Errorf("every schedule requires a duration, for example: every 15m")
	}

	interval, err := parseDuration(strings.Join(parts, ""))
	if err != nil {
		return Schedule{}, err
	}
	if interval <= 0 {
		return Schedule{}, fmt.Errorf("duration must be greater than zero")
	}

	schedule := Schedule{
		Kind:            ScheduleEvery,
		Interval:        interval,
		OnUnitActiveSec: formatSystemdDuration(interval),
	}
	if untilToken != "" {
		until, err := parseUntil(untilToken, now)
		if err != nil {
			return Schedule{}, err
		}
		schedule.Until = &until
	}
	return schedule, nil
}

func parseDaily(tokens []string, now time.Time) (Schedule, error) {
	timeToken, untilToken, err := parseAtSchedule(tokens, "daily at <time> [until <date-or-time>]")
	if err != nil {
		return Schedule{}, err
	}
	hour, minute, err := parseClock(timeToken)
	if err != nil {
		return Schedule{}, err
	}

	schedule := Schedule{Kind: ScheduleDaily, OnCalendar: fmt.Sprintf("*-*-* %02d:%02d:00", hour, minute)}
	if untilToken != "" {
		until, err := parseUntil(untilToken, now)
		if err != nil {
			return Schedule{}, err
		}
		schedule.Until = &until
	}
	return schedule, nil
}

func parseWeekdays(tokens []string, now time.Time) (Schedule, error) {
	timeToken, untilToken, err := parseAtSchedule(tokens, "weekdays at <time> [until <date-or-time>]")
	if err != nil {
		return Schedule{}, err
	}
	hour, minute, err := parseClock(timeToken)
	if err != nil {
		return Schedule{}, err
	}

	schedule := Schedule{Kind: ScheduleWeekdays, OnCalendar: fmt.Sprintf("Mon..Fri *-*-* %02d:%02d:00", hour, minute)}
	if untilToken != "" {
		until, err := parseUntil(untilToken, now)
		if err != nil {
			return Schedule{}, err
		}
		schedule.Until = &until
	}
	return schedule, nil
}

func parseWeekly(tokens []string, now time.Time) (Schedule, error) {
	if len(tokens) < 4 || !strings.EqualFold(tokens[2], "at") {
		return Schedule{}, fmt.Errorf("weekly schedule must be: weekly <weekday> at <time> [until <date-or-time>]")
	}
	weekday, err := parseWeekday(tokens[1])
	if err != nil {
		return Schedule{}, err
	}

	timeToken := tokens[3]
	untilToken := ""
	switch len(tokens) {
	case 4:
	case 6:
		if !strings.EqualFold(tokens[4], "until") {
			return Schedule{}, fmt.Errorf("weekly schedule must be: weekly <weekday> at <time> [until <date-or-time>]")
		}
		untilToken = tokens[5]
	default:
		return Schedule{}, fmt.Errorf("weekly schedule must be: weekly <weekday> at <time> [until <date-or-time>]")
	}

	hour, minute, err := parseClock(timeToken)
	if err != nil {
		return Schedule{}, err
	}

	schedule := Schedule{Kind: ScheduleWeekly, OnCalendar: fmt.Sprintf("%s *-*-* %02d:%02d:00", weekday, hour, minute)}
	if untilToken != "" {
		until, err := parseUntil(untilToken, now)
		if err != nil {
			return Schedule{}, err
		}
		schedule.Until = &until
	}
	return schedule, nil
}

func parseOnce(tokens []string, now time.Time) (Schedule, error) {
	if len(tokens) < 3 || !strings.EqualFold(tokens[1], "at") {
		return Schedule{}, fmt.Errorf("once schedule must be: once at <time-or-date> or once at <YYYY-MM-DD> <time>")
	}
	when, err := parseOneShot(tokens[2:], now)
	if err != nil {
		return Schedule{}, err
	}
	return Schedule{Kind: ScheduleOnce, OnCalendar: formatOnCalendarTime(when)}, nil
}

func parseAtSchedule(tokens []string, usage string) (timeToken, untilToken string, err error) {
	if len(tokens) < 3 || !strings.EqualFold(tokens[1], "at") {
		return "", "", fmt.Errorf("schedule must be: %s", usage)
	}
	timeToken = tokens[2]
	switch len(tokens) {
	case 3:
		return timeToken, "", nil
	case 5:
		if !strings.EqualFold(tokens[3], "until") {
			return "", "", fmt.Errorf("schedule must be: %s", usage)
		}
		return timeToken, tokens[4], nil
	default:
		return "", "", fmt.Errorf("schedule must be: %s", usage)
	}
}

func splitUntil(tokens []string) ([]string, string, error) {
	untilAt := -1
	for i, token := range tokens {
		if strings.EqualFold(token, "until") {
			if untilAt != -1 {
				return nil, "", fmt.Errorf("schedule can include only one until clause")
			}
			untilAt = i
		}
	}
	if untilAt == -1 {
		return tokens, "", nil
	}
	if untilAt == 0 {
		return nil, "", fmt.Errorf("every schedule requires a duration before until")
	}
	if untilAt != len(tokens)-2 {
		return nil, "", fmt.Errorf("until must be followed by one date or time")
	}
	return tokens[:untilAt], tokens[untilAt+1], nil
}

func parseDuration(input string) (time.Duration, error) {
	if strings.TrimSpace(input) == "" {
		return 0, fmt.Errorf("empty duration")
	}

	matches := durationPartRe.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return 0, fmt.Errorf("invalid duration %q; use units like 15m, 2h, or 1d", input)
	}

	var total time.Duration
	pos := 0
	for _, match := range matches {
		if match[0] != pos {
			return 0, fmt.Errorf("invalid duration %q; use units like 15m, 2h, or 1d", input)
		}
		value := input[match[2]:match[3]]
		unit := strings.ToLower(input[match[4]:match[5]])
		amount, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration number %q", value)
		}

		var unitDuration time.Duration
		switch unit {
		case "us":
			unitDuration = time.Microsecond
		case "ms":
			unitDuration = time.Millisecond
		case "s":
			unitDuration = time.Second
		case "m":
			unitDuration = time.Minute
		case "h":
			unitDuration = time.Hour
		case "d":
			unitDuration = 24 * time.Hour
		default:
			return 0, fmt.Errorf("unknown duration unit %q", unit)
		}
		total += time.Duration(amount * float64(unitDuration))
		pos = match[1]
	}
	if pos != len(input) {
		return 0, fmt.Errorf("invalid duration %q; use units like 15m, 2h, or 1d", input)
	}
	if total > 0 && total%time.Microsecond != 0 {
		return 0, fmt.Errorf("duration %q is below systemd timer precision; use microseconds or larger", input)
	}
	return total, nil
}

func formatSystemdDuration(d time.Duration) string {
	switch {
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", d/(24*time.Hour))
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", d/time.Minute)
	case d%time.Second == 0:
		return fmt.Sprintf("%ds", d/time.Second)
	case d%time.Millisecond == 0:
		return fmt.Sprintf("%dms", d/time.Millisecond)
	case d%time.Microsecond == 0:
		return fmt.Sprintf("%dus", d/time.Microsecond)
	default:
		return fmt.Sprintf("%dus", d/time.Microsecond)
	}
}

func parseClock(input string) (int, int, error) {
	clean := strings.ToLower(strings.TrimSpace(input))
	if match := time12Re.FindStringSubmatch(clean); match != nil {
		hour, _ := strconv.Atoi(match[1])
		minute := 0
		if match[2] != "" {
			minute, _ = strconv.Atoi(match[2])
		}
		period := match[3]
		if period == "pm" || period == "p" {
			if hour != 12 {
				hour += 12
			}
		} else if hour == 12 {
			hour = 0
		}
		return hour, minute, nil
	}

	if match := time24Re.FindStringSubmatch(clean); match != nil {
		hour, _ := strconv.Atoi(match[1])
		minute, _ := strconv.Atoi(match[2])
		return hour, minute, nil
	}

	return 0, 0, fmt.Errorf("invalid time %q; use formats like 9pm, 9:30pm, 21:00, or 09:30", input)
}

func parseUntil(input string, now time.Time) (time.Time, error) {
	if hour, minute, err := parseClock(input); err == nil {
		until := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
		if !until.After(now) {
			return time.Time{}, fmt.Errorf("until time %q has already passed today; use a future time or a year-first date", input)
		}
		return until, nil
	}

	date, err := parseDate(input)
	if err != nil {
		return time.Time{}, err
	}
	until := time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 0, now.Location())
	if !until.After(now) {
		return time.Time{}, fmt.Errorf("until date %q has already passed", input)
	}
	return until, nil
}

func parseOneShot(tokens []string, now time.Time) (time.Time, error) {
	if len(tokens) == 2 {
		date, err := parseDate(tokens[0])
		if err != nil {
			return time.Time{}, err
		}
		hour, minute, err := parseClock(tokens[1])
		if err != nil {
			return time.Time{}, err
		}
		when := time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, now.Location())
		if !when.After(now) {
			return time.Time{}, fmt.Errorf("time %q on date %q has already passed", tokens[1], tokens[0])
		}
		return when, nil
	}
	if len(tokens) != 1 {
		return time.Time{}, fmt.Errorf("once schedule must be: once at <time-or-date> or once at <YYYY-MM-DD> <time>")
	}

	input := tokens[0]
	if hour, minute, err := parseClock(input); err == nil {
		when := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
		if !when.After(now) {
			when = when.AddDate(0, 0, 1)
		}
		return when, nil
	}

	date, err := parseDate(input)
	if err != nil {
		return time.Time{}, err
	}
	when := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, now.Location())
	if !when.After(now) {
		return time.Time{}, fmt.Errorf("date %q has already passed", input)
	}
	return when, nil
}

func parseDate(input string) (time.Time, error) {
	clean := strings.TrimSpace(input)
	if ambiguousDateRe.MatchString(clean) {
		return time.Time{}, fmt.Errorf("ambiguous date %q; use year-first format YYYY-MM-DD", input)
	}

	match := yearFirstRe.FindStringSubmatch(clean)
	if match == nil {
		if startsYearRe.MatchString(clean) {
			return time.Time{}, fmt.Errorf("invalid date %q; use one separator consistently in YYYY-MM-DD, YYYY/MM/DD, or YYYY.MM.DD", input)
		}
		return time.Time{}, fmt.Errorf("invalid date %q; use year-first format YYYY-MM-DD", input)
	}
	if match[2] != match[4] {
		return time.Time{}, fmt.Errorf("invalid date %q; use one separator consistently in YYYY-MM-DD, YYYY/MM/DD, or YYYY.MM.DD", input)
	}

	year, _ := strconv.Atoi(match[1])
	month, _ := strconv.Atoi(match[3])
	day, _ := strconv.Atoi(match[5])
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local)
	if date.Year() != year || int(date.Month()) != month || date.Day() != day {
		return time.Time{}, fmt.Errorf("invalid date %q; use a real date in YYYY-MM-DD format", input)
	}
	return date, nil
}

func parseWeekday(input string) (string, error) {
	switch strings.ToLower(input) {
	case "mon", "monday":
		return "Mon", nil
	case "tue", "tues", "tuesday":
		return "Tue", nil
	case "wed", "wednesday":
		return "Wed", nil
	case "thu", "thur", "thurs", "thursday":
		return "Thu", nil
	case "fri", "friday":
		return "Fri", nil
	case "sat", "saturday":
		return "Sat", nil
	case "sun", "sunday":
		return "Sun", nil
	default:
		return "", fmt.Errorf("invalid weekday %q; use Monday, Tuesday, Wednesday, Thursday, Friday, Saturday, or Sunday", input)
	}
}

func formatOnCalendarTime(t time.Time) string {
	return fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())
}
