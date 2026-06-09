package timer

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Styling definitions
var (
	remainingStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	arrowStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	targetStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("105"))
	pausedStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
	commandLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	commandStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)
)

// ErrInterrupted is returned when the user interrupts execution (Ctrl+C).
var ErrInterrupted = errors.New("interrupted")

// Runner represents the countdown runner configuration and execution state.
type Runner struct {
	Duration time.Duration
	Command  []string
	Quiet    bool
}

// NewRunner creates a new countdown timer runner.
func NewRunner(d time.Duration, cmd []string, quiet bool) *Runner {
	return &Runner{
		Duration: d,
		Command:  cmd,
		Quiet:    quiet,
	}
}

// Run starts the countdown, handles keyboard pausing, and executes the delayed command.
func (r *Runner) Run() error {
	pauseChan := make(chan struct{}, 1)
	interruptChan := make(chan struct{}, 1)
	stopChan := make(chan struct{})
	doneChan := make(chan struct{})
	defer func() {
		// Ensure channel is not closed twice
		select {
		case <-stopChan:
		default:
			close(stopChan)
		}
	}()

	// Start key listener (platform-specific)
	go r.listenInput(pauseChan, interruptChan, stopChan, doneChan)

	var paused bool
	remaining := r.Duration
	endTime := time.Now().Add(remaining)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	if !r.Quiet && len(r.Command) > 0 {
		formattedCmd := FormatCommand(r.Command)
		if formattedCmd != "" {
			fmt.Printf("%s\n    %s %s\n\n",
				commandLabelStyle.Render("Scheduled command:"),
				arrowStyle.Render(">"),
				commandStyle.Render(formattedCmd),
			)
		}
	}

	r.printLine(remaining, endTime, paused)

	for remaining > 0 {
		select {
		case <-interruptChan:
			if !r.Quiet {
				fmt.Println()
			}
			close(stopChan)
			<-doneChan
			return ErrInterrupted
		case <-pauseChan:
			paused = !paused
			if !paused {
				endTime = time.Now().Add(remaining)
			}
			r.printLine(remaining, endTime, paused)
		case <-ticker.C:
			if !paused {
				remaining = time.Until(endTime)
				if remaining < 0 {
					remaining = 0
				}
			} else {
				endTime = time.Now().Add(remaining)
			}
			r.printLine(remaining, endTime, paused)
		}
	}

	// Signal key listener goroutine to stop and wait for it to release stdin
	close(stopChan)
	<-doneChan

	// Print final completed status
	if !r.Quiet {
		r.printLine(0, time.Now(), false)
		fmt.Println()
	}

	// Execute delayed command
	if len(r.Command) > 0 {
		cmdName := r.Command[0]
		cmdArgs := r.Command[1:]
		cmd := exec.Command(cmdName, cmdArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		// Ignore signals during command execution to let the child process handle them
		ignoreSignals()
		defer resetSignals()

		err := cmd.Run()
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) printLine(remaining time.Duration, endTime time.Time, paused bool) {
	if r.Quiet {
		return
	}

	remStr := FormatRemaining(remaining)
	targetStr := FormatEndTime(endTime, time.Now())

	var output string
	if paused {
		output = fmt.Sprintf("%s %s %s %s",
			pausedStyle.Render("[PAUSED]"),
			remainingStyle.Render(remStr),
			arrowStyle.Render("->"),
			targetStyle.Render(targetStr),
		)
	} else {
		output = fmt.Sprintf("%s %s %s",
			remainingStyle.Render(remStr),
			arrowStyle.Render("->"),
			targetStyle.Render(targetStr),
		)
	}

	fmt.Printf("\r\x1b[K%s", output)
}

// FormatRemaining formats a duration into a readable "d h m s" string.
func FormatRemaining(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	// Round to nearest second for cleaner countdown display
	d = (d + 500*time.Millisecond).Truncate(time.Second)

	days := int(d / (24 * time.Hour))
	d -= time.Duration(days) * 24 * time.Hour
	hours := int(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	minutes := int(d / time.Minute)
	d -= time.Duration(minutes) * time.Minute
	seconds := int(d / time.Second)

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	return strings.Join(parts, " ")
}

// GetDayString resolves the relative calendar date string for the target time.
func GetDayString(target, now time.Time) string {
	targetLocal := target.Local()
	nowLocal := now.Local()

	tYear, tMonth, tDay := targetLocal.Date()
	nYear, nMonth, nDay := nowLocal.Date()

	if tYear == nYear && tMonth == nMonth && tDay == nDay {
		return "today"
	}

	tomorrow := nowLocal.AddDate(0, 0, 1)
	tomYear, tomMonth, tomDay := tomorrow.Date()
	if tYear == tomYear && tMonth == tomMonth && tDay == tomDay {
		return "tomorrow"
	}

	return targetLocal.Format("2006.01.02")
}

// FormatEndTime formats the target end time to a 12-hour clock (with lower-case a/p) and relative day.
func FormatEndTime(target, now time.Time) string {
	hourMin := target.Format("3:04")
	ampm := "a"
	if target.Hour() >= 12 {
		ampm = "p"
	}
	dayStr := GetDayString(target, now)
	return fmt.Sprintf("%s%s %s", hourMin, ampm, dayStr)
}

// FormatCommand formats the command argument list for display.
func FormatCommand(cmd []string) string {
	if len(cmd) == 0 {
		return ""
	}
	if len(cmd) == 3 && (cmd[1] == "-c" || cmd[1] == "/c") {
		lower0 := strings.ToLower(cmd[0])
		if strings.HasSuffix(lower0, "sh") || strings.HasSuffix(lower0, "bash") || strings.HasSuffix(lower0, "zsh") || strings.HasSuffix(lower0, "cmd.exe") || strings.HasSuffix(lower0, "cmd") || strings.HasSuffix(lower0, "powershell.exe") || strings.HasSuffix(lower0, "powershell") {
			return cmd[2]
		}
	}
	var parts []string
	for _, arg := range cmd {
		if strings.Contains(arg, " ") {
			parts = append(parts, fmt.Sprintf("%q", arg))
		} else {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}
