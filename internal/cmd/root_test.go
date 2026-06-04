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

	var result map[string]interface{}
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
