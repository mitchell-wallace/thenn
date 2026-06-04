package timer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseDuration parses a string like "2h 13m 55s" or "1d 2h" into a time.Duration.
func ParseDuration(s string) (time.Duration, error) {
	// Strip all whitespace
	clean := strings.Join(strings.Fields(s), "")
	if clean == "" {
		return 0, fmt.Errorf("empty duration")
	}

	re := regexp.MustCompile(`(\d+)([a-zA-Z]+)`)
	matches := re.FindAllStringSubmatch(clean, -1)
	if len(matches) == 0 {
		return 0, fmt.Errorf("invalid duration format: %q", s)
	}

	// Verify that the matches completely cover the cleaned string to detect syntax errors
	matchedLen := 0
	for _, m := range matches {
		matchedLen += len(m[0])
	}
	if matchedLen != len(clean) {
		return 0, fmt.Errorf("invalid duration format: %q", s)
	}

	var total time.Duration
	for _, m := range matches {
		valStr := m[1]
		unit := strings.ToLower(m[2])

		val, err := strconv.ParseInt(valStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid number %q: %w", valStr, err)
		}

		var d time.Duration
		switch unit {
		case "d", "day", "days":
			d = time.Duration(val) * 24 * time.Hour
		case "h", "hr", "hrs", "hour", "hours":
			d = time.Duration(val) * time.Hour
		case "m", "min", "mins", "minute", "minutes":
			d = time.Duration(val) * time.Minute
		case "s", "sec", "secs", "second", "seconds":
			d = time.Duration(val) * time.Second
		case "ms", "msec", "msecs", "millisecond", "milliseconds":
			d = time.Duration(val) * time.Millisecond
		default:
			return 0, fmt.Errorf("unknown unit %q in duration %q", unit, s)
		}
		total += d
	}

	return total, nil
}
