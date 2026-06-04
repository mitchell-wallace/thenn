package timer

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/term"
)

func openPty() (pty, tty *os.File, err error) {
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
	// Open slave using syscall with O_NONBLOCK so Go's runtime registers it with the netpoller
	sfd, err := syscall.Open(sname, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		pty.Close()
		return nil, nil, err
	}
	tty = os.NewFile(uintptr(sfd), sname)

	return pty, tty, nil
}

func TestGoroutineLeakWithPty(t *testing.T) {
	pty, tty, err := openPty()
	if err != nil {
		t.Skip("PTY creation not supported/failed:", err)
	}
	defer pty.Close()
	defer tty.Close()

	// Redirect os.Stdin
	oldStdin := os.Stdin
	os.Stdin = tty
	defer func() {
		os.Stdin = oldStdin
	}()

	initial := runtime.NumGoroutine()

	isTerm := term.IsTerminal(int(os.Stdin.Fd()))
	t.Logf("IsStdinTerminal: %v", isTerm)
	if !isTerm {
		t.Fatalf("PTY is not recognized as a terminal")
	}

	r := NewRunner(50*time.Millisecond, nil, true)
	err = r.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Wait for goroutine cleanup
	time.Sleep(150 * time.Millisecond)

	final := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d, Final: %d", initial, final)
	if final > initial {
		buf := make([]byte, 10240)
		n := runtime.Stack(buf, true)
		t.Errorf("Goroutine leak detected: went from %d to %d\nStack traces:\n%s", initial, final, string(buf[:n]))
	}
}
