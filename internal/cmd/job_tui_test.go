package cmd

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mitchell-wallace/thenn/internal/job"
)

func TestJobTUIViewShowsJobsAndShortcuts(t *testing.T) {
	m := testJobTUIModel(t)
	view := m.View()
	for _, want := range []string{"thenn job", "backup", "daily at 9pm", "p pause", "r resume", "tab help"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestJobTUIViewHelpTabShowsSyntax(t *testing.T) {
	m := testJobTUIModel(t)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	model := updated.(*jobTUIModel)
	view := model.View()
	for _, want := range []string{"Schedule syntax", "every 3h until 2026-07-23", "Dates are always year-first"} {
		if !strings.Contains(view, want) {
			t.Fatalf("help view missing %q:\n%s", want, view)
		}
	}
}

func testJobTUIModel(t *testing.T) *jobTUIModel {
	t.Helper()
	schedule, err := job.ParseScheduleString("daily at 9pm", job.WithNow(time.Date(2026, 7, 5, 12, 0, 0, 0, time.Local)))
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := job.NewMetadata("backup", "daily at 9pm", schedule, []string{"echo", "ok"}, "/tmp", time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return &jobTUIModel{
		jobs:   []job.Metadata{metadata},
		width:  100,
		height: 28,
	}
}
