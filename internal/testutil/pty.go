//go:build !windows

package testutil

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

func OpenPty() (pty, tty *os.File, err error) {
	pty, err = os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}

	var snum int
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, pty.Fd(), uintptr(syscall.TIOCGPTN), uintptr(unsafe.Pointer(&snum))) //nolint:gosec
	if errno != 0 {
		_ = pty.Close()
		return nil, nil, errno
	}

	var unlock int
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, pty.Fd(), uintptr(syscall.TIOCSPTLCK), uintptr(unsafe.Pointer(&unlock))) //nolint:gosec
	if errno != 0 {
		_ = pty.Close()
		return nil, nil, errno
	}

	sname := fmt.Sprintf("/dev/pts/%d", snum)
	sfd, err := syscall.Open(sname, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		_ = pty.Close()
		return nil, nil, err
	}
	tty = os.NewFile(uintptr(sfd), sname)

	return pty, tty, nil
}
