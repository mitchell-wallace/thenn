package job

import (
	"fmt"
	"strings"
	"time"
)

// Metadata is the persisted definition for a scheduled thenn job.
type Metadata struct {
	Label          string    `json:"label"`
	OriginalPhrase string    `json:"original_phrase"`
	ParsedSchedule Schedule  `json:"parsed_schedule"`
	CommandArgv    []string  `json:"command_argv"`
	CWD            string    `json:"cwd"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// NewParsedSchedule validates and wraps a systemd OnCalendar expression.
func NewParsedSchedule(onCalendar string) (Schedule, error) {
	onCalendar = strings.TrimSpace(onCalendar)
	if onCalendar == "" {
		return Schedule{}, fmt.Errorf("schedule on_calendar is required")
	}
	if strings.ContainsAny(onCalendar, "\r\n") {
		return Schedule{}, fmt.Errorf("schedule on_calendar cannot contain newlines")
	}
	return Schedule{OnCalendar: onCalendar}, nil
}

// NewMetadata creates validated metadata with initialized timestamps.
func NewMetadata(label, originalPhrase string, schedule Schedule, argv []string, cwd string, now time.Time) (Metadata, error) {
	metadata := Metadata{
		Label:          label,
		OriginalPhrase: strings.TrimSpace(originalPhrase),
		ParsedSchedule: schedule,
		CommandArgv:    append([]string(nil), argv...),
		CWD:            cwd,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := metadata.Validate(); err != nil {
		return Metadata{}, err
	}
	return metadata, nil
}

// Validate checks that required metadata fields are safe to persist and render.
func (m Metadata) Validate() error {
	if err := ValidateLabel(m.Label); err != nil {
		return err
	}
	if strings.TrimSpace(m.OriginalPhrase) == "" {
		return fmt.Errorf("original phrase is required")
	}
	if err := validateSchedule(m.ParsedSchedule); err != nil {
		return err
	}
	if len(m.CommandArgv) == 0 || strings.TrimSpace(m.CommandArgv[0]) == "" {
		return fmt.Errorf("command argv is required")
	}
	for _, arg := range m.CommandArgv {
		if strings.ContainsAny(arg, "\x00") {
			return fmt.Errorf("command argv cannot contain NUL bytes")
		}
	}
	if strings.TrimSpace(m.CWD) == "" {
		return fmt.Errorf("cwd is required")
	}
	if strings.ContainsAny(m.CWD, "\x00\r\n") {
		return fmt.Errorf("cwd contains invalid characters")
	}
	if m.CreatedAt.IsZero() {
		return fmt.Errorf("created_at is required")
	}
	if m.UpdatedAt.IsZero() {
		return fmt.Errorf("updated_at is required")
	}
	return nil
}

func validateSchedule(schedule Schedule) error {
	if strings.TrimSpace(schedule.OnCalendar) == "" && strings.TrimSpace(schedule.OnUnitActiveSec) == "" {
		return fmt.Errorf("parsed schedule requires on_calendar or on_unit_active_sec")
	}
	if schedule.OnCalendar != "" && schedule.OnUnitActiveSec != "" {
		return fmt.Errorf("parsed schedule cannot contain both on_calendar and on_unit_active_sec")
	}
	if strings.ContainsAny(schedule.OnCalendar, "\r\n") {
		return fmt.Errorf("schedule on_calendar cannot contain newlines")
	}
	if strings.ContainsAny(schedule.OnUnitActiveSec, "\r\n") {
		return fmt.Errorf("schedule on_unit_active_sec cannot contain newlines")
	}
	return nil
}
