package cmd

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mitchell-wallace/thenn/internal/timer"
)

var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true)

	focusedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205"))

	blurredStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	docStyle = lipgloss.NewStyle().Margin(1, 2)

	bannerStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1).
			Width(64).
			Foreground(lipgloss.Color("252"))

	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			Italic(true)

	submitActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("229")).
				Background(lipgloss.Color("62")).
				Bold(true).
				Padding(0, 3).
				MarginTop(1)

	submitInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("244")).
				Background(lipgloss.Color("236")).
				Padding(0, 3).
				MarginTop(1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))
)

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(4*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type model struct {
	durationInput textinput.Model
	commandInput  textinput.Model
	focusIndex    int // 0: duration, 1: command, 2: submit button
	quitting      bool
	aborted       bool
	err           error

	hints       []string
	hintIndex   int
	hintsHidden bool
}

func initialModel(prepopulatedCmd string) model {
	d := textinput.New()
	d.Placeholder = "e.g. 10s, 5m, 1500, 3:00p"
	d.Focus()
	d.Prompt = focusedStyle.Render("> ")
	d.TextStyle = focusedStyle

	c := textinput.New()
	c.Placeholder = "e.g. echo 'done'"
	c.Prompt = blurredStyle.Render("> ")
	c.SetValue(prepopulatedCmd)

	hints := []string{
		"Run a coding agent after a usage limit resets (e.g., codex exec \"Fix broken tests\")",
		"Use continue commands to auto-resume agents (e.g., claude -c \"continue\")",
		"Press Spacebar while the timer is running to pause the countdown",
	}

	return model{
		durationInput: d,
		commandInput:  c,
		focusIndex:    0,
		hints:         hints,
		hintIndex:     0,
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tickCmd())
}

func (m *model) updateFocus() tea.Cmd {
	m.durationInput.Blur()
	m.commandInput.Blur()
	m.durationInput.Prompt = blurredStyle.Render("> ")
	m.commandInput.Prompt = blurredStyle.Render("> ")

	switch m.focusIndex {
	case 0:
		m.durationInput.Focus()
		m.durationInput.Prompt = focusedStyle.Render("> ")
	case 1:
		m.commandInput.Focus()
		m.commandInput.Prompt = focusedStyle.Render("> ")
	}
	return nil
}

func (m *model) submit() tea.Cmd {
	val := strings.TrimSpace(m.durationInput.Value())
	if val == "" {
		m.err = errors.New("duration is required")
		return nil
	}
	_, err := timer.ParseDurationOrTarget(val, time.Now())
	if err != nil {
		m.err = err
		return nil
	}
	m.err = nil
	m.quitting = true
	return tea.Quit
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.aborted = true
			m.quitting = true
			return m, tea.Quit

		case "esc":
			m.hintsHidden = !m.hintsHidden
			return m, nil

		case "tab", "down":
			m.focusIndex = (m.focusIndex + 1) % 3
			cmd := m.updateFocus()
			return m, cmd

		case "shift+tab", "up":
			m.focusIndex = (m.focusIndex - 1 + 3) % 3
			cmd := m.updateFocus()
			return m, cmd

		case "enter":
			if m.focusIndex == 2 {
				cmd := m.submit()
				return m, cmd
			}
			if m.focusIndex == 0 {
				val := strings.TrimSpace(m.durationInput.Value())
				if val == "" {
					m.err = errors.New("duration is required")
					return m, nil
				}
				_, err := timer.ParseDurationOrTarget(val, time.Now())
				if err != nil {
					m.err = err
					return m, nil
				}
				m.err = nil
				m.focusIndex = 1
				cmd := m.updateFocus()
				return m, cmd
			}
			if m.focusIndex == 1 {
				m.focusIndex = 2
				cmd := m.updateFocus()
				return m, cmd
			}
		}

	case tickMsg:
		if !m.quitting {
			m.hintIndex = (m.hintIndex + 1) % len(m.hints)
			return m, tickCmd()
		}
	}

	var cmd tea.Cmd
	switch m.focusIndex {
	case 0:
		m.durationInput, cmd = m.durationInput.Update(msg)
	case 1:
		m.commandInput, cmd = m.commandInput.Update(msg)
	}
	return m, cmd
}

func (m *model) View() string {
	if m.quitting {
		return ""
	}

	var s strings.Builder

	// Render tip box banner
	if !m.hintsHidden {
		hintText := m.hints[m.hintIndex]
		examplesText := "💡 Post-Finish Command Examples:\n" +
			"  • claude -c \"continue\"\n" +
			"  • codex continue \"continue\"\n" +
			"  • opencode -c \"continue\"\n" +
			"  • agy --continue \"continue\"\n\n" +
			"⏰ Delay Target Examples:\n" +
			"  • 10s, 5m, 2h 15m\n" +
			"  • 1500, 3:00p\n\n" +
			"💡 Tip:\n" +
			"  " + hintText + "\n\n" +
			hintStyle.Render("(esc to dismiss)")

		s.WriteString(bannerStyle.Render(examplesText) + "\n\n")
	} else {
		s.WriteString(hintStyle.Render("[esc to show tips & examples]") + "\n\n")
	}

	// Duration Input
	s.WriteString(titleStyle.Render("How long should we delay?") + "\n")
	s.WriteString(m.durationInput.View() + "\n")

	// Real-time validation
	val := strings.TrimSpace(m.durationInput.Value())
	if val != "" {
		if dur, err := timer.ParseDurationOrTarget(val, time.Now()); err != nil {
			s.WriteString(errorStyle.Render("❌ "+err.Error()) + "\n")
		} else {
			targetTime := time.Now().Add(dur)
			targetStr := timer.FormatEndTime(targetTime, time.Now())
			s.WriteString(successStyle.Render("✔ Will finish at "+targetStr) + "\n")
		}
	} else if m.err != nil {
		s.WriteString(errorStyle.Render("❌ "+m.err.Error()) + "\n")
	}
	s.WriteString("\n")

	// Command Input
	s.WriteString(titleStyle.Render("What command should run when finished? (Optional)") + "\n")
	s.WriteString(m.commandInput.View() + "\n\n")

	// Submit button
	var btn string
	if m.focusIndex == 2 {
		btn = submitActiveStyle.Render("SUBMIT")
	} else {
		btn = submitInactiveStyle.Render("SUBMIT")
	}
	s.WriteString(btn + "\n")

	return docStyle.Render(s.String())
}

// runInteractive runs the interactive Bubble Tea UI and returns the entered duration and command.
func runInteractive(prepopulatedCmd string) (string, []string, error) {
	mModel := initialModel(prepopulatedCmd)
	p := tea.NewProgram(&mModel)
	m, err := p.Run()
	if err != nil {
		return "", nil, err
	}

	finalModel := m.(*model)
	if finalModel.aborted {
		return "", nil, timer.ErrInterrupted
	}

	duration := strings.TrimSpace(finalModel.durationInput.Value())
	var commandPart []string
	cmdStr := strings.TrimSpace(finalModel.commandInput.Value())
	if cmdStr != "" {
		var shell string
		var shellArgs []string
		if runtime.GOOS == "windows" {
			shell = os.Getenv("COMSPEC")
			if shell == "" {
				shell = "cmd.exe"
			}
			shellArgs = []string{"/c", cmdStr}
		} else {
			shell = os.Getenv("SHELL")
			if shell == "" {
				shell = "sh"
			}
			shellArgs = []string{"-c", cmdStr}
		}
		commandPart = append([]string{shell}, shellArgs...)
	}

	return duration, commandPart, nil
}
