//go:build linux

package job

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls  []runCall
	output []byte
	err    error
}

type runCall struct {
	name string
	args []string
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, runCall{name: name, args: append([]string(nil), args...)})
	return f.output, f.err
}

func TestSystemdBackendWithFakeSystemctl(t *testing.T) {
	runner := &fakeRunner{output: []byte("loaded active")}
	backend := &SystemdBackend{
		UnitDir:    t.TempDir(),
		BinaryPath: "/usr/bin/thenn",
		Runner:     runner,
	}
	metadata := testMetadata(t)
	ctx := context.Background()

	if err := backend.Install(ctx, metadata); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(backend.UnitDir, ServiceUnitName(metadata.Label))); err != nil {
		t.Fatalf("service unit not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(backend.UnitDir, TimerUnitName(metadata.Label))); err != nil {
		t.Fatalf("timer unit not written: %v", err)
	}

	if err := backend.Enable(ctx, metadata.Label); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if err := backend.EnableNow(ctx, metadata.Label); err != nil {
		t.Fatalf("EnableNow() error = %v", err)
	}
	if err := backend.Start(ctx, metadata.Label); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := backend.StartService(ctx, metadata.Label); err != nil {
		t.Fatalf("StartService() error = %v", err)
	}
	status, err := backend.Status(ctx, metadata.Label)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status != "loaded active" {
		t.Fatalf("Status() = %q, want %q", status, "loaded active")
	}
	if err := backend.Stop(ctx, metadata.Label); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := backend.Disable(ctx, metadata.Label); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if err := backend.DisableNow(ctx, metadata.Label); err != nil {
		t.Fatalf("DisableNow() error = %v", err)
	}
	logs, err := backend.Journal(ctx, metadata.Label, 20)
	if err != nil {
		t.Fatalf("Journal() error = %v", err)
	}
	if logs != "loaded active" {
		t.Fatalf("Journal() = %q, want %q", logs, "loaded active")
	}
	if err := backend.Remove(ctx, metadata.Label); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	want := []runCall{
		{name: "systemctl", args: []string{"--user", "daemon-reload"}},
		{name: "systemctl", args: []string{"--user", "enable", "thenn-job-backup-daily.timer"}},
		{name: "systemctl", args: []string{"--user", "enable", "--now", "thenn-job-backup-daily.timer"}},
		{name: "systemctl", args: []string{"--user", "start", "thenn-job-backup-daily.timer"}},
		{name: "systemctl", args: []string{"--user", "start", "thenn-job-backup-daily.service"}},
		{name: "systemctl", args: []string{"--user", "status", "thenn-job-backup-daily.timer", "thenn-job-backup-daily.service"}},
		{name: "systemctl", args: []string{"--user", "stop", "thenn-job-backup-daily.timer"}},
		{name: "systemctl", args: []string{"--user", "disable", "thenn-job-backup-daily.timer"}},
		{name: "systemctl", args: []string{"--user", "disable", "--now", "thenn-job-backup-daily.timer"}},
		{name: "journalctl", args: []string{"--user-unit", "thenn-job-backup-daily.service", "--no-pager", "-n", "20"}},
		{name: "systemctl", args: []string{"--user", "disable", "--now", "thenn-job-backup-daily.timer"}},
		{name: "systemctl", args: []string{"--user", "stop", "thenn-job-backup-daily.service"}},
		{name: "systemctl", args: []string{"--user", "clean", "--what=state", "thenn-job-backup-daily.timer"}},
		{name: "systemctl", args: []string{"--user", "daemon-reload"}},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("systemctl calls = %#v, want %#v", runner.calls, want)
	}
}

func TestCheckAvailable(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr error
	}{
		{name: "empty output is available"},
		{
			name:    "systemctl missing",
			err:     &exec.Error{Name: "systemctl", Err: exec.ErrNotFound},
			wantErr: ErrSystemctlNotFound,
		},
		{
			name:    "user manager unreachable",
			err:     errors.New("Failed to connect to bus: No such file or directory"),
			wantErr: ErrUserSystemdUnavailable,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{err: tc.err}
			backend := &SystemdBackend{Runner: runner}
			err := backend.CheckAvailable(context.Background())
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("CheckAvailable() error = %v, want %v", err, tc.wantErr)
			}
			wantCalls := []runCall{{name: "systemctl", args: []string{"--user", "show-environment"}}}
			if !reflect.DeepEqual(runner.calls, wantCalls) {
				t.Fatalf("calls = %#v, want %#v", runner.calls, wantCalls)
			}
		})
	}
}

func TestInstallRejectsExistingUnitsWithoutOverwriting(t *testing.T) {
	unitDir := t.TempDir()
	timerPath := filepath.Join(unitDir, TimerUnitName("backup-daily"))
	if err := os.WriteFile(timerPath, []byte("existing timer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	backend := &SystemdBackend{UnitDir: unitDir, BinaryPath: "/usr/bin/thenn", Runner: &fakeRunner{}}

	err := backend.Install(context.Background(), testMetadata(t))
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Install() error = %v, want unit collision", err)
	}
	contents, readErr := os.ReadFile(timerPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(contents) != "existing timer\n" {
		t.Fatalf("existing timer was overwritten: %q", contents)
	}
	servicePath := filepath.Join(unitDir, ServiceUnitName("backup-daily"))
	if _, statErr := os.Stat(servicePath); !os.IsNotExist(statErr) {
		t.Fatalf("partially-created service was not cleaned up: %v", statErr)
	}
}

func TestJournalReportsMissingJournalctl(t *testing.T) {
	runner := &fakeRunner{err: &exec.Error{Name: "journalctl", Err: exec.ErrNotFound}}
	backend := &SystemdBackend{Runner: runner}
	_, err := backend.Journal(context.Background(), "backup-daily", 20)
	if !errors.Is(err, ErrJournalctlNotFound) {
		t.Fatalf("Journal() error = %v, want %v", err, ErrJournalctlNotFound)
	}
}

// failCleanRunner refuses `systemctl clean` (as real systemd does for units
// without state or already unloaded) but succeeds for everything else.
type failCleanRunner struct {
	fakeRunner
}

func (f *failCleanRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, runCall{name: name, args: append([]string(nil), args...)})
	for _, a := range args {
		if a == "clean" {
			return []byte("Failed to clean: Unit is not loaded."), errors.New("exit status 1")
		}
	}
	return f.output, f.err
}

func TestRemoveSucceedsWhenCleanRefusesAndStampRemoved(t *testing.T) {
	unitDir, stateDir := t.TempDir(), t.TempDir()
	backend := &SystemdBackend{UnitDir: unitDir, StateDir: stateDir, BinaryPath: "/usr/bin/thenn", Runner: &failCleanRunner{}}
	metadata := testMetadata(t)
	ctx := context.Background()
	if err := backend.Install(ctx, metadata); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	stamp := filepath.Join(stateDir, "stamp-"+TimerUnitName(metadata.Label))
	if err := os.WriteFile(stamp, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := backend.Remove(ctx, metadata.Label); err != nil {
		t.Fatalf("Remove() must tolerate systemctl clean refusal: %v", err)
	}
	if _, err := os.Stat(stamp); !os.IsNotExist(err) {
		t.Fatalf("persistent stamp should be removed, stat err = %v", err)
	}
	// Second removal cycle on the same label must also be clean (idempotence).
	if err := backend.Install(ctx, metadata); err != nil {
		t.Fatalf("re-Install() error = %v", err)
	}
	if err := backend.Remove(ctx, metadata.Label); err != nil {
		t.Fatalf("second Remove() error = %v", err)
	}
}
