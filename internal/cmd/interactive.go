package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/mitchell-wallace/thenn/internal/timer"
)

var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#d7005f", Dark: "205"}).
			Bold(true)

	focusedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#d7005f", Dark: "205"})

	blurredStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "242", Dark: "240"})

	docStyle = lipgloss.NewStyle().Margin(1, 2)

	bannerStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "26", Dark: "62"}).
			Padding(0, 1).
			Width(64).
			Height(5).
			Foreground(lipgloss.AdaptiveColor{Light: "235", Dark: "252"})

	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "244"}).
			Italic(true)

	placeholderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "245", Dark: "243"})

	hintCommandStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "25", Dark: "75"}).
				Bold(true)

	submitActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "232", Dark: "229"}).
				Background(lipgloss.AdaptiveColor{Light: "26", Dark: "62"}).
				Bold(true).
				Padding(0, 3).
				MarginTop(1)

	submitInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "244"}).
				Background(lipgloss.AdaptiveColor{Light: "254", Dark: "236"}).
				Padding(0, 3).
				MarginTop(1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "42"})
)

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(15*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type debounceTickMsg time.Time

func debounceTick() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return debounceTickMsg(t)
	})
}

type UserConfig struct {
	AlwaysHideHints        bool     `json:"always_hide_hints"`
	DismissedHints         []string `json:"dismissed_hints"`
	DisableCommandChecking bool     `json:"disable_command_checking"`
}

func loadConfig() UserConfig {
	var cfg UserConfig
	dir, err := os.UserConfigDir()
	if err != nil {
		return cfg
	}
	path := filepath.Join(dir, "thenn", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func saveConfig(cfg UserConfig) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	thennDir := filepath.Join(dir, "thenn")
	_ = os.MkdirAll(thennDir, 0o755)
	path := filepath.Join(thennDir, "config.json")
	data, _ := json.Marshal(cfg)
	_ = os.WriteFile(path, data, 0o644)
}

func commandCheckingEnabled() bool {
	return !loadConfig().DisableCommandChecking
}

type model struct {
	durationInput textinput.Model
	commandInput  textarea.Model
	focusIndex    int // 0: duration, 1: command, 2: submit button
	quitting      bool
	aborted       bool
	err           error

	// Hint cycling & configuration
	hints           []string
	hintIndex       int
	hintsHidden     bool
	alwaysHideHints bool
	commandChecking bool
	lastTickTime    time.Time

	// Debounced validation
	durationLastKeyPressTime time.Time
	durationValidated        bool
	durationValidationErr    error
	durationValidationTarget string
	commandLastKeyPressTime  time.Time
	commandValidated         bool
	commandWarnings          []commandWarning
	commandValidationValue   string
}

func initialModel(prepopulatedDuration, prepopulatedCmd string) model {
	d := textinput.New()
	d.Placeholder = "e.g. 10s, 5m, 1500, 3:00p"
	d.PlaceholderStyle = placeholderStyle
	d.Width = 50
	d.Focus()
	d.Prompt = focusedStyle.Render("> ")
	d.TextStyle = focusedStyle
	d.SetValue(prepopulatedDuration)

	c := textarea.New()
	c.Placeholder = "e.g. echo 'done'"
	c.FocusedStyle.Placeholder = placeholderStyle
	c.BlurredStyle.Placeholder = placeholderStyle
	c.SetWidth(50)
	initialHeight := countWrappedLines(prepopulatedCmd, 48)
	if initialHeight < 1 {
		initialHeight = 1
	}
	c.SetHeight(initialHeight)
	c.Prompt = "> "
	c.FocusedStyle.Prompt = focusedStyle
	c.BlurredStyle.Prompt = blurredStyle
	c.FocusedStyle.Text = focusedStyle
	c.BlurredStyle.Text = blurredStyle
	c.SetValue(prepopulatedCmd)
	c.ShowLineNumbers = false

	// Build the initial list of hints
	allHints := []string{
		"Start a new Claude session: `claude \"Fix broken tests\"`; resume one: `claude -c`",
		"Start a new Antigravity session: `agy -i \"Fix broken tests\"`; resume one: `agy --continue`",
		"Start a new Codex session: `codex \"Fix broken tests\"`; resume one: `codex resume --last`",
		"Start a new opencode session: `opencode`; resume one: `opencode -c`",
		"Run an agent non-interactively after a limit resets: `codex exec \"Fix broken tests\"` or `opencode run \"Fix broken tests\"`",
		"Resume non-interactively with a prompt: `codex resume --last \"continue\"` or `opencode run -c \"continue\"`",
		"Press `Spacebar` while the timer is running to pause the countdown",
	}

	cfg := loadConfig()

	// Filter dismissed hints
	var activeHints []string
	dismissedMap := make(map[string]bool)
	for _, dh := range cfg.DismissedHints {
		dismissedMap[dh] = true
	}
	for _, h := range allHints {
		if !dismissedMap[h] && !dismissedMap[stripHintMarkup(h)] {
			activeHints = append(activeHints, h)
		}
	}

	m := model{
		durationInput:   d,
		commandInput:    c,
		focusIndex:      0,
		hints:           activeHints,
		hintIndex:       0,
		hintsHidden:     cfg.AlwaysHideHints,
		alwaysHideHints: cfg.AlwaysHideHints,
		commandChecking: !cfg.DisableCommandChecking,
		lastTickTime:    time.Now(),
	}
	if strings.TrimSpace(prepopulatedDuration) != "" {
		m.durationLastKeyPressTime = time.Now()
	}
	if strings.TrimSpace(prepopulatedCmd) != "" {
		m.commandLastKeyPressTime = time.Now()
	}
	return m
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tickCmd(), debounceTick())
}

func (m *model) updateFocus() tea.Cmd {
	m.durationInput.Blur()
	m.commandInput.Blur()
	m.durationInput.Prompt = blurredStyle.Render("> ")

	var cmd tea.Cmd
	switch m.focusIndex {
	case 0:
		m.durationInput.Focus()
		m.durationInput.Prompt = focusedStyle.Render("> ")
	case 1:
		cmd = m.commandInput.Focus()
	}
	return cmd
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
		m.durationValidationErr = err
		m.durationValidated = true
		return nil
	}
	m.err = nil
	m.quitting = true
	return tea.Quit
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		width := msg.Width - 4
		if width < 20 {
			width = 20
		}
		m.commandInput.SetWidth(width)
		h := countWrappedLines(m.commandInput.Value(), width-2)
		if h < 1 {
			h = 1
		}
		m.commandInput.SetHeight(h)
		m.durationInput, _ = m.durationInput.Update(msg)
		m.commandInput, _ = m.commandInput.Update(msg)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.aborted = true
			m.quitting = true
			return m, tea.Quit

		case "esc":
			if !m.alwaysHideHints && len(m.hints) > 0 {
				m.hintsHidden = !m.hintsHidden
			}
			return m, nil

		case "ctrl+d":
			if m.hintsHidden && !m.alwaysHideHints {
				m.alwaysHideHints = true
				cfg := loadConfig()
				cfg.AlwaysHideHints = true
				saveConfig(cfg)
			}
			return m, nil

		case "ctrl+t":
			if !m.hintsHidden && !m.alwaysHideHints && len(m.hints) > 0 {
				// Don't show this hint again
				ignoredHint := m.hints[m.hintIndex]
				cfg := loadConfig()
				cfg.DismissedHints = append(cfg.DismissedHints, ignoredHint)
				saveConfig(cfg)

				// Remove from current active list
				m.hints = append(m.hints[:m.hintIndex], m.hints[m.hintIndex+1:]...)
				if len(m.hints) == 0 {
					m.alwaysHideHints = true
					m.hintsHidden = true
				} else {
					m.hintIndex %= len(m.hints)
				}
				m.lastTickTime = time.Now()
			}
			return m, nil

		case "ctrl+n":
			if !m.hintsHidden && len(m.hints) > 0 {
				m.hintIndex = (m.hintIndex + 1) % len(m.hints)
				m.lastTickTime = time.Now()
			}

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

		// Keystrokes reset the relevant validation timer.
		switch m.focusIndex {
		case 0:
			m.durationLastKeyPressTime = time.Now()
			m.durationValidated = false
			m.durationValidationErr = nil
			m.durationValidationTarget = ""
			m.err = nil
		case 1:
			m.commandLastKeyPressTime = time.Now()
			m.commandValidated = false
			m.commandWarnings = nil
			m.commandValidationValue = ""
		}

	case tickMsg:
		if !m.quitting && len(m.hints) > 0 {
			if time.Since(m.lastTickTime) >= 15*time.Second {
				m.hintIndex = (m.hintIndex + 1) % len(m.hints)
				m.lastTickTime = time.Now()
			}
			return m, tickCmd()
		}

	case debounceTickMsg:
		if !m.quitting {
			if !m.durationValidated && !m.durationLastKeyPressTime.IsZero() && time.Since(m.durationLastKeyPressTime) >= 300*time.Millisecond {
				val := strings.TrimSpace(m.durationInput.Value())
				if val != "" {
					dur, err := timer.ParseDurationOrTarget(val, time.Now())
					m.durationValidationErr = err
					if err == nil {
						targetTime := time.Now().Add(dur)
						m.durationValidationTarget = timer.FormatEndTime(targetTime, time.Now())
					}
				}
				m.durationValidated = true
			}
			if !m.commandValidated && !m.commandLastKeyPressTime.IsZero() && time.Since(m.commandLastKeyPressTime) >= 300*time.Millisecond {
				val := strings.TrimSpace(m.commandInput.Value())
				m.commandValidationValue = val
				m.commandWarnings = nil
				if val != "" && m.commandChecking {
					m.commandWarnings = checkCommand(resolveShell(val))
				}
				m.commandValidated = true
			}
			return m, debounceTick()
		}
	}

	var cmd tea.Cmd
	switch m.focusIndex {
	case 0:
		m.durationInput, cmd = m.durationInput.Update(msg)
	case 1:
		m.commandInput, cmd = m.commandInput.Update(msg)
		h := countWrappedLines(m.commandInput.Value(), m.commandInput.Width()-2)
		if h < 1 {
			h = 1
		}
		m.commandInput.SetHeight(h)
	}
	return m, cmd
}

func (m *model) View() string {
	if m.quitting {
		return ""
	}

	var s strings.Builder

	// Render hints banner
	if !m.alwaysHideHints && len(m.hints) > 0 {
		if !m.hintsHidden {
			hintText := renderHintText(m.hints[m.hintIndex])
			examplesText := "💡 Tip:\n" +
				"  " + hintText + "\n\n" +
				hintStyle.Render("CTRL+N: cycle tips | ESC: dismiss | CTRL+T: ignore tip")

			s.WriteString(bannerStyle.Render(examplesText) + "\n\n")
		} else {
			s.WriteString(hintStyle.Render("ESC: show tips | CTRL+D: hide hints forever") + "\n\n")
		}
	}

	// Duration Input
	s.WriteString(titleStyle.Render("How long should we delay?") + "\n")
	s.WriteString(m.durationInput.View() + "\n")

	// Real-time validation (debounced)
	val := strings.TrimSpace(m.durationInput.Value())
	if val != "" {
		if m.durationValidated {
			if m.durationValidationErr != nil {
				s.WriteString(errorStyle.Render("❌ "+m.durationValidationErr.Error()) + "\n")
			} else {
				s.WriteString(successStyle.Render("✔ Will finish at "+m.durationValidationTarget) + "\n")
			}
		} else {
			// typing... wait for debounce
			s.WriteString(hintStyle.Render("   typing...") + "\n")
		}
	} else if m.err != nil {
		s.WriteString(errorStyle.Render("❌ "+m.err.Error()) + "\n")
	}
	s.WriteString("\n")

	// Command Input
	s.WriteString(titleStyle.Render("What command should run when finished? (Optional)") + "\n")
	s.WriteString(m.commandInput.View() + "\n")
	cmdVal := strings.TrimSpace(m.commandInput.Value())
	if cmdVal != "" {
		if m.commandValidated && m.commandValidationValue == cmdVal {
			switch {
			case !m.commandChecking:
				s.WriteString(hintStyle.Render("   command checking disabled") + "\n")
			case len(m.commandWarnings) == 0:
				s.WriteString(successStyle.Render("✔ No issues detected") + "\n")
			default:
				for _, warning := range m.commandWarnings {
					s.WriteString(errorStyle.Render("⚠ "+warning.Message) + "\n")
				}
			}
		} else {
			s.WriteString(hintStyle.Render("   checking command...") + "\n")
		}
	}
	s.WriteString("\n")

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

func renderHintText(text string) string {
	var b strings.Builder
	inCommand := false
	var command strings.Builder
	for _, r := range text {
		if r == '`' {
			if inCommand {
				b.WriteString(hintCommandStyle.Render(command.String()))
				command.Reset()
			} else {
				b.WriteString(command.String())
				command.Reset()
			}
			inCommand = !inCommand
			continue
		}
		if inCommand {
			command.WriteRune(r)
		} else {
			b.WriteRune(r)
		}
	}
	if command.Len() > 0 {
		b.WriteString(command.String())
	}
	return b.String()
}

func stripHintMarkup(text string) string {
	return strings.ReplaceAll(text, "`", "")
}

// runInteractive runs the interactive Bubble Tea UI and returns the entered duration and command.
func runInteractive(prepopulatedCmd string) (string, []string, error) {
	return runInteractiveWithValues("", prepopulatedCmd)
}

func runInteractiveWithValues(prepopulatedDuration, prepopulatedCmd string) (string, []string, error) {
	mModel := initialModel(prepopulatedDuration, prepopulatedCmd)
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
		commandPart = resolveShell(cmdStr)
	}

	return duration, commandPart, nil
}

// countWrappedLines calculates the number of visual lines the text will occupy when wrapped to the given width.
func countWrappedLines(text string, width int) int {
	if text == "" {
		return 1
	}

	lines := strings.Split(text, "\n")
	totalLines := 0

	for _, rawLine := range lines {
		if rawLine == "" {
			totalLines++
			continue
		}

		lineCount := 1
		currentLineLen := 0
		wordLen := 0
		spaceCount := 0

		for _, r := range rawLine {
			rw := runewidth.RuneWidth(r)
			if unicode.IsSpace(r) {
				spaceCount++
			} else {
				if spaceCount > 0 {
					if currentLineLen+wordLen+spaceCount > width {
						lineCount++
						currentLineLen = wordLen + spaceCount
					} else {
						currentLineLen += wordLen + spaceCount
					}
					wordLen = rw
					spaceCount = 0
				} else {
					wordLen += rw
				}
			}

			if wordLen > width {
				lineCount++
				wordLen = rw
			}
		}

		if wordLen > 0 || spaceCount > 0 {
			if currentLineLen+wordLen+spaceCount > width {
				lineCount++
			}
		}

		totalLines += lineCount
	}

	return totalLines
}
