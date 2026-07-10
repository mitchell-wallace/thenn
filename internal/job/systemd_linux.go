//go:build linux

package job

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func newSystemdBackend() (*SystemdBackend, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	binaryPath, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &SystemdBackend{
		UnitDir:    filepath.Join(configDir, "systemd", "user"),
		BinaryPath: binaryPath,
		Runner:     ExecRunner{},
	}, nil
}

// Install writes service and timer unit files and reloads the user systemd manager.
func (b *SystemdBackend) Install(ctx context.Context, metadata Metadata) error { //nolint:gocritic // Backend methods keep value-style metadata ownership consistent.
	units, err := RenderUnits(metadata, b.BinaryPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(b.UnitDir, 0o755); err != nil {
		return err
	}
	servicePath := filepath.Join(b.UnitDir, ServiceUnitName(metadata.Label))
	timerPath := filepath.Join(b.UnitDir, TimerUnitName(metadata.Label))
	if err := writeUnitFile(servicePath, units.Service); err != nil {
		return err
	}
	if err := writeUnitFile(timerPath, units.Timer); err != nil {
		_ = os.Remove(servicePath)
		return err
	}
	if err := b.DaemonReload(ctx); err != nil {
		_ = os.Remove(timerPath)
		_ = os.Remove(servicePath)
		return err
	}
	return nil
}

func writeUnitFile(path, contents string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("systemd unit %s already exists; remove it before reusing this job label", path)
	}
	if err != nil {
		return err
	}
	if _, err := file.WriteString(contents); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

// DaemonReload reloads the user systemd manager.
func (b *SystemdBackend) DaemonReload(ctx context.Context) error {
	_, err := b.systemctl(ctx, "daemon-reload")
	return err
}

// Enable enables the job timer.
func (b *SystemdBackend) Enable(ctx context.Context, label string) error {
	if err := ValidateLabel(label); err != nil {
		return err
	}
	_, err := b.systemctl(ctx, "enable", TimerUnitName(label))
	return err
}

// EnableNow enables and starts the job timer.
func (b *SystemdBackend) EnableNow(ctx context.Context, label string) error {
	if err := ValidateLabel(label); err != nil {
		return err
	}
	_, err := b.systemctl(ctx, "enable", "--now", TimerUnitName(label))
	return err
}

// Disable disables the job timer.
func (b *SystemdBackend) Disable(ctx context.Context, label string) error {
	if err := ValidateLabel(label); err != nil {
		return err
	}
	_, err := b.systemctl(ctx, "disable", TimerUnitName(label))
	return err
}

// DisableNow stops and disables the job timer.
func (b *SystemdBackend) DisableNow(ctx context.Context, label string) error {
	if err := ValidateLabel(label); err != nil {
		return err
	}
	_, err := b.systemctl(ctx, "disable", "--now", TimerUnitName(label))
	return err
}

// Start starts the job timer.
func (b *SystemdBackend) Start(ctx context.Context, label string) error {
	if err := ValidateLabel(label); err != nil {
		return err
	}
	_, err := b.systemctl(ctx, "start", TimerUnitName(label))
	return err
}

// StartService runs the job command immediately through its systemd service.
func (b *SystemdBackend) StartService(ctx context.Context, label string) error {
	if err := ValidateLabel(label); err != nil {
		return err
	}
	_, err := b.systemctl(ctx, "start", ServiceUnitName(label))
	return err
}

// Stop stops the job timer.
func (b *SystemdBackend) Stop(ctx context.Context, label string) error {
	if err := ValidateLabel(label); err != nil {
		return err
	}
	_, err := b.systemctl(ctx, "stop", TimerUnitName(label))
	return err
}

// StopService stops a currently running job command.
func (b *SystemdBackend) StopService(ctx context.Context, label string) error {
	if err := ValidateLabel(label); err != nil {
		return err
	}
	_, err := b.systemctl(ctx, "stop", ServiceUnitName(label))
	return err
}

// Remove stops the timer and service, deletes their unit files, and reloads systemd.
func (b *SystemdBackend) Remove(ctx context.Context, label string) error {
	if err := ValidateLabel(label); err != nil {
		return err
	}
	disableErr := b.DisableNow(ctx, label)
	stopErr := b.StopService(ctx, label)
	cleanErr := b.cleanTimerState(ctx, label)
	if err := errors.Join(disableErr, stopErr, cleanErr); err != nil {
		return fmt.Errorf("stop job before removal: %w", err)
	}
	// Removal must not fail on cleanup hygiene: cleanTimerState above only
	// reports filesystem errors; systemctl-clean refusals (no state, unit
	// already unloaded) are tolerated inside it.
	if err := b.removeUnitFiles(label); err != nil {
		return err
	}
	return b.DaemonReload(ctx)
}

// RollbackInstall removes units owned by a failed create operation.
func (b *SystemdBackend) RollbackInstall(ctx context.Context, label string) error {
	if err := ValidateLabel(label); err != nil {
		return err
	}
	_ = b.DisableNow(ctx, label)
	_ = b.StopService(ctx, label)
	_ = b.cleanTimerState(ctx, label)
	removeErr := b.removeUnitFiles(label)
	reloadErr := b.DaemonReload(ctx)
	return errors.Join(removeErr, reloadErr)
}

// cleanTimerState removes Persistent= timestamp state before a timer unit is
// uninstalled, so reusing a label cannot inherit an old calendar deadline.
// It is idempotent: systemctl-clean refusals (no matching state, unit already
// unloaded) are tolerated because the stamp file is also removed directly;
// only a real filesystem error is reported.
func (b *SystemdBackend) cleanTimerState(ctx context.Context, label string) error {
	// Best-effort: fails on units without state or already-unloaded units,
	// and those refusals must not block removal.
	_, _ = b.systemctl(ctx, "clean", "--what=state", TimerUnitName(label))
	stamp := filepath.Join(b.timerStateDir(), "stamp-"+TimerUnitName(label))
	if err := os.Remove(stamp); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove persistent timer stamp: %w", err)
	}
	return nil
}

// timerStateDir returns where the user manager stores Persistent= stamp files.
func (b *SystemdBackend) timerStateDir() string {
	if b.StateDir != "" {
		return b.StateDir
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "systemd", "timers")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "systemd", "timers")
}

func (b *SystemdBackend) removeUnitFiles(label string) error {
	paths := []string{
		filepath.Join(b.UnitDir, TimerUnitName(label)),
		filepath.Join(b.UnitDir, ServiceUnitName(label)),
		filepath.Join(b.UnitDir, "timers.target.wants", TimerUnitName(label)),
	}
	var removeErrs []error
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			removeErrs = append(removeErrs, err)
		}
	}
	return errors.Join(removeErrs...)
}

// Status returns systemctl status output for the job timer.
func (b *SystemdBackend) Status(ctx context.Context, label string) (string, error) {
	if err := ValidateLabel(label); err != nil {
		return "", err
	}
	output, err := b.systemctl(ctx, "status", TimerUnitName(label), ServiceUnitName(label))
	return string(output), err
}

// Journal returns recent logs for the job service.
func (b *SystemdBackend) Journal(ctx context.Context, label string, lines int) (string, error) {
	if err := ValidateLabel(label); err != nil {
		return "", err
	}
	if lines <= 0 {
		lines = 80
	}
	output, err := b.runner().Run(ctx, "journalctl", "--user-unit", ServiceUnitName(label), "--no-pager", "-n", fmt.Sprintf("%d", lines))
	if commandNotFound(err) {
		return "", ErrJournalctlNotFound
	}
	return string(output), err
}
