//go:build windows
// +build windows

package timer

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

var terminateSignals = []os.Signal{os.Interrupt}

func (r *Runner) listenInput(pauseChan, editChan, interruptChan chan<- struct{}, stopChan <-chan struct{}, doneChan chan struct{}) {
	close(doneChan)
}

// ignoreSignals ignores standard interrupt signals on Windows.
func ignoreSignals() {
	signal.Ignore(os.Interrupt)
}

// resetSignals restores default signal behavior.
func resetSignals() {
	signal.Reset(os.Interrupt)
}

// ExtractExitCode extracts the exit code from an error on Windows.
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
		return exitErr.ExitCode()
	}
	return 1
}
