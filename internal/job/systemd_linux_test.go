//go:build linux

package job

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type fakeRunner struct {
	calls  []runCall
	output []byte
}

type runCall struct {
	name string
	args []string
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, runCall{name: name, args: append([]string(nil), args...)})
	return f.output, nil
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
		{name: "systemctl", args: []string{"--user", "status", "thenn-job-backup-daily.timer"}},
		{name: "systemctl", args: []string{"--user", "stop", "thenn-job-backup-daily.timer"}},
		{name: "systemctl", args: []string{"--user", "disable", "thenn-job-backup-daily.timer"}},
		{name: "systemctl", args: []string{"--user", "disable", "--now", "thenn-job-backup-daily.timer"}},
		{name: "journalctl", args: []string{"--user-unit", "thenn-job-backup-daily.service", "--no-pager", "-n", "20"}},
		{name: "systemctl", args: []string{"--user", "daemon-reload"}},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("systemctl calls = %#v, want %#v", runner.calls, want)
	}
}
