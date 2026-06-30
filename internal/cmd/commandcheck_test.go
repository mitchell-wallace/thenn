package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckCommand_ShellSyntaxWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell -n validation is Unix-only")
	}

	warnings := checkCommand([]string{"sh", "-c", "if true; then"})
	if len(warnings) == 0 {
		t.Fatal("expected shell syntax warning")
	}
	if warnings[0].Code != "shell-syntax" {
		t.Fatalf("expected shell-syntax warning, got %#v", warnings[0])
	}
}

func TestCheckCommand_ShellMissingExecutableWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell validation is Unix-only")
	}

	warnings := checkCommand([]string{"sh", "-c", "thenn-definitely-missing-command"})
	if len(warnings) == 0 {
		t.Fatal("expected missing executable warning")
	}
	if warnings[0].Code != "command-not-found" {
		t.Fatalf("expected command-not-found warning, got %#v", warnings[0])
	}
}

func TestCheckCommand_DirectMissingExecutableWarning(t *testing.T) {
	warnings := checkCommand([]string{"thenn-definitely-missing-command"})
	if len(warnings) == 0 {
		t.Fatal("expected missing executable warning")
	}
	if warnings[0].Code != "command-not-found" {
		t.Fatalf("expected command-not-found warning, got %#v", warnings[0])
	}
}

func TestCheckCommand_GenericUnknownOptionWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper script is Unix-only")
	}

	tmp := t.TempDir()
	fake := filepath.Join(tmp, "devtool")
	contents := "#!/bin/sh\nif [ \"$1\" = \"--help\" ]; then\n  printf '%s\n' 'Usage: devtool [OPTIONS] [path]' 'Options:' '  -a, --all' '  -q, --quiet'\n  exit 0\nfi\n"
	if err := os.WriteFile(fake, []byte(contents), 0o755); err != nil {
		t.Fatalf("write fake devtool: %v", err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

	warnings := checkCommand([]string{"devtool", "-z"})
	if len(warnings) == 0 {
		t.Fatal("expected unknown option warning")
	}
	if warnings[0].Code != "command-args" || !strings.Contains(warnings[0].Message, "-z") {
		t.Fatalf("expected command-args warning for -z, got %#v", warnings[0])
	}
}

func TestCheckCommand_GenericSubcommandWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper script is Unix-only")
	}

	tmp := t.TempDir()
	fake := filepath.Join(tmp, "devcmd")
	contents := "#!/bin/sh\nif [ \"$1\" = \"--help\" ]; then\n  printf '%s\n' 'Usage: devcmd <command> [args]' 'Commands:' '  build  Build project' '  test   Test project'\n  exit 0\nfi\n"
	if err := os.WriteFile(fake, []byte(contents), 0o755); err != nil {
		t.Fatalf("write fake devcmd: %v", err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

	warnings := checkCommand([]string{"devcmd", "buld"})
	if len(warnings) == 0 {
		t.Fatal("expected subcommand warning")
	}
	if warnings[0].Code != "command-args" || !strings.Contains(warnings[0].Message, "buld") {
		t.Fatalf("expected command-args warning for buld, got %#v", warnings[0])
	}
}

func TestCheckCommand_GitProviderSubcommandWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper script is Unix-only")
	}

	tmp := t.TempDir()
	fake := filepath.Join(tmp, "git")
	contents := "#!/bin/sh\nif [ \"$1\" = \"--help\" ]; then\n  printf '%s\n' 'usage: git [-C <path>] <command> [<args>]'\n  exit 0\nfi\nif [ \"$1\" = \"help\" ] && [ \"$2\" = \"-a\" ]; then\n  printf '%s\n' 'Main Commands' '   status   Show status' '   worktree Manage worktrees'\n  exit 0\nfi\n"
	if err := os.WriteFile(fake, []byte(contents), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

	warnings := checkCommand([]string{"git", "worktrees", "list"})
	if len(warnings) == 0 {
		t.Fatal("expected git subcommand warning")
	}
	if warnings[0].Code != "command-args" || !strings.Contains(warnings[0].Message, "worktrees") {
		t.Fatalf("expected command-args warning for worktrees, got %#v", warnings[0])
	}

	if warnings := checkCommand([]string{"git", "worktree", "list"}); len(warnings) != 0 {
		t.Fatalf("expected no warning for valid worktree subcommand, got %#v", warnings)
	}
}

func TestCheckCommand_GenericMissingPathOperandWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper script is Unix-only")
	}

	tmp := t.TempDir()
	fake := filepath.Join(tmp, "showfile")
	contents := "#!/bin/sh\nif [ \"$1\" = \"--help\" ]; then\n  printf '%s\n' 'Usage: showfile [OPTION]... [FILE]...' 'Options:' '  -n, --number'\n  exit 0\nfi\n"
	if err := os.WriteFile(fake, []byte(contents), 0o755); err != nil {
		t.Fatalf("write fake showfile: %v", err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

	warnings := checkCommand([]string{"showfile", "thenn-definitely-missing-file"})
	if len(warnings) == 0 {
		t.Fatal("expected missing path operand warning")
	}
	if warnings[0].Code != "command-args" || !strings.Contains(warnings[0].Message, "thenn-definitely-missing-file") {
		t.Fatalf("expected command-args warning for missing path, got %#v", warnings[0])
	}
}

func TestCheckCommand_AgentInvalidSubcommandWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper script is Unix-only")
	}

	tmp := t.TempDir()
	fakeCodex := filepath.Join(tmp, "codex")
	contents := "#!/bin/sh\nprintf '%s\n' 'Commands:' '  exec  Run Codex non-interactively' '  resume  Resume a session'\n"
	if err := os.WriteFile(fakeCodex, []byte(contents), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

	warnings := checkCommand([]string{"sh", "-c", "codex continue"})
	if len(warnings) == 0 {
		t.Fatal("expected agent CLI warning")
	}
	if warnings[0].Code != "agent-cli" || !strings.Contains(warnings[0].Message, "continue") {
		t.Fatalf("expected agent CLI warning for continue, got %#v", warnings[0])
	}
}

func TestCheckCommand_AgentResumeFlagWithPromptNoWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper script is Unix-only")
	}

	tmp := t.TempDir()
	fakeClaude := filepath.Join(tmp, "claude")
	contents := "#!/bin/sh\nprintf '%s\n' 'Commands:' '  agents  Manage background agents' '  mcp  Configure MCP servers'\n"
	if err := os.WriteFile(fakeClaude, []byte(contents), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

	warnings := checkCommand([]string{"sh", "-c", "claude -c continue"})
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings for resume flag with prompt, got %#v", warnings)
	}
}

func TestCheckCommand_ValidShellCommandNoWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell validation is Unix-only")
	}

	warnings := checkCommand([]string{"sh", "-c", "echo hello"})
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", warnings)
	}
}
