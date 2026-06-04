//go:build windows
// +build windows

package timer

func (r *Runner) listenInput(pauseChan chan<- struct{}, stopChan <-chan struct{}) {
	// No-op fallback for Windows
}
