package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mitchell-wallace/thenn/internal/job"
	"github.com/spf13/cobra"
)

const jobTUIActionTimeout = 10 * time.Second

type jobTUITab int

const (
	jobTabJobs jobTUITab = iota
	jobTabHelp
)

var (
	jobFrameStyle = lipgloss.NewStyle().Padding(0, 1)
	jobPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "240", Dark: "238"}).
			Padding(0, 1)
	jobPanelFocusedStyle = jobPanelStyle.
				BorderForeground(lipgloss.AdaptiveColor{Light: "26", Dark: "62"})
	jobSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "232", Dark: "229"}).
				Background(lipgloss.AdaptiveColor{Light: "26", Dark: "62"}).
				Bold(true)
	jobMutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "244"})
	jobErrorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	jobOKStyle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "42"})
	jobHeaderStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "25", Dark: "75"}).Bold(true)
	jobTabStyle    = lipgloss.NewStyle().Padding(0, 1)
	jobActiveTab   = jobTabStyle.Foreground(lipgloss.AdaptiveColor{Light: "232", Dark: "229"}).Background(lipgloss.AdaptiveColor{Light: "26", Dark: "62"}).Bold(true)
)

type jobTUIModel struct {
	store         *job.Store
	backend       job.Backend
	jobs          []job.Metadata
	selected      int
	tab           jobTUITab
	width         int
	height        int
	status        string
	err           string
	logs          string
	confirmDelete string
}

func runJobTUI(cmd *cobra.Command) error {
	store, backend, err := newAvailableJobStoreAndBackend(cmd.Context())
	if err != nil {
		return err
	}
	m := jobTUIModel{store: store, backend: backend}
	m.reload()
	p := tea.NewProgram(&m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func (m *jobTUIModel) Init() tea.Cmd { return nil }

func (m *jobTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		m.err = ""
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			if m.tab == jobTabJobs {
				m.tab = jobTabHelp
			} else {
				m.tab = jobTabJobs
			}
			m.confirmDelete = ""
		case "?":
			m.tab = jobTabHelp
			m.confirmDelete = ""
		case "esc":
			m.tab = jobTabJobs
			m.confirmDelete = ""
		case "up", "k":
			if m.tab == jobTabJobs && m.selected > 0 {
				m.selected--
				m.logs = ""
			}
			m.confirmDelete = ""
		case "down", "j":
			if m.tab == jobTabJobs && m.selected < len(m.jobs)-1 {
				m.selected++
				m.logs = ""
			}
			m.confirmDelete = ""
		case "R":
			m.reload()
			m.status = "jobs refreshed"
			m.confirmDelete = ""
		case "p":
			m.withSelected("paused", func(ctx context.Context, selected job.Metadata) error {
				return m.backend.DisableNow(ctx, selected.Label)
			})
			m.confirmDelete = ""
		case "r":
			m.withSelected("resumed", func(ctx context.Context, selected job.Metadata) error {
				return m.backend.EnableNow(ctx, selected.Label)
			})
			m.confirmDelete = ""
		case "enter":
			m.withSelected("started", func(ctx context.Context, selected job.Metadata) error {
				return m.backend.StartService(ctx, selected.Label)
			})
			m.confirmDelete = ""
		case "l":
			m.loadLogs()
			m.confirmDelete = ""
		case "d":
			m.deleteSelected()
		}
	}
	return m, nil
}

func (m *jobTUIModel) View() string {
	if m.width == 0 {
		m.width = 100
	}
	if m.height == 0 {
		m.height = 30
	}

	bodyHeight := max(8, m.height-6)
	header := m.renderHeader()
	footer := jobMutedStyle.Render("up/down select  enter run  p pause  r resume  d delete  l logs  R refresh  ? help  q quit")
	var body string
	if m.tab == jobTabHelp {
		body = m.renderHelp(bodyHeight)
	} else {
		body = m.renderJobs(bodyHeight)
	}

	line := ""
	if m.err != "" {
		line = jobErrorStyle.Render(m.err)
	} else if m.status != "" {
		line = jobOKStyle.Render(m.status)
	}

	return jobFrameStyle.Width(m.width).Render(strings.Join([]string{header, body, line, footer}, "\n"))
}

func (m *jobTUIModel) renderHeader() string {
	jobsTab := jobTabStyle.Render("Jobs")
	helpTab := jobTabStyle.Render("Help")
	if m.tab == jobTabJobs {
		jobsTab = jobActiveTab.Render("Jobs")
	} else {
		helpTab = jobActiveTab.Render("Help")
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, jobHeaderStyle.Render("thenn job"), "  ", jobsTab, helpTab)
}

func (m *jobTUIModel) renderJobs(height int) string {
	leftWidth := max(28, min(42, m.width/3))
	rightWidth := max(40, m.width-leftWidth-8)
	left := jobPanelFocusedStyle.Width(leftWidth).Height(height).Render(m.renderJobList(leftWidth, height))
	right := jobPanelStyle.Width(rightWidth).Height(height).Render(m.renderJobDetails(rightWidth, height))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}

func (m *jobTUIModel) renderJobList(width, height int) string {
	if len(m.jobs) == 0 {
		return jobMutedStyle.Render("No thenn jobs yet.\n\nCreate one with:\nthenn job every 3h --label check -- echo ok")
	}
	var b strings.Builder
	b.WriteString(jobHeaderStyle.Render("Managed Jobs") + "\n\n")
	maxRows := max(1, height-4)
	start := 0
	if m.selected >= maxRows {
		start = m.selected - maxRows + 1
	}
	for i := start; i < len(m.jobs) && i < start+maxRows; i++ {
		metadata := m.jobs[i]
		line := fmt.Sprintf("%-18s %s", metadata.Label, metadata.OriginalPhrase)
		line = truncate(line, width-4)
		if i == m.selected {
			b.WriteString(jobSelectedStyle.Width(width - 4).Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func (m *jobTUIModel) renderJobDetails(width, height int) string {
	selected, ok := m.selectedJob()
	if !ok {
		return jobMutedStyle.Render("No job selected.")
	}
	var b strings.Builder
	b.WriteString(jobHeaderStyle.Render(selected.Label) + "\n")
	b.WriteString("Schedule: ")
	b.WriteString(selected.OriginalPhrase)
	b.WriteByte('\n')
	if selected.ParsedSchedule.OnCalendar != "" {
		b.WriteString("Systemd:  OnCalendar=")
		b.WriteString(selected.ParsedSchedule.OnCalendar)
		b.WriteByte('\n')
	}
	if selected.ParsedSchedule.OnUnitActiveSec != "" {
		b.WriteString("Systemd:  OnActiveSec=")
		b.WriteString(selected.ParsedSchedule.OnUnitActiveSec)
		b.WriteString(", OnUnitInactiveSec=")
		b.WriteString(selected.ParsedSchedule.OnUnitActiveSec)
		b.WriteByte('\n')
	}
	if selected.ParsedSchedule.Until != nil {
		b.WriteString("Until:    ")
		b.WriteString(selected.ParsedSchedule.Until.Format("2006-01-02 15:04"))
		b.WriteByte('\n')
	}
	b.WriteString("Command:  ")
	b.WriteString(formatJobCommand(selected.CommandArgv))
	b.WriteByte('\n')
	b.WriteString("CWD:      ")
	b.WriteString(selected.CWD)
	b.WriteByte('\n')
	b.WriteString("\n")
	if m.confirmDelete == selected.Label {
		b.WriteString(jobErrorStyle.Render("Press d again to remove this job."))
		b.WriteString("\n\n")
	}
	if m.logs != "" {
		b.WriteString(jobHeaderStyle.Render("Recent Logs") + "\n")
		b.WriteString(truncateLines(m.logs, width-4, max(3, height-12)))
	} else {
		b.WriteString(jobMutedStyle.Render("Press l to load recent logs."))
	}
	return b.String()
}

func (m *jobTUIModel) renderHelp(height int) string {
	content := `Schedule syntax

Keys

  up/down or j/k  select job
  enter           run selected job now
  p               pause timer
  r               resume timer
  d               remove job, press twice to confirm
  l               load recent logs
  R               refresh jobs
  tab, ?, esc     switch tabs/help
  q               quit

  every 15m
  every 3h until 2026-07-23
  daily at 9pm
  weekdays at 08:30
  weekly monday at 10am
  once at 2026-07-23 21:00

Dates are always year-first to avoid ambiguity:
  2026-07-23, 2026/07/23, 2026.07.23

Times can be written as:
  9pm, 9:30pm, 21:00, 09:30

CLI examples

  thenn job every 3h --label check-api -- curl https://example.com
  thenn job daily at 9pm --label review -- codex exec "review"
  thenn job syntax
`
	return jobPanelFocusedStyle.Width(max(60, m.width-6)).Height(height).Render(content)
}

func (m *jobTUIModel) reload() {
	jobs, err := m.store.List()
	if err != nil {
		m.err = err.Error()
		return
	}
	m.jobs = jobs
	if m.selected >= len(m.jobs) {
		m.selected = len(m.jobs) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

func (m *jobTUIModel) selectedJob() (job.Metadata, bool) {
	if len(m.jobs) == 0 || m.selected < 0 || m.selected >= len(m.jobs) {
		return job.Metadata{}, false
	}
	return m.jobs[m.selected], true
}

func (m *jobTUIModel) withSelected(action string, fn func(context.Context, job.Metadata) error) {
	selected, ok := m.selectedJob()
	if !ok {
		m.err = "no job selected"
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), jobTUIActionTimeout)
	defer cancel()
	if err := fn(ctx, selected); err != nil {
		m.err = err.Error()
		return
	}
	m.status = fmt.Sprintf("%s job %s", action, selected.Label)
}

func (m *jobTUIModel) loadLogs() {
	selected, ok := m.selectedJob()
	if !ok {
		m.err = "no job selected"
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), jobTUIActionTimeout)
	defer cancel()
	logs, err := m.backend.Journal(ctx, selected.Label, defaultJobLogLines)
	if err != nil {
		m.err = err.Error()
		return
	}
	m.logs = logs
	m.status = "loaded logs for " + selected.Label
}

func (m *jobTUIModel) deleteSelected() {
	selected, ok := m.selectedJob()
	if !ok {
		m.err = "no job selected"
		return
	}
	if m.confirmDelete != selected.Label {
		m.confirmDelete = selected.Label
		m.status = "press d again to remove " + selected.Label
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), jobTUIActionTimeout)
	defer cancel()
	if err := m.backend.Remove(ctx, selected.Label); err != nil {
		m.err = err.Error()
		return
	}
	if err := m.store.Delete(selected.Label); err != nil {
		m.err = err.Error()
		return
	}
	m.status = "removed job " + selected.Label
	m.confirmDelete = ""
	m.logs = ""
	m.reload()
}

func truncate(input string, width int) string {
	if width <= 0 || len(input) <= width {
		return input
	}
	if width <= 3 {
		return input[:width]
	}
	return input[:width-3] + "..."
}

func truncateLines(input string, width, maxLines int) string {
	lines := strings.Split(strings.TrimRight(input, "\n"), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	for i, line := range lines {
		lines[i] = truncate(line, width)
	}
	return strings.Join(lines, "\n")
}
