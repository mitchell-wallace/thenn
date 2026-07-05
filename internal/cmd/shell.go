package cmd

import (
	"os"
	"path/filepath"
	"runtime"
)

func resolveShell(command string) []string {
	var shell string
	var shellArgs []string
	if runtime.GOOS == "windows" {
		shell = os.Getenv("COMSPEC")
		if shell == "" {
			shell = "cmd.exe"
		}
		shellArgs = []string{"/c", command}
	} else {
		shell = os.Getenv("SHELL")
		if shell == "" {
			shell = "sh"
		}
		if supportsInteractiveShell(filepath.Base(shell)) {
			shellArgs = []string{"-ic", command}
		} else {
			shellArgs = []string{"-c", command}
		}
	}
	return append([]string{shell}, shellArgs...)
}

func supportsInteractiveShell(name string) bool {
	switch name {
	case "bash", "zsh", "fish", "ksh":
		return true
	}
	return false
}
