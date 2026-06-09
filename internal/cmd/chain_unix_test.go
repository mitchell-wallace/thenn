//go:build !windows
// +build !windows

package cmd

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/mitchell-wallace/thenn/internal/testutil"
)

func TestE2E_CommandChaining_RealTerminal(t *testing.T) {
	pty, tty, err := testutil.OpenPty()
	if err != nil {
		t.Skip("PTY creation not supported/failed:", err)
	}
	defer pty.Close()
	defer tty.Close()

	// Execute through a real shell to test the standard 'thenn && ...' chaining syntax
	cmd := exec.Command("sh", "-c", binaryPath+" 50ms && echo 'chain-success'")
	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start command: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("command exited with error: %v", err)
		}
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("Test timed out! Command chained under real terminal hung.")
	}

	// Close parent's reference to tty so that reading from pty will receive EOF
	_ = tty.Close()

	// Read all data captured by the master end of the PTY in a goroutine with a timeout
	readDone := make(chan struct{})
	var output string
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(pty)
		output = buf.String()
		close(readDone)
	}()

	select {
	case <-readDone:
	case <-time.After(1 * time.Second):
		t.Fatal("Reading from PTY timed out!")
	}

	t.Logf("PTY Output:\n%s", output)

	if !strings.Contains(output, "chain-success") {
		t.Errorf("expected output to contain 'chain-success', got %q", output)
	}

	if !strings.Contains(output, "0s") {
		t.Errorf("expected output to show '0s' target time, got %q", output)
	}
}

func TestE2E_TargetTime_RealTerminal(t *testing.T) {
	pty, tty, err := testutil.OpenPty()
	if err != nil {
		t.Skip("PTY creation not supported/failed:", err)
	}
	defer pty.Close()
	defer tty.Close()

	// Calculate a target time 5 minutes in the past
	pastTime := time.Now().Add(-5 * time.Minute)
	// Format as 12h: e.g. "11:03am" or "11:03pm" -> we'll use lowercase "11:03a" or "11:03p"
	hourMin := pastTime.Format("3:04")
	ampm := "a"
	if pastTime.Hour() >= 12 {
		ampm = "p"
	}
	targetArg := fmt.Sprintf("%s%s", hourMin, ampm) // e.g. "11:03a"

	// Run thenn in the real terminal
	cmd := exec.Command(binaryPath, targetArg)
	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start command: %v", err)
	}

	// Wait 200ms for it to print the first countdown line, then kill it (since it's waiting 24 hours)
	time.Sleep(200 * time.Millisecond)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	_ = tty.Close()

	// Read output from PTY
	readDone := make(chan struct{})
	var output string
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(pty)
		output = buf.String()
		close(readDone)
	}()

	select {
	case <-readDone:
	case <-time.After(1 * time.Second):
		t.Fatal("Reading from PTY timed out!")
	}

	t.Logf("PTY Output:\n%s", output)

	// Since the target was 5 minutes in the past, it should be set to tomorrow.
	if !strings.Contains(output, "tomorrow") {
		t.Errorf("expected output to indicate tomorrow, got %q", output)
	}

	expectedTargetStr := pastTime.Format("3:04") + ampm
	if !strings.Contains(output, expectedTargetStr) {
		t.Errorf("expected output to contain target time %q, got %q", expectedTargetStr, output)
	}
}

func TestE2E_CommandFlag_RealTerminal(t *testing.T) {
	pty, tty, err := testutil.OpenPty()
	if err != nil {
		t.Skip("PTY creation not supported/failed:", err)
	}
	defer pty.Close()
	defer tty.Close()

	// Run thenn with -c in a real terminal
	cmd := exec.Command(binaryPath, "10ms", "-c", "echo 'c-flag-success'")
	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start command: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("command exited with error: %v", err)
		}
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("Test timed out! Command Flag under real terminal hung.")
	}

	_ = tty.Close()

	// Read output from PTY
	readDone := make(chan struct{})
	var output string
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(pty)
		output = buf.String()
		close(readDone)
	}()

	select {
	case <-readDone:
	case <-time.After(1 * time.Second):
		t.Fatal("Reading from PTY timed out!")
	}

	t.Logf("PTY Output:\n%s", output)

	if !strings.Contains(output, "c-flag-success") {
		t.Errorf("expected output to contain 'c-flag-success', got %q", output)
	}
}

