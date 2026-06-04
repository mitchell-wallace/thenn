package timer

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Styling definitions
var (
	remainingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	arrowStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	targetStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("105"))
	pausedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
)

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
	pauseChan := make(chan struct{})
	interruptChan := make(chan struct{})
	stopChan := make(chan struct{})
	defer func() {
		// Ensure channel is not closed twice
		select {
		case <-stopChan:
		default:
			close(stopChan)
		}
	}()

	// Start key listener (platform-specific)
	go r.listenInput(pauseChan, interruptChan, stopChan)

	var paused bool
	remaining := r.Duration
	endTime := time.Now().Add(remaining)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	r.printLine(remaining, endTime, paused)

	for remaining > 0 {
		select {
		case <-interruptChan:
			if !r.Quiet {
				fmt.Println()
			}
			return fmt.Errorf("interrupted")
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

	// Clean up display line
	if !r.Quiet {
		fmt.Printf("\r\x1b[K")
	}

	// Signal key listener goroutine to stop and sleep briefly to ensure fd release
	close(stopChan)
	time.Sleep(60 * time.Millisecond)

	// Execute delayed command
	if len(r.Command) > 0 {
		cmdName := r.Command[0]
		cmdArgs := r.Command[1:]
		cmd := exec.Command(cmdName, cmdArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

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
	pmStr := target.Format("pm")
	ampm := "a"
	if strings.Contains(strings.ToLower(pmStr), "pm") {
		ampm = "p"
	}
	dayStr := GetDayString(target, now)
	return fmt.Sprintf("%s%s %s", hourMin, ampm, dayStr)
}
