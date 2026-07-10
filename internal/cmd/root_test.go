package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mitchell-wallace/thenn/internal/job"
)

var binaryPath string

func TestMain(m *testing.M) {
	// Create a temporary directory for the test binary
	tmpDir, err := os.MkdirTemp("", "thenn-test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	binaryPath = filepath.Join(tmpDir, "thenn")
	if err := os.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "config")); err != nil {
		panic(err)
	}

	// Compile thenn binary
	cmd := exec.Command("go", "build", "-o", binaryPath, "../../cmd/thenn")
	if err := cmd.Run(); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

func runThenn(args ...string) (stdout, stderr string, exitCode int, err error) {
	return runThennInDir("", args...)
}

func runThennInDir(dir string, args ...string) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.Command(binaryPath, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()
	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	if runErr != nil {
		if exitError, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			err = runErr
		}
	} else {
		exitCode = 0
	}
	return
}

func installFakeSystemd(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "systemd.log")
	systemctl := `#!/bin/sh
printf 'systemctl' >> "$THENN_FAKE_SYSTEMD_LOG"
for arg in "$@"; do printf ' %s' "$arg" >> "$THENN_FAKE_SYSTEMD_LOG"; done
printf '\n' >> "$THENN_FAKE_SYSTEMD_LOG"
if [ "$1" = "--user" ] && [ "$2" = "status" ]; then
  printf 'fake timer status\n'
fi
exit 0
`
	journalctl := `#!/bin/sh
printf 'journalctl' >> "$THENN_FAKE_SYSTEMD_LOG"
for arg in "$@"; do printf ' %s' "$arg" >> "$THENN_FAKE_SYSTEMD_LOG"; done
printf '\n' >> "$THENN_FAKE_SYSTEMD_LOG"
printf 'fake journal output\n'
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(systemctl), 0o755); err != nil {
		t.Fatalf("write fake systemctl: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "journalctl"), []byte(journalctl), 0o755); err != nil {
		t.Fatalf("write fake journalctl: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("THENN_FAKE_SYSTEMD_LOG", logPath)
	return logPath
}

func TestE2E_Success(t *testing.T) {
	stdout, stderr, code, err := runThenn("10ms", "-q", "--", "echo", "hello")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d. stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "hello") {
		t.Errorf("expected stdout to contain 'hello', got %q", stdout)
	}
}

func TestE2E_CommandFlag_Success(t *testing.T) {
	stdout, stderr, code, err := runThenn("10ms", "-q", "-c", "echo hello")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d. stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "hello") {
		t.Errorf("expected stdout to contain 'hello', got %q", stdout)
	}
	if strings.Contains(stderr, "thenn: warning:") {
		t.Errorf("expected no warning for valid command, got stderr %q", stderr)
	}
}

func TestE2E_CommandFlag_WarnsButDoesNotBlock(t *testing.T) {
	_, stderr, code, err := runThenn("10ms", "-q", "-c", "if true; then")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if code == 0 {
		t.Errorf("expected delayed invalid command to fail after warning, got exit 0")
	}
	if !strings.Contains(stderr, "thenn: warning:") || !strings.Contains(stderr, "shell syntax") {
		t.Errorf("expected proactive shell syntax warning, got stderr %q", stderr)
	}
}

func TestE2E_CommandFlag_WarningJsonOutput(t *testing.T) {
	_, stderr, code, err := runThenn("--json-output", "10ms", "-q", "-c", "if true; then")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if code == 0 {
		t.Errorf("expected delayed invalid command to fail after warning, got exit 0")
	}
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected warning and error JSON lines, got %q", stderr)
	}
	var warning map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &warning); err != nil {
		t.Fatalf("failed to parse warning JSON %q: %v", lines[0], err)
	}
	if warning["type"] != "warning" || warning["code"] != "shell-syntax" {
		t.Errorf("unexpected warning JSON: %#v", warning)
	}
}

func TestE2E_CommandCheckingDisabledByConfig(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	configDir := filepath.Join(configHome, "thenn")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("make config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"disable_command_checking":true}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, stderr, code, err := runThenn("10ms", "-q", "--", "missing-command")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if code == 0 {
		t.Errorf("expected delayed invalid command to fail, got exit 0")
	}
	if strings.Contains(stderr, "thenn: warning:") {
		t.Errorf("expected command checking to be disabled, got stderr %q", stderr)
	}
}

func TestE2E_CommandFlag_MutualExclusion(t *testing.T) {
	_, stderr, code, err := runThenn("10ms", "-q", "-c", "echo hello", "echo", "world")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if code == 0 {
		t.Errorf("expected non-zero exit code for mutual exclusion, got 0")
	}
	if !strings.Contains(stderr, "cannot specify both -c/--command and positional command arguments") {
		t.Errorf("expected stderr to explain mutual exclusion, got %q", stderr)
	}
}

func TestE2E_BareCommand_NonInteractive(t *testing.T) {
	_, stderr, code, err := runThenn("-q")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr, "a duration must be specified (e.g. 10s, 5m, 2h)") {
		t.Errorf("unexpected stderr: %q", stderr)
	}
}

func TestE2E_InvalidDuration(t *testing.T) {
	_, stderr, code, err := runThenn("10", "-q")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr, "invalid duration: invalid duration format: \"10\"") {
		t.Errorf("unexpected stderr: %q", stderr)
	}
}

func TestE2E_InvalidDuration_Json(t *testing.T) {
	_, stderr, code, err := runThenn("--json-output", "10", "-q")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &result); err != nil {
		t.Fatalf("failed to parse JSON stderr %q: %v", stderr, err)
	}

	if result["exitCode"] != 1.0 {
		t.Errorf("expected exitCode 1, got %v", result["exitCode"])
	}
	if !strings.Contains(result["error"].(string), "invalid duration") {
		t.Errorf("unexpected error in JSON: %v", result["error"])
	}
}

func TestProtectCommandFlags_InsertsSeparatorBeforeCommand(t *testing.T) {
	args := protectCommandFlags([]string{"1ms", "fd", "-z"})
	joined := strings.Join(args, " ")
	if joined != "1ms -- fd -z" {
		t.Fatalf("expected command flags to be protected, got %q", joined)
	}
}

func TestProtectCommandFlags_PreservesThennFlagsBeforeCommand(t *testing.T) {
	args := protectCommandFlags([]string{"1ms", "-q", "fd", "-z"})
	joined := strings.Join(args, " ")
	if joined != "1ms -q -- fd -z" {
		t.Fatalf("expected thenn flags before command to be preserved, got %q", joined)
	}
}

func TestProtectCommandFlags_LeavesUnknownThennFlagForCobra(t *testing.T) {
	args := protectCommandFlags([]string{"1ms", "--quieet"})
	joined := strings.Join(args, " ")
	if joined != "1ms --quieet" {
		t.Fatalf("expected unknown thenn flag to be left for Cobra, got %q", joined)
	}
}

func TestProtectCommandFlags_LeavesJobSubcommand(t *testing.T) {
	args := protectCommandFlags([]string{"job", "list"})
	joined := strings.Join(args, " ")
	if joined != "job list" {
		t.Fatalf("expected job subcommand args to be preserved, got %q", joined)
	}
}

func TestE2E_JobBareCommandShowsHelp(t *testing.T) {
	stdout, stderr, code, err := runThenn("job")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Manage scriptable scheduled jobs") || !strings.Contains(stdout, "Usage:") {
		t.Fatalf("expected job help, got stdout %q stderr %q", stdout, stderr)
	}
}

func TestE2E_JobCommandsFailClearlyWithoutSystemd(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	tests := [][]string{
		{"job", "every", "15m", "--label", "every-test", "--", "echo", "ok"},
		{"job", "daily", "at", "9pm", "--label", "daily-test", "--", "echo", "ok"},
		{"job", "once", "at", "2099-01-01", "--label", "once-test", "--", "echo", "ok"},
		{"job", "weekly", "monday", "at", "9pm", "--label", "weekly-test", "--", "echo", "ok"},
		{"job", "weekdays", "at", "9pm", "--label", "weekdays-test", "--", "echo", "ok"},
		{"job", "list"},
		{"job", "show", "missing"},
		{"job", "run", "missing"},
		{"job", "pause", "missing"},
		{"job", "resume", "missing"},
		{"job", "remove", "missing"},
		{"job", "logs", "missing"},
	}
	wantStderr := "thenn: " + job.ErrSystemctlNotFound.Error() + "\n"
	for _, args := range tests {
		args := args
		t.Run(strings.Join(args[1:], "_"), func(t *testing.T) {
			stdout, stderr, code, err := runThenn(args...)
			if err != nil {
				t.Fatalf("run failed: %v", err)
			}
			if code != 1 {
				t.Fatalf("exit code = %d, want 1; stdout %q stderr %q", code, stdout, stderr)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if stderr != wantStderr {
				t.Fatalf("stderr = %q, want one clear error %q", stderr, wantStderr)
			}
		})
	}
}

func TestE2E_JobCreateAndManageWithFakeSystemd(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	logPath := installFakeSystemd(t)

	stdout, stderr, code, err := runThenn("job", "every", "15m", "--label", "backup", "--", "sh", "-c", "printf job")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected create exit code 0, got %d. stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "created job backup") {
		t.Fatalf("unexpected create stdout %q", stdout)
	}

	metadataPath := filepath.Join(configHome, "thenn", "jobs", "backup.json")
	metadata, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if !strings.Contains(string(metadata), `"label": "backup"`) || !strings.Contains(string(metadata), `"sh"`) {
		t.Fatalf("metadata missing expected fields:\n%s", metadata)
	}

	unitDir := filepath.Join(configHome, "systemd", "user")
	service, err := os.ReadFile(filepath.Join(unitDir, "thenn-job-backup.service"))
	if err != nil {
		t.Fatalf("read service unit: %v", err)
	}
	if !strings.Contains(string(service), " job exec backup") {
		t.Fatalf("service unit missing exec command:\n%s", service)
	}
	timer, err := os.ReadFile(filepath.Join(unitDir, "thenn-job-backup.timer"))
	if err != nil {
		t.Fatalf("read timer unit: %v", err)
	}
	if !strings.Contains(string(timer), "OnUnitInactiveSec=15m") {
		t.Fatalf("timer unit missing interval:\n%s", timer)
	}

	stdout, stderr, code, err = runThenn("job", "list")
	if err != nil || code != 0 {
		t.Fatalf("list failed code %d err %v stderr %s", code, err, stderr)
	}
	if !strings.Contains(stdout, "backup") || !strings.Contains(stdout, "every 15m") {
		t.Fatalf("unexpected list stdout %q", stdout)
	}

	stdout, stderr, code, err = runThenn("job", "show", "backup")
	if err != nil || code != 0 {
		t.Fatalf("show failed code %d err %v stderr %s", code, err, stderr)
	}
	if !strings.Contains(stdout, "Label: backup") || !strings.Contains(stdout, "fake timer status") {
		t.Fatalf("unexpected show stdout %q", stdout)
	}

	stdout, stderr, code, err = runThenn("job", "logs", "backup")
	if err != nil || code != 0 {
		t.Fatalf("logs failed code %d err %v stderr %s", code, err, stderr)
	}
	if !strings.Contains(stdout, "fake journal output") {
		t.Fatalf("unexpected logs stdout %q", stdout)
	}

	for _, tc := range []struct {
		args []string
		want string
	}{
		{args: []string{"job", "pause", "backup"}, want: "paused job backup"},
		{args: []string{"job", "resume", "backup"}, want: "resumed job backup"},
		{args: []string{"job", "run", "backup"}, want: "started job backup"},
		{args: []string{"job", "remove", "backup"}, want: "removed job backup"},
	} {
		stdout, stderr, code, err = runThenn(tc.args...)
		if err != nil || code != 0 {
			t.Fatalf("%s failed code %d err %v stderr %s", strings.Join(tc.args, " "), code, err, stderr)
		}
		if !strings.Contains(stdout, tc.want) {
			t.Fatalf("expected %q in stdout, got %q", tc.want, stdout)
		}
	}
	if _, err := os.Stat(metadataPath); !os.IsNotExist(err) {
		t.Fatalf("expected metadata to be removed, stat err = %v", err)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake systemd log: %v", err)
	}
	log := string(logData)
	for _, want := range []string{
		"systemctl --user daemon-reload",
		"systemctl --user enable --now thenn-job-backup.timer",
		"systemctl --user status thenn-job-backup.timer thenn-job-backup.service",
		"journalctl --user-unit thenn-job-backup.service --no-pager -n 80",
		"systemctl --user disable --now thenn-job-backup.timer",
		"systemctl --user stop thenn-job-backup.service",
		"systemctl --user start thenn-job-backup.service",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("fake systemd log missing %q:\n%s", want, log)
		}
	}
}

func TestE2E_JobCreateRejectsDuplicateLabel(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	installFakeSystemd(t)

	_, stderr, code, err := runThenn("job", "every", "15m", "--label", "backup", "--", "echo", "one")
	if err != nil || code != 0 {
		t.Fatalf("initial create failed code %d err %v stderr %s", code, err, stderr)
	}
	_, stderr, code, err = runThenn("job", "every", "30m", "--label", "backup", "--", "echo", "two")
	if err != nil {
		t.Fatalf("duplicate create failed to start: %v", err)
	}
	if code == 0 {
		t.Fatal("duplicate create succeeded, want failure")
	}
	if !strings.Contains(stderr, `job "backup" already exists`) {
		t.Fatalf("duplicate create stderr = %q", stderr)
	}
}

func TestE2E_JobCreateRollsBackMetadataOnSystemdFailure(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	dir := t.TempDir()
	logPath := filepath.Join(dir, "systemd.log")
	systemctl := `#!/bin/sh
printf 'systemctl' >> "$THENN_FAKE_SYSTEMD_LOG"
for arg in "$@"; do printf ' %s' "$arg" >> "$THENN_FAKE_SYSTEMD_LOG"; done
printf '\n' >> "$THENN_FAKE_SYSTEMD_LOG"
if [ "$2" = "enable" ]; then exit 2; fi
exit 0
`
	journalctl := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(systemctl), 0o755); err != nil {
		t.Fatalf("write fake systemctl: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "journalctl"), []byte(journalctl), 0o755); err != nil {
		t.Fatalf("write fake journalctl: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("THENN_FAKE_SYSTEMD_LOG", logPath)

	_, stderr, code, err := runThenn("job", "every", "15m", "--label", "broken", "--", "echo", "one")
	if err != nil {
		t.Fatalf("create failed to start: %v", err)
	}
	if code == 0 {
		t.Fatal("create succeeded despite fake systemd failure")
	}
	if _, err := os.Stat(filepath.Join(configHome, "thenn", "jobs", "broken.json")); !os.IsNotExist(err) {
		t.Fatalf("metadata was not rolled back, stat err = %v stderr = %s", err, stderr)
	}
	unitDir := filepath.Join(configHome, "systemd", "user")
	for _, name := range []string{"thenn-job-broken.service", "thenn-job-broken.timer"} {
		if _, err := os.Stat(filepath.Join(unitDir, name)); !os.IsNotExist(err) {
			t.Fatalf("unit %s was not rolled back, stat err = %v stderr = %s", name, err, stderr)
		}
	}
}

func TestE2E_JobExecUsesStoredCWDAndReturnsChildStatus(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	logPath := installFakeSystemd(t)
	workDir := t.TempDir()

	_, stderr, code, err := runThennInDir(workDir, "job", "once", "at", "2099-01-01", "--label", "once-cwd", "--", "sh", "-c", "pwd; exit 7")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected create exit code 0, got %d. stderr: %s", code, stderr)
	}

	stdout, _, code, err := runThenn("job", "exec", "once-cwd")
	if err != nil {
		t.Fatalf("exec failed to start: %v", err)
	}
	if code != 7 {
		t.Fatalf("expected child exit code 7, got %d", code)
	}
	if strings.TrimSpace(stdout) != workDir {
		t.Fatalf("expected exec cwd %q, got stdout %q", workDir, stdout)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake systemd log: %v", err)
	}
	if !strings.Contains(string(logData), "systemctl --user disable --now thenn-job-once-cwd.timer") {
		t.Fatalf("expected once exec to disable timer, log:\n%s", logData)
	}
}

func TestE2E_CommandMatchingDuration(t *testing.T) {
	_, stderr, code, err := runThenn("10ms", "-q", "--", "10ms")
	// Since "10ms" is not an executable command, it should fail to run the command, but parsing should succeed
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	// It should fail to find command "10ms" (exit code 1 or 127/etc depending on OS, but not "invalid duration: empty duration")
	if strings.Contains(stderr, "invalid duration") {
		t.Errorf("expected parsing to succeed, but got error: %s", stderr)
	}
	if code == 0 {
		t.Errorf("expected execution to fail (command 10ms doesn't exist), but got exit 0")
	}
}

func TestE2E_VersionFlag(t *testing.T) {
	stdout, _, code, err := runThenn("--version")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("expected version output, got empty")
	}
}

func TestE2E_ConfigHelp(t *testing.T) {
	stdout, stderr, code, err := runThenn("config", "--help")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d. stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Configure thenn interactively") {
		t.Errorf("expected config help output, got stdout %q stderr %q", stdout, stderr)
	}
}
