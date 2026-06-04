//go:build !windows
// +build !windows

package timer

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/term"
)

func (r *Runner) listenInput(pauseChan, interruptChan chan<- struct{}, stopChan <-chan struct{}, doneChan chan struct{}) {
	defer close(doneChan)

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return
	}
	defer func() {
		_ = term.Restore(fd, oldState)
	}()

	var buf [1]byte
	for {
		select {
		case <-stopChan:
			return
		default:
			// Set a short read deadline so we can periodically check stopChan
			_ = os.Stdin.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			n, err := os.Stdin.Read(buf[:])
			// Reset deadline
			_ = os.Stdin.SetReadDeadline(time.Time{})

			if err == nil && n > 0 {
				switch buf[0] {
				case ' ':
					select {
					case pauseChan <- struct{}{}:
					default:
					}
				case 3: // Ctrl+C
					select {
					case interruptChan <- struct{}{}:
					default:
					}
					return
				}
			}
		}
	}
}

// ignoreSignals ignores SIGINT and SIGTERM during command execution.
func ignoreSignals() {
	signal.Ignore(os.Interrupt, syscall.SIGTERM)
}

// resetSignals restores default signal behavior.
func resetSignals() {
	signal.Reset(os.Interrupt, syscall.SIGTERM)
}

// ExtractExitCode extracts the exit code from an error, handling signals and standard POSIX shell exit codes.
func ExtractExitCode(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, exec.ErrNotFound) {
		return 127
	}
	if errors.Is(err, syscall.EACCES) {
		return 126
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		code := exitErr.ExitCode()
		if code == -1 {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				return 128 + int(status.Signal())
			}
		}
		return code
	}
	return 1
}
