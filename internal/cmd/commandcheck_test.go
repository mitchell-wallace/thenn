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
