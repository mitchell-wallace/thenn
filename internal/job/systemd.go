package job

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ErrUnsupported is returned by the systemd backend on unsupported platforms.
var ErrUnsupported = errors.New("thenn job requires Linux with user-level systemd (systemctl --user)")

// ErrSystemctlNotFound means the systemctl client is not installed or is not in PATH.
var ErrSystemctlNotFound = errors.New("thenn job requires user-level systemd (systemctl --user), but systemctl was not found in PATH; install systemd to use job commands")

// ErrUserSystemdUnavailable means systemctl exists but cannot reach the user manager.
var ErrUserSystemdUnavailable = errors.New("thenn job requires user-level systemd (systemctl --user), but the user service manager is not reachable; run this command from a systemd user session")

// ErrJournalctlNotFound means journalctl is unavailable for reading job logs.
var ErrJournalctlNotFound = errors.New("journalctl was not found in PATH; install journalctl (normally provided by systemd) to view job logs")

// CommandRunner abstracts process execution for systemctl calls.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner runs external commands with os/exec.
type ExecRunner struct{}

// Run executes name with args and returns combined stdout/stderr.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s %s failed: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

// SystemdBackend manages user-level systemd units for thenn jobs.
type SystemdBackend struct {
	UnitDir    string
	BinaryPath string
	Runner     CommandRunner
}

// UnitFiles contains rendered service and timer unit contents.
type UnitFiles struct {
	Service string
	Timer   string
}

// NewSystemdBackend creates the default systemd backend for the current platform.
func NewSystemdBackend() (*SystemdBackend, error) {
	return newSystemdBackend()
}

// RenderUnits renders the service and timer units for metadata.
func RenderUnits(metadata Metadata, binaryPath string) (UnitFiles, error) { //nolint:gocritic // Rendering from a value avoids backend-side mutation.
	if err := metadata.Validate(); err != nil {
		return UnitFiles{}, err
	}
	if strings.TrimSpace(binaryPath) == "" {
		return UnitFiles{}, fmt.Errorf("binary path is required")
	}
	if strings.ContainsAny(binaryPath, "\x00\r\n") {
		return UnitFiles{}, fmt.Errorf("binary path contains invalid characters")
	}

	serviceName := ServiceUnitName(metadata.Label)
	timerTriggers, err := renderTimerTriggers(metadata.ParsedSchedule)
	if err != nil {
		return UnitFiles{}, err
	}
	service := fmt.Sprintf(`[Unit]
Description=thenn job %s

[Service]
Type=oneshot
WorkingDirectory=%s
ExecStart=%s job exec %s
`, metadata.Label, quoteSystemdArg(metadata.CWD, false), quoteSystemdArg(binaryPath, true), metadata.Label)

	timer := fmt.Sprintf(`[Unit]
Description=Run thenn job %s

[Timer]
%s
Unit=%s

[Install]
WantedBy=timers.target
`, metadata.Label, timerTriggers, serviceName)

	return UnitFiles{Service: service, Timer: timer}, nil
}

func renderTimerTriggers(schedule Schedule) (string, error) {
	if err := validateSchedule(schedule); err != nil {
		return "", err
	}
	if schedule.OnCalendar != "" {
		return "OnCalendar=" + schedule.OnCalendar + "\nPersistent=true", nil
	}
	return "OnActiveSec=" + schedule.OnUnitActiveSec + "\nOnUnitActiveSec=" + schedule.OnUnitActiveSec, nil
}

// ServiceUnitName returns the systemd service unit name for label.
func ServiceUnitName(label string) string {
	return "thenn-job-" + label + ".service"
}

// TimerUnitName returns the systemd timer unit name for label.
func TimerUnitName(label string) string {
	return "thenn-job-" + label + ".timer"
}

var safeSystemdArgRe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

func quoteSystemdArg(arg string, escapeDollar bool) string {
	arg = strings.ReplaceAll(arg, "%", "%%")
	if escapeDollar {
		arg = strings.ReplaceAll(arg, "$", "$$")
	}
	if safeSystemdArgRe.MatchString(arg) {
		return arg
	}
	arg = strings.ReplaceAll(arg, `\`, `\\`)
	arg = strings.ReplaceAll(arg, `"`, `\"`)
	return `"` + arg + `"`
}

func (b *SystemdBackend) runner() CommandRunner {
	if b.Runner != nil {
		return b.Runner
	}
	return ExecRunner{}
}

func (b *SystemdBackend) systemctl(ctx context.Context, args ...string) ([]byte, error) {
	fullArgs := append([]string{"--user"}, args...)
	return b.runner().Run(ctx, "systemctl", fullArgs...)
}

// CheckAvailable verifies that systemctl can reach the user service manager.
func (b *SystemdBackend) CheckAvailable(ctx context.Context) error {
	_, err := b.systemctl(ctx, "show-environment")
	if err == nil {
		return nil
	}
	if commandNotFound(err) {
		return ErrSystemctlNotFound
	}
	return ErrUserSystemdUnavailable
}

func commandNotFound(err error) bool {
	return errors.Is(err, exec.ErrNotFound)
}
