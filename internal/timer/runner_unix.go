//go:build !windows
// +build !windows

package timer

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"golang.org/x/term"
)

func (r *Runner) listenInput(pauseChan chan<- struct{}, stopChan <-chan struct{}) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return
	}
	defer term.Restore(fd, oldState)

	// Set non-blocking read on stdin so we can check stopChan in the loop
	_ = syscall.SetNonblock(fd, true)
	defer func() {
		_ = syscall.SetNonblock(fd, false)
	}()

	var buf [1]byte
	for {
		select {
		case <-stopChan:
			return
		default:
			n, err := os.Stdin.Read(buf[:])
			if err == nil && n > 0 {
				if buf[0] == ' ' {
					select {
					case pauseChan <- struct{}{}:
					default:
					}
				} else if buf[0] == 3 { // Ctrl+C
					_ = term.Restore(fd, oldState)
					_ = syscall.SetNonblock(fd, false)
					fmt.Println()
					os.Exit(130)
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}
