//go:build !windows
// +build !windows

package timer

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

var terminateSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}

func (r *Runner) listenInput(pauseChan, editChan, interruptChan chan<- struct{}, stopChan <-chan struct{}, doneChan chan struct{}) {
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
	fds := []unix.PollFd{
		{
			Fd:     int32(fd),
			Events: unix.POLLIN,
		},
	}

	for {
		select {
		case <-stopChan:
			return
		default:
			// Poll standard input with a 50ms timeout.
			n, err := unix.Poll(fds, 50)
			if err != nil {
				if errors.Is(err, syscall.EINTR) {
					continue
				}
				return
			}
			if n > 0 && (fds[0].Revents&unix.POLLIN) != 0 {
				nRead, err := os.Stdin.Read(buf[:])
				if err == nil && nRead > 0 {
					switch buf[0] {
					case ' ':
						select {
						case pauseChan <- struct{}{}:
						default:
						}
					case '\r', '\n':
						select {
						case editChan <- struct{}{}:
						default:
						}
						return
					case 27: // Esc
						select {
						case interruptChan <- struct{}{}:
						default:
						}
						return
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
