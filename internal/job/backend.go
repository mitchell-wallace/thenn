package job

import "context"

// Backend manages the lifecycle and runtime state of scheduled jobs.
type Backend interface {
	Name() string
	CheckAvailable(ctx context.Context) error
	Install(ctx context.Context, metadata Metadata) error
	EnableNow(ctx context.Context, label string) error
	DisableNow(ctx context.Context, label string) error
	StartService(ctx context.Context, label string) error
	Remove(ctx context.Context, label string) error
	RollbackInstall(ctx context.Context, label string) error
	Status(ctx context.Context, label string) (string, error)
	Journal(ctx context.Context, label string, lines int) (string, error)
}

var _ Backend = (*SystemdBackend)(nil)
