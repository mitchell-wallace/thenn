//go:build !windows
// +build !windows

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func openPtyTest() (pty, tty *os.File, err error) {
	pty, err = os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}

	// TIOCGPTN gets the slave pty number
	var snum int
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, pty.Fd(), uintptr(syscall.TIOCGPTN), uintptr(unsafe.Pointer(&snum)))
	if errno != 0 {
		pty.Close()
		return nil, nil, errno
	}

	// TIOCSPTLCK unlocks the slave pty
	var unlock int
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, pty.Fd(), uintptr(syscall.TIOCSPTLCK), uintptr(unsafe.Pointer(&unlock)))
	if errno != 0 {
		pty.Close()
		return nil, nil, errno
	}

	sname := fmt.Sprintf("/dev/pts/%d", snum)
	// Open slave using syscall with O_NONBLOCK so it doesn't block on Open
	sfd, err := syscall.Open(sname, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		pty.Close()
		return nil, nil, err
	}
	tty = os.NewFile(uintptr(sfd), sname)

	return pty, tty, nil
}

func TestE2E_CommandChaining_RealTerminal(t *testing.T) {
	pty, tty, err := openPtyTest()
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
