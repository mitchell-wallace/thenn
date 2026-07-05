package job

import (
	"fmt"
	"regexp"
	"strings"
)

const maxLabelLength = 80

var labelRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)

// ValidateLabel accepts simple labels safe for filenames and systemd unit names.
func ValidateLabel(label string) error {
	if label == "" {
		return fmt.Errorf("label is required")
	}
	if strings.TrimSpace(label) != label {
		return fmt.Errorf("label cannot contain leading or trailing whitespace")
	}
	if !labelRe.MatchString(label) {
		return fmt.Errorf("label %q must start with a letter or number and contain only letters, numbers, dots, underscores, or hyphens", label)
	}
	return nil
}

// SanitizeLabel converts arbitrary text into a simple label.
func SanitizeLabel(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	var b strings.Builder
	lastWasSep := false

	for _, r := range input {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		sep := r == '.' || r == '_' || r == '-'

		if valid {
			b.WriteRune(r)
			lastWasSep = false
			continue
		}
		if sep && b.Len() > 0 && !lastWasSep {
			b.WriteRune(r)
			lastWasSep = true
			continue
		}
		if b.Len() > 0 && !lastWasSep {
			b.WriteByte('-')
			lastWasSep = true
		}
	}

	label := strings.Trim(b.String(), ".-_")
	if len(label) > maxLabelLength {
		label = strings.Trim(label[:maxLabelLength], ".-_")
	}
	if label == "" {
		return "job"
	}
	return label
}
