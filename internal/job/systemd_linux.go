//go:build linux

package job

import (
	"context"
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
func (b *SystemdBackend) Install(ctx context.Context, metadata Metadata) error {
	units, err := RenderUnits(metadata, b.BinaryPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(b.UnitDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(b.UnitDir, ServiceUnitName(metadata.Label)), []byte(units.Service), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(b.UnitDir, TimerUnitName(metadata.Label)), []byte(units.Timer), 0o644); err != nil {
		return err
	}
	return b.DaemonReload(ctx)
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

// Remove deletes service and timer unit files and reloads the user systemd manager.
func (b *SystemdBackend) Remove(ctx context.Context, label string) error {
	if err := ValidateLabel(label); err != nil {
		return err
	}
	_ = b.DisableNow(ctx, label)
	if err := os.Remove(filepath.Join(b.UnitDir, TimerUnitName(label))); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(filepath.Join(b.UnitDir, ServiceUnitName(label))); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(filepath.Join(b.UnitDir, "timers.target.wants", TimerUnitName(label))); err != nil && !os.IsNotExist(err) {
		return err
	}
	return b.DaemonReload(ctx)
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
	return string(output), err
}
