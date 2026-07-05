//go:build !linux

package job

import "context"

func newSystemdBackend() (*SystemdBackend, error) {
	return nil, ErrUnsupported
}

// Install returns ErrUnsupported on non-Linux platforms.
func (b *SystemdBackend) Install(ctx context.Context, metadata Metadata) error {
	return ErrUnsupported
}

// DaemonReload returns ErrUnsupported on non-Linux platforms.
func (b *SystemdBackend) DaemonReload(ctx context.Context) error {
	return ErrUnsupported
}

// Enable returns ErrUnsupported on non-Linux platforms.
func (b *SystemdBackend) Enable(ctx context.Context, label string) error {
	return ErrUnsupported
}

// EnableNow returns ErrUnsupported on non-Linux platforms.
func (b *SystemdBackend) EnableNow(ctx context.Context, label string) error {
	return ErrUnsupported
}

// Disable returns ErrUnsupported on non-Linux platforms.
func (b *SystemdBackend) Disable(ctx context.Context, label string) error {
	return ErrUnsupported
}

// DisableNow returns ErrUnsupported on non-Linux platforms.
func (b *SystemdBackend) DisableNow(ctx context.Context, label string) error {
	return ErrUnsupported
}

// Start returns ErrUnsupported on non-Linux platforms.
func (b *SystemdBackend) Start(ctx context.Context, label string) error {
	return ErrUnsupported
}

// StartService returns ErrUnsupported on non-Linux platforms.
func (b *SystemdBackend) StartService(ctx context.Context, label string) error {
	return ErrUnsupported
}

// Stop returns ErrUnsupported on non-Linux platforms.
func (b *SystemdBackend) Stop(ctx context.Context, label string) error {
	return ErrUnsupported
}

// Remove returns ErrUnsupported on non-Linux platforms.
func (b *SystemdBackend) Remove(ctx context.Context, label string) error {
	return ErrUnsupported
}

// Status returns ErrUnsupported on non-Linux platforms.
func (b *SystemdBackend) Status(ctx context.Context, label string) (string, error) {
	return "", ErrUnsupported
}

// Journal returns ErrUnsupported on non-Linux platforms.
func (b *SystemdBackend) Journal(ctx context.Context, label string, lines int) (string, error) {
	return "", ErrUnsupported
}
