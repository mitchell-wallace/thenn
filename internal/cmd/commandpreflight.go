package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

type commandSpec struct {
	Name             string
	Options          map[string]optionSpec
	Subcommands      map[string]bool
	HasSubcommandArg bool
	PathOnlyOperands bool
}

type optionSpec struct {
	TakesValue bool
}

type commandSpecProvider interface {
	Spec(name string) (commandSpec, bool)
}

type defaultSpecProvider struct{}

type gitSpecProvider struct{}

var commandSpecProviders = []commandSpecProvider{
	gitSpecProvider{},
	defaultSpecProvider{},
}

func checkCommandPreflight(commands [][]string) []commandWarning {
	for _, command := range commands {
		if len(command) < 2 || skipShellExecutableCheck(command[0]) {
			continue
		}
		if warning, ok := checkCommandAgainstSpec(command); ok {
			return []commandWarning{warning}
		}
	}
	return nil
}

func checkCommandAgainstSpec(command []string) (commandWarning, bool) {
	spec, ok := loadCommandSpec(command[0])
	if !ok {
		return commandWarning{}, false
	}

	args, warning, ok := validateOptions(spec, command[1:])
	if ok {
		return warning, true
	}
	if warning, ok := validateSubcommand(spec, args); ok {
		return warning, true
	}
	if warning, ok := validatePathOperands(spec, args); ok {
		return warning, true
	}
	return commandWarning{}, false
}

func loadCommandSpec(name string) (commandSpec, bool) {
	for _, provider := range commandSpecProviders {
		if spec, ok := provider.Spec(name); ok {
			return spec, true
		}
	}
	return commandSpec{}, false
}

func (defaultSpecProvider) Spec(name string) (commandSpec, bool) {
	help, ok := commandHelp(name, "--help")
	if !ok {
		return commandSpec{}, false
	}
	return parseCommandSpec(filepath.Base(name), help), true
}

func (gitSpecProvider) Spec(name string) (commandSpec, bool) {
	if filepath.Base(name) != "git" {
		return commandSpec{}, false
	}
	help, ok := commandHelp(name, "--help")
	if !ok {
		return commandSpec{}, false
	}
	spec := parseCommandSpec("git", help)
	if allCommands, ok := commandHelp(name, "help", "-a"); ok {
		spec.Subcommands = parseCommandList(allCommands)
	}
	return spec, true
}

func commandHelp(name string, args ...string) (string, bool) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil || (err != nil && len(out) == 0) {
		return "", false
	}
	return string(out), true
}

func parseCommandSpec(name, help string) commandSpec {
	return commandSpec{
		Name:             name,
		Options:          parseOptions(help),
		Subcommands:      parseCommandList(help),
		HasSubcommandArg: helpHasSubcommandArg(help),
		PathOnlyOperands: helpHasOnlyPathOperands(help),
	}
}

func parseOptions(help string) map[string]optionSpec {
	options := make(map[string]optionSpec)
	for _, line := range strings.Split(help, "\n") {
		fields := strings.Fields(line)
		for i, field := range fields {
			for _, part := range strings.Split(field, ",") {
				name, takesValue, ok := parseOptionToken(part)
				if !ok {
					continue
				}
				if !takesValue && i+1 < len(fields) && looksLikeValuePlaceholder(fields[i+1]) {
					takesValue = true
				}
				options[name] = optionSpec{TakesValue: takesValue}
			}
		}
	}
	return options
}

func parseOptionToken(token string) (string, bool, bool) {
	token = strings.TrimSpace(token)
	token = strings.Trim(token, "[](),")
	token = strings.TrimRight(token, ";:.")
	if strings.HasPrefix(token, "--") {
		name := token
		takesValue := false
		if idx := strings.IndexAny(name, "=[<"); idx != -1 {
			takesValue = true
			name = name[:idx]
		}
		if len(name) > 2 && optionNameLooksValid(strings.TrimPrefix(name, "--")) {
			return name, takesValue, true
		}
	}
	if strings.HasPrefix(token, "-") && len([]rune(token)) == 2 {
		return token, false, true
	}
	return "", false, false
}

func optionNameLooksValid(name string) bool {
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			continue
		}
		return false
	}
	return name != ""
}

func looksLikeValuePlaceholder(s string) bool {
	s = strings.Trim(s, "[](),")
	return strings.HasPrefix(s, "<") || (s != "" && strings.ToUpper(s) == s && strings.ContainsFunc(s, unicode.IsLetter))
}

func parseCommandList(help string) map[string]bool {
	commands := make(map[string]bool)
	for _, line := range strings.Split(help, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "-") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 || !looksLikeSubcommand(fields[0]) {
			continue
		}
		if strings.HasPrefix(fields[1], "-") || strings.HasSuffix(fields[0], ":") {
			continue
		}
		commands[fields[0]] = true
	}
	return commands
}

func helpHasSubcommandArg(help string) bool {
	for _, line := range usageLines(help) {
		for _, field := range strings.Fields(strings.ToLower(line)) {
			field = strings.Trim(field, "[]<>")
			if field == "command" || field == "subcommand" || field == "cmd" {
				return true
			}
		}
	}
	return false
}

func helpHasOnlyPathOperands(help string) bool {
	lowerUsage := strings.ToLower(strings.Join(usageLines(help), " "))
	if strings.Contains(lowerUsage, "pattern") || strings.Contains(lowerUsage, "command") || strings.Contains(lowerUsage, "args") {
		return false
	}
	for _, line := range usageLines(help) {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "file") || strings.Contains(lower, "path") || strings.Contains(lower, "dir") || strings.Contains(lower, "directory") {
			return true
		}
	}
	return false
}

func usageLines(help string) []string {
	var lines []string
	inUsage := false
	for _, line := range strings.Split(help, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), "usage:") {
			lines = append(lines, trimmed)
			inUsage = true
			continue
		}
		if inUsage && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && trimmed != "" {
			lines = append(lines, trimmed)
			continue
		}
		inUsage = false
	}
	return lines
}

func validateOptions(spec commandSpec, args []string) ([]string, commandWarning, bool) {
	if len(spec.Options) == 0 {
		return args, commandWarning{}, false
	}

	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		if strings.HasPrefix(arg, "--") {
			name := arg
			if idx := strings.IndexRune(name, '='); idx != -1 {
				name = name[:idx]
			}
			option, ok := spec.Options[name]
			if !ok {
				return positional, commandArgWarning(spec.Name, fmt.Sprintf("does not list %q as an option in its live --help output", arg)), true
			}
			if option.TakesValue && !strings.Contains(arg, "=") && i+1 < len(args) {
				i++
			}
			continue
		}

		for j, r := range arg[1:] {
			name := "-" + string(r)
			option, ok := spec.Options[name]
			if !ok {
				return positional, commandArgWarning(spec.Name, fmt.Sprintf("does not list %q as an option in its live --help output", name)), true
			}
			if option.TakesValue {
				if j == len([]rune(arg[1:]))-1 && i+1 < len(args) {
					i++
				}
				break
			}
		}
	}
	return positional, commandWarning{}, false
}

func validateSubcommand(spec commandSpec, args []string) (commandWarning, bool) {
	if !spec.HasSubcommandArg || len(spec.Subcommands) == 0 || len(args) == 0 {
		return commandWarning{}, false
	}
	first := args[0]
	if spec.Subcommands[first] || !looksLikeSubcommand(first) {
		return commandWarning{}, false
	}
	return commandArgWarning(spec.Name, fmt.Sprintf("does not list %q as a command in its live help output", first)), true
}

func validatePathOperands(spec commandSpec, args []string) (commandWarning, bool) {
	if !spec.PathOnlyOperands {
		return commandWarning{}, false
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "$") || strings.ContainsAny(arg, "*?[") {
			continue
		}
		if _, err := os.Stat(arg); os.IsNotExist(err) {
			return commandArgWarning(spec.Name, fmt.Sprintf("path operand does not exist: %s", arg)), true
		}
	}
	return commandWarning{}, false
}

func commandArgWarning(name, message string) commandWarning {
	return commandWarning{
		Code:    "command-args",
		Message: fmt.Sprintf("%s %s", name, message),
	}
}
