package timer

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/mitchell-wallace/thenn/internal/testutil"
	"golang.org/x/term"
)

func TestGoroutineLeakWithPty(t *testing.T) {
	pty, tty, err := testutil.OpenPty()
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
