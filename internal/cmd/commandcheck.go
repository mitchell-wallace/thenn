package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type commandWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func checkCommand(command []string) []commandWarning {
	if len(command) == 0 {
		return nil
	}

	if shell, script, ok := shellCommand(command); ok {
		warnings := checkShellCommand(shell, script)
		if len(warnings) > 0 {
			return warnings
		}

		warnings = checkShellcheck(shell, script)
		if len(warnings) > 0 {
			return warnings
		}

		commands := tokenizeShellCommands(script)
		warnings = checkShellExecutables(commands)
		if len(warnings) > 0 {
			return warnings
		}

		warnings = checkCommandPreflight(commands)
		if len(warnings) > 0 {
			return warnings
		}

		return checkAgentCommands(commands)
	}

	warnings := checkDirectCommand(command)
	if len(warnings) > 0 {
		return warnings
	}
	warnings = checkCommandPreflight([][]string{command})
	if len(warnings) > 0 {
		return warnings
	}
	return checkAgentCommands([][]string{command})
}

func shellCommand(command []string) (string, string, bool) {
	if len(command) == 3 && (command[1] == "-c" || command[1] == "/c") {
		return command[0], command[2], true
	}
	return "", "", false
}

func checkShellCommand(shell, script string) []commandWarning {
	if runtime.GOOS == "windows" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, "-n", "-c", script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return nil
	}

	msg := strings.TrimSpace(stderr.String())
	if ctx.Err() != nil {
		msg = "shell syntax check timed out"
	} else if msg == "" {
		msg = err.Error()
	}

	return []commandWarning{{
		Code:    "shell-syntax",
		Message: fmt.Sprintf("command may have invalid shell syntax: %s", msg),
	}}
}

func checkShellcheck(shell, script string) []commandWarning {
	if runtime.GOOS == "windows" {
		return nil
	}

	shellcheck, err := exec.LookPath("shellcheck")
	if err != nil {
		return nil
	}

	shellName := filepath.Base(shell)
	shellName = strings.TrimPrefix(shellName, "-")
	if shellName == "" {
		shellName = "sh"
	}
	if shellName == "zsh" {
		// ShellCheck does not parse zsh; sh is the least surprising fallback.
		shellName = "sh"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, shellcheck, "-s", shellName, "-")
	cmd.Stdin = strings.NewReader(script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err == nil {
		return nil
	}

	msg := strings.TrimSpace(stdout.String())
	if msg == "" {
		msg = strings.TrimSpace(stderr.String())
	}
	if ctx.Err() != nil {
		msg = "ShellCheck timed out"
	} else if msg == "" {
		msg = err.Error()
	}

	return []commandWarning{{
		Code:    "shellcheck",
		Message: fmt.Sprintf("ShellCheck warning for delayed command: %s", oneLine(msg)),
	}}
}

func checkDirectCommand(command []string) []commandWarning {
	cmdName := command[0]
	if cmdName == "" {
		return nil
	}

	if strings.ContainsRune(cmdName, os.PathSeparator) {
		info, err := os.Stat(cmdName)
		if err != nil {
			return []commandWarning{{Code: "command-not-found", Message: fmt.Sprintf("delayed command executable was not found: %s", cmdName)}}
		}
		if info.IsDir() {
			return []commandWarning{{Code: "command-not-executable", Message: fmt.Sprintf("delayed command points to a directory: %s", cmdName)}}
		}
		return nil
	}

	if _, err := exec.LookPath(cmdName); err != nil {
		return []commandWarning{{Code: "command-not-found", Message: fmt.Sprintf("delayed command executable was not found in PATH: %s", cmdName)}}
	}
	return nil
}

func checkShellExecutables(commands [][]string) []commandWarning {
	var warnings []commandWarning
	for _, command := range commands {
		if len(command) == 0 {
			continue
		}
		cmdName := command[0]
		if strings.Contains(cmdName, "=") && !strings.HasPrefix(cmdName, "=") {
			if len(command) == 1 {
				continue
			}
			cmdName = command[1]
		}
		if skipShellExecutableCheck(cmdName) {
			continue
		}
		warnings = append(warnings, checkDirectCommand([]string{cmdName})...)
	}
	return warnings
}

func skipShellExecutableCheck(cmdName string) bool {
	if cmdName == "" || strings.HasPrefix(cmdName, "$") || strings.Contains(cmdName, "=") {
		return true
	}
	switch cmdName {
	case "alias", "bg", "break", "case", "cd", "command", "continue", "do", "done", "elif", "else", "eval", "exec", "exit", "export", "false", "fc", "fg", "fi", "for", "function", "getopts", "hash", "if", "jobs", "local", "printf", "pwd", "read", "readonly", "return", "set", "shift", "test", "then", "times", "trap", "true", "type", "ulimit", "umask", "unalias", "unset", "until", "wait", "while", "{", "}", "!":
		return true
	}
	return false
}

func checkAgentCommands(commands [][]string) []commandWarning {
	var warnings []commandWarning
	for _, command := range commands {
		if len(command) == 0 {
			continue
		}

		name := filepath.Base(command[0])
		switch name {
		case "codex", "claude", "opencode", "agy":
			if warning, ok := checkAgentCommand(name, command[1:]); ok {
				warnings = append(warnings, warning)
			}
		}
	}
	return warnings
}

func checkAgentCommand(name string, args []string) (commandWarning, bool) {
	help, err := agentHelp(name)
	if err != nil {
		return commandWarning{}, false
	}
	if hasResumeFlag(name, args) {
		return commandWarning{}, false
	}

	subcommands := parseHelpCommands(help)
	if len(subcommands) == 0 {
		return commandWarning{}, false
	}

	first, ok := firstCommandArg(name, args)
	if !ok {
		return commandWarning{}, false
	}

	if subcommands[first] {
		return commandWarning{}, false
	}

	if !looksLikeSubcommand(first) {
		return commandWarning{}, false
	}

	return commandWarning{
		Code:    "agent-cli",
		Message: fmt.Sprintf("%s does not list %q as a command in its live --help output", name, first),
	}, true
}

func hasResumeFlag(name string, args []string) bool {
	if name == "codex" {
		return false
	}
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "-c" || arg == "--continue" {
			return true
		}
		if arg != "" && !strings.HasPrefix(arg, "-") {
			return false
		}
	}
	return false
}

func agentHelp(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "--help")
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil && len(out) == 0 {
		return "", err
	}
	return string(out), nil
}

func parseHelpCommands(help string) map[string]bool {
	commands := make(map[string]bool)
	inCommands := false
	for _, line := range strings.Split(help, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "Commands:" || trimmed == "Available subcommands:" {
			inCommands = true
			continue
		}
		if !inCommands {
			continue
		}
		if trimmed == "" || strings.HasSuffix(trimmed, ":") {
			if len(commands) > 0 {
				break
			}
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		cmdName := fields[0]
		if strings.HasPrefix(cmdName, "-") {
			break
		}
		if cmdName == "opencode" && len(fields) > 1 {
			cmdName = fields[1]
		}
		cmdName = strings.Trim(cmdName, "[]<>")
		for _, alias := range strings.Split(cmdName, "|") {
			alias = strings.TrimSpace(alias)
			if alias != "" && !strings.ContainsAny(alias, "[]<>") {
				commands[alias] = true
			}
		}
	}
	return commands
}

func firstCommandArg(agent string, args []string) (string, bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if i+1 < len(args) {
				return args[i+1], true
			}
			return "", false
		}
		if strings.HasPrefix(arg, "-") {
			if optionTakesValue(agent, arg) && !strings.Contains(arg, "=") {
				i++
			}
			continue
		}
		return arg, true
	}
	return "", false
}

func optionTakesValue(agent, arg string) bool {
	if strings.Contains(arg, "=") {
		return false
	}
	if arg == "-c" {
		return agent == "codex"
	}
	switch arg {
	case "--continue", "--dangerously-skip-permissions", "--help", "-h", "--version", "-v", "--print", "-p":
		return false
	}
	return strings.HasPrefix(arg, "--") || (strings.HasPrefix(arg, "-") && len([]rune(arg)) == 2)
}

func looksLikeSubcommand(arg string) bool {
	if arg == "" || strings.ContainsAny(arg, `/\\.`) {
		return false
	}
	if strings.ContainsAny(arg, " \t\n\r") {
		return false
	}
	return strings.ToLower(arg) == arg
}

func tokenizeShellCommands(script string) [][]string {
	var commands [][]string
	var current []string
	var token strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	flushToken := func() {
		if token.Len() > 0 {
			current = append(current, token.String())
			token.Reset()
		}
	}
	flushCommand := func() {
		flushToken()
		if len(current) > 0 {
			commands = append(commands, current)
			current = nil
		}
	}

	for _, r := range script {
		if escaped {
			token.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && !inSingle {
			escaped = true
			continue
		}
		if r == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if r == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if !inSingle && !inDouble {
			switch r {
			case ' ', '\t', '\n', '\r':
				flushToken()
				continue
			case ';', '|', '&':
				flushCommand()
				continue
			}
		}
		token.WriteRune(r)
	}
	flushCommand()
	return commands
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 400 {
		return s[:397] + "..."
	}
	return s
}

func emitCommandWarnings(warnings []commandWarning) {
	for _, warning := range warnings {
		if jsonOutput {
			b, _ := json.Marshal(map[string]any{
				"type":    "warning",
				"code":    warning.Code,
				"message": warning.Message,
			})
			fmt.Fprintln(os.Stderr, string(b))
			continue
		}
		fmt.Fprintf(os.Stderr, "thenn: warning: %s\n", warning.Message)
	}
}
