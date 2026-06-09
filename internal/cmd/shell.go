package cmd

import (
	"os"
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
		shellArgs = []string{"-c", command}
	}
	return append([]string{shell}, shellArgs...)
}
