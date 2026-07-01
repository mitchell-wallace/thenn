package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
	cmd := exec.Command(binaryPath, args...)
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
