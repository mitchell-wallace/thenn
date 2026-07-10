//go:build linux && integration

package job

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestRealSystemdEveryWaitsAfterOneshotCompletion installs the units rendered
// by thenn into a real user manager. It is opt-in because it mutates the
// caller's user manager briefly and cannot run in containers without systemd.
func TestRealSystemdEveryWaitsAfterOneshotCompletion(t *testing.T) {
	if os.Getenv("THENN_SYSTEMD_INTEGRATION") != "1" {
		t.Skip("set THENN_SYSTEMD_INTEGRATION=1 to test against systemd --user")
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		t.Skipf("systemctl is unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if output, err := realSystemctl(ctx, "show-environment"); err != nil {
		t.Skipf("systemd user manager is unavailable: %v: %s", err, output)
	}

	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		t.Skip("XDG_RUNTIME_DIR is unavailable")
	}
	label := fmt.Sprintf("integration-%d", time.Now().UnixNano())
	serviceName := ServiceUnitName(label)
	timerName := TimerUnitName(label)
	tempDir := t.TempDir()
	eventsPath := filepath.Join(tempDir, "events")
	scriptPath := filepath.Join(tempDir, "run.sh")
	script := fmt.Sprintf("#!/bin/sh\nset -eu\nprintf 'start %%s\\n' \"$(date +%%s%%N)\" >> %s\nsleep 0.7\nprintf 'end %%s\\n' \"$(date +%%s%%N)\" >> %s\n", shellQuote(eventsPath), shellQuote(eventsPath))
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	schedule, err := ParseScheduleString("every 300ms", WithNow(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := NewMetadata(label, "every 300ms", schedule, []string{"true"}, tempDir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	units, err := RenderUnits(metadata, "/usr/bin/false")
	if err != nil {
		t.Fatal(err)
	}
	startLine := "ExecStart=/usr/bin/false job exec " + label
	units.Service = strings.Replace(units.Service, startLine, "ExecStart="+quoteSystemdArg(scriptPath, true), 1)
	if strings.Contains(units.Service, startLine) {
		t.Fatal("failed to replace rendered integration command")
	}

	servicePath := filepath.Join(tempDir, serviceName)
	timerPath := filepath.Join(tempDir, timerName)
	if err := os.WriteFile(servicePath, []byte(units.Service), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(timerPath, []byte(units.Timer), 0o600); err != nil {
		t.Fatal(err)
	}

	linkedService := filepath.Join(runtimeDir, "systemd", "user", serviceName)
	linkedTimer := filepath.Join(runtimeDir, "systemd", "user", timerName)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = realSystemctl(cleanupCtx, "disable", "--now", timerName)
		_, _ = realSystemctl(cleanupCtx, "stop", serviceName)
		_, _ = realSystemctl(cleanupCtx, "clean", "--what=state", timerName)
		_ = os.Remove(linkedTimer)
		_ = os.Remove(linkedService)
		_, _ = realSystemctl(cleanupCtx, "daemon-reload")
		_, _ = realSystemctl(cleanupCtx, "reset-failed", serviceName, timerName)
	})

	if output, err := realSystemctl(ctx, "--runtime", "link", servicePath, timerPath); err != nil {
		t.Fatalf("link units: %v: %s", err, output)
	}
	if output, err := realSystemctl(ctx, "daemon-reload"); err != nil {
		t.Fatalf("daemon-reload: %v: %s", err, output)
	}
	if output, err := realSystemctl(ctx, "start", timerName); err != nil {
		t.Fatalf("start timer: %v: %s", err, output)
	}

	events := waitForCompletedRuns(t, eventsPath, 3, 15*time.Second)
	var previousEnd int64
	for i := 0; i < 3; i++ {
		start := events[i*2]
		end := events[i*2+1]
		if start.kind != "start" || end.kind != "end" {
			t.Fatalf("events are not serialized start/end pairs: %#v", events)
		}
		if end.nsec <= start.nsec {
			t.Fatalf("run %d ended before it started: %#v", i, events)
		}
		if i > 0 && start.nsec-previousEnd < int64(200*time.Millisecond) {
			t.Fatalf("run %d started only %s after the prior completion; want fixed delay near 300ms: %#v", i, time.Duration(start.nsec-previousEnd), events)
		}
		previousEnd = end.nsec
	}
}

type integrationEvent struct {
	kind string
	nsec int64
}

func waitForCompletedRuns(t *testing.T, path string, runs int, timeout time.Duration) []integrationEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			events, parseErr := parseIntegrationEvents(string(data))
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			if len(events) >= runs*2 {
				return events[:runs*2]
			}
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d completed systemd runs", runs)
	return nil
}

func parseIntegrationEvents(contents string) ([]integrationEvent, error) {
	lines := strings.Fields(contents)
	if len(lines)%2 != 0 {
		return nil, fmt.Errorf("malformed event log %q", contents)
	}
	events := make([]integrationEvent, 0, len(lines)/2)
	for i := 0; i < len(lines); i += 2 {
		nsec, err := strconv.ParseInt(lines[i+1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse event timestamp: %w", err)
		}
		events = append(events, integrationEvent{kind: lines[i], nsec: nsec})
	}
	return events, nil
}

func realSystemctl(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "systemctl", append([]string{"--user"}, args...)...)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
