package timer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseDuration parses a string like "2h 13m 55s" or "1d 2h" into a time.Duration.
// It supports floating point numbers like "1.5h" or "0.5m".
func ParseDuration(s string) (time.Duration, error) {
	// Strip all whitespace
	clean := strings.Join(strings.Fields(s), "")
	if clean == "" {
		return 0, fmt.Errorf("empty duration")
	}

	re := regexp.MustCompile(`(\d+(?:\.\d+)?)([a-zA-Z]+)`)
	matches := re.FindAllStringSubmatch(clean, -1)
	if len(matches) == 0 {
		hint := SuggestHint(s)
		if hint != "" {
			return 0, fmt.Errorf("invalid duration format: %q. %s", s, hint)
		}
		return 0, fmt.Errorf("invalid duration format: %q", s)
	}

	// Verify that the matches completely cover the cleaned string to detect syntax errors
	matchedLen := 0
	for _, m := range matches {
		matchedLen += len(m[0])
	}
	if matchedLen != len(clean) {
		hint := SuggestHint(s)
		if hint != "" {
			return 0, fmt.Errorf("invalid duration format: %q. %s", s, hint)
		}
		return 0, fmt.Errorf("invalid duration format: %q", s)
	}

	var total time.Duration
	for _, m := range matches {
		valStr := m[1]
		unit := strings.ToLower(m[2])

		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid number %q: %w", valStr, err)
		}

		var d time.Duration
		switch unit {
		case "d", "day", "days":
			d = time.Duration(val * float64(24*time.Hour))
		case "h", "hr", "hrs", "hour", "hours":
			d = time.Duration(val * float64(time.Hour))
		case "m", "min", "mins", "minute", "minutes":
			d = time.Duration(val * float64(time.Minute))
		case "s", "sec", "secs", "second", "seconds":
			d = time.Duration(val * float64(time.Second))
		case "ms", "msec", "msecs", "millisecond", "milliseconds":
			d = time.Duration(val * float64(time.Millisecond))
		default:
			return 0, fmt.Errorf("unknown unit %q in duration %q", unit, s)
		}
		total += d
	}

	return total, nil
}

// SuggestHint analyzes an invalid duration string and returns a suggestion, if possible.
func SuggestHint(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}

	partRe := regexp.MustCompile(`^(\d+(?:\.\d+)?)([a-zA-Z]*)$`)

	var suggestions []string
	var lastUnit string

	hasMissingUnit := false
	allNumbersOnly := true

	for _, field := range fields {
		matches := partRe.FindStringSubmatch(field)
		if len(matches) == 0 {
			return ""
		}

		numStr := matches[1]
		unitStr := strings.ToLower(matches[2])

		if unitStr != "" {
			allNumbersOnly = false
			lastUnit = unitStr
			suggestions = append(suggestions, numStr+unitStr)
		} else {
			hasMissingUnit = true
			var suggestedUnit string
			switch lastUnit {
			case "d", "day", "days":
				suggestedUnit = "h"
			case "h", "hr", "hrs", "hour", "hours":
				suggestedUnit = "m"
			case "m", "min", "mins", "minute", "minutes":
				suggestedUnit = "s"
			case "s", "sec", "secs", "second", "seconds":
				suggestedUnit = "ms"
			default:
				suggestedUnit = ""
			}
			if suggestedUnit != "" {
				suggestions = append(suggestions, numStr+suggestedUnit)
				lastUnit = suggestedUnit
			} else {
				suggestions = append(suggestions, numStr)
			}
		}
	}

	if !hasMissingUnit {
		return ""
	}

	if allNumbersOnly {
		if len(fields) == 1 {
			val := fields[0]
			return fmt.Sprintf("Did you mean %q, %q, or %q?", val+"s", val+"m", val+"h")
		}
		return ""
	}

	suggestedStr := strings.Join(suggestions, " ")
	return fmt.Sprintf("Did you mean %q?", suggestedStr)
}
