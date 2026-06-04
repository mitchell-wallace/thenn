//go:build !windows
// +build !windows

package timer

import (
	"os"
	"time"

	"golang.org/x/term"
)

func (r *Runner) listenInput(pauseChan, interruptChan chan<- struct{}, stopChan <-chan struct{}) {
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
