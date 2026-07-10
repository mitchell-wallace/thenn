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
		results := cleanupRealSystemdUnits(cleanupCtx, serviceName, timerName, linkedService, linkedTimer, realSystemctl, os.Remove)
		for _, result := range results {
			if result.err != nil {
				t.Error(formatIntegrationCleanupEvidence(result))
				continue
			}
			t.Log(formatIntegrationCleanupEvidence(result))
		}
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
		t.Log(formatIntegrationPairEvidence(i+1, start, end))
		if i > 0 {
			t.Log(formatIntegrationGapEvidence(i, i+1, previousEnd, start.nsec))
		}
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

type integrationCleanupResult struct {
	action string
	target string
	output string
	err    error
}

type integrationSystemctl func(context.Context, ...string) (string, error)

func formatIntegrationPairEvidence(run int, start, end integrationEvent) string {
	return fmt.Sprintf(
		"REAL_SYSTEMD_TIMING run=%d start_kind=%q start_ns=%d end_kind=%q end_ns=%d duration_ns=%d",
		run, start.kind, start.nsec, end.kind, end.nsec, end.nsec-start.nsec,
	)
}

func formatIntegrationGapEvidence(fromRun, toRun int, previousEnd, nextStart int64) string {
	return fmt.Sprintf(
		"REAL_SYSTEMD_GAP from_run=%d to_run=%d completion_gap_ns=%d required_min_ns=%d",
		fromRun, toRun, nextStart-previousEnd, int64(200*time.Millisecond),
	)
}

func formatIntegrationCleanupEvidence(result integrationCleanupResult) string {
	if result.err != nil {
		return fmt.Sprintf(
			"REAL_SYSTEMD_CLEANUP action=%q target=%q status=failed error=%q output=%q",
			result.action, result.target, result.err, result.output,
		)
	}
	return fmt.Sprintf(
		"REAL_SYSTEMD_CLEANUP action=%q target=%q status=ok output=%q",
		result.action, result.target, result.output,
	)
}

func cleanupRealSystemdUnits(
	ctx context.Context,
	serviceName string,
	timerName string,
	linkedService string,
	linkedTimer string,
	systemctl integrationSystemctl,
	unlink func(string) error,
) []integrationCleanupResult {
	results := make([]integrationCleanupResult, 0, 7)
	runSystemctl := func(action, target string, args ...string) {
		output, err := systemctl(ctx, args...)
		// cleanup is idempotent: "No matching resources found" (clean on a
		// timer with no Persistent= state) and "Unit ... not loaded"
		// (reset-failed after daemon-reload already unloaded the units) mean
		// the desired state is already reached, so they are recorded as ok
		// while preserving the original output as evidence.
		if err != nil && benignSystemdCleanupOutput(output) {
			err = nil
		}
		results = append(results, integrationCleanupResult{action: action, target: target, output: output, err: err})
	}
	runUnlink := func(action, path string) {
		results = append(results, integrationCleanupResult{action: action, target: path, err: unlink(path)})
	}

	runSystemctl("disable --now timer", timerName, "disable", "--now", timerName)
	runSystemctl("stop service", serviceName, "stop", serviceName)
	runSystemctl("clean --what=state timer", timerName, "clean", "--what=state", timerName)
	runUnlink("unlink timer", linkedTimer)
	runUnlink("unlink service", linkedService)
	runSystemctl("daemon-reload", "user manager", "daemon-reload")
	runSystemctl("reset-failed service and timer", serviceName+","+timerName, "reset-failed", serviceName, timerName)

	return results
}

// benignSystemdCleanupOutput reports whether output is a systemctl cleanup
// failure that indicates the target was already clean or unloaded, so the
// cleanup step is already complete and should be recorded as idempotent
// success rather than failure.
//
// Observed on Ubuntu 24.04:
//   - clean --what=state on a timer with no Persistent= state:
//     "Failed to clean unit <unit>.timer: No matching resources found."
//   - reset-failed on a unit the manager already unloaded after daemon-reload:
//     "Failed to reset failed state of unit <unit>: Unit <unit> not loaded."
func benignSystemdCleanupOutput(output string) bool {
	return strings.Contains(output, "No matching resources found") || strings.Contains(output, "not loaded")
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

func TestIntegrationEvidenceFormatting(t *testing.T) {
	start := integrationEvent{kind: "start", nsec: 1_000_000_000}
	end := integrationEvent{kind: "end", nsec: 1_700_000_000}
	if got, want := formatIntegrationPairEvidence(2, start, end), `REAL_SYSTEMD_TIMING run=2 start_kind="start" start_ns=1000000000 end_kind="end" end_ns=1700000000 duration_ns=700000000`; got != want {
		t.Fatalf("pair evidence = %q, want %q", got, want)
	}
	if got, want := formatIntegrationGapEvidence(1, 2, 700_000_000, 1_000_000_000), "REAL_SYSTEMD_GAP from_run=1 to_run=2 completion_gap_ns=300000000 required_min_ns=200000000"; got != want {
		t.Fatalf("gap evidence = %q, want %q", got, want)
	}
	result := integrationCleanupResult{action: "stop service", target: "example.service", output: "details"}
	if got, want := formatIntegrationCleanupEvidence(result), `REAL_SYSTEMD_CLEANUP action="stop service" target="example.service" status=ok output="details"`; got != want {
		t.Fatalf("successful cleanup evidence = %q, want %q", got, want)
	}
	result.err = fmt.Errorf("stop failed")
	if got, want := formatIntegrationCleanupEvidence(result), `REAL_SYSTEMD_CLEANUP action="stop service" target="example.service" status=failed error="stop failed" output="details"`; got != want {
		t.Fatalf("failed cleanup evidence = %q, want %q", got, want)
	}
}

func TestCleanupRealSystemdUnitsAttemptsEveryAction(t *testing.T) {
	var calls []string
	systemctl := func(_ context.Context, args ...string) (string, error) {
		call := strings.Join(args, " ")
		calls = append(calls, "systemctl "+call)
		if call == "stop example.service" {
			return "stop output", fmt.Errorf("stop failed")
		}
		return "", nil
	}
	unlink := func(path string) error {
		calls = append(calls, "unlink "+path)
		if path == "/runtime/example.timer" {
			return fmt.Errorf("unlink failed")
		}
		return nil
	}

	results := cleanupRealSystemdUnits(
		context.Background(),
		"example.service",
		"example.timer",
		"/runtime/example.service",
		"/runtime/example.timer",
		systemctl,
		unlink,
	)

	wantCalls := []string{
		"systemctl disable --now example.timer",
		"systemctl stop example.service",
		"systemctl clean --what=state example.timer",
		"unlink /runtime/example.timer",
		"unlink /runtime/example.service",
		"systemctl daemon-reload",
		"systemctl reset-failed example.service example.timer",
	}
	if got, want := strings.Join(calls, "\n"), strings.Join(wantCalls, "\n"); got != want {
		t.Fatalf("cleanup calls:\n%s\nwant:\n%s", got, want)
	}
	if len(results) != len(wantCalls) {
		t.Fatalf("cleanup returned %d results, want %d", len(results), len(wantCalls))
	}
	if results[1].err == nil || results[1].output != "stop output" {
		t.Fatalf("stop failure result = %#v", results[1])
	}
	if results[3].err == nil {
		t.Fatalf("unlink failure result = %#v", results[3])
	}
	for i, result := range results {
		if i != 1 && i != 3 && result.err != nil {
			t.Errorf("cleanup result %d unexpectedly failed: %#v", i, result)
		}
	}
}

func TestBenignSystemdCleanupOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "clean no resources", output: "Failed to clean unit x.timer: No matching resources found.", want: true},
		{name: "reset-failed not loaded", output: "Failed to reset failed state of unit x.service: Unit x.service not loaded.", want: true},
		{name: "genuine clean failure", output: "Failed to clean unit x.timer: Access denied", want: false},
		{name: "empty output", output: "", want: false},
		{name: "unrelated stop output", output: "stop output", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := benignSystemdCleanupOutput(tc.output); got != tc.want {
				t.Fatalf("benignSystemdCleanupOutput(%q) = %v, want %v", tc.output, got, tc.want)
			}
		})
	}
}

// TestCleanupRealSystemdUnitsTreatsBenignErrorsAsSuccess reproduces the exact
// cleanup failure observed on the Ubuntu 24.04 qualifying host (run
// 29080435784): clean found no Persistent= state and reset-failed ran after
// daemon-reload already unloaded the units. Both must be recorded as ok so
// all seven cleanup actions pass, while the original systemctl output is kept
// as evidence.
func TestCleanupRealSystemdUnitsTreatsBenignErrorsAsSuccess(t *testing.T) {
	systemctl := func(_ context.Context, args ...string) (string, error) {
		call := strings.Join(args, " ")
		switch {
		case strings.HasPrefix(call, "clean --what=state"):
			return "Failed to clean unit example.timer: No matching resources found.", fmt.Errorf("exit status 1")
		case strings.HasPrefix(call, "reset-failed"):
			return "Failed to reset failed state of unit example.service: Unit example.service not loaded.\nFailed to reset failed state of unit example.timer: Unit example.timer not loaded.", fmt.Errorf("exit status 1")
		default:
			return "", nil
		}
	}
	unlink := func(string) error { return nil }

	results := cleanupRealSystemdUnits(
		context.Background(),
		"example.service",
		"example.timer",
		"/runtime/example.service",
		"/runtime/example.timer",
		systemctl,
		unlink,
	)

	if len(results) != 7 {
		t.Fatalf("cleanup returned %d results, want 7", len(results))
	}
	for _, result := range results {
		if result.err != nil {
			t.Errorf("cleanup action %q unexpectedly failed: %v output=%q", result.action, result.err, result.output)
		}
	}
	if !strings.Contains(results[2].output, "No matching resources found") {
		t.Errorf("clean result lost benign evidence: %q", results[2].output)
	}
	if !strings.Contains(results[6].output, "not loaded") {
		t.Errorf("reset-failed result lost benign evidence: %q", results[6].output)
	}
}
