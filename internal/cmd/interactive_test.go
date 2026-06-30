package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInteractiveCommandValidationWarnsWhileTyping(t *testing.T) {
	tmp := t.TempDir()
	fake := filepath.Join(tmp, "devtool")
	contents := "#!/bin/sh\nif [ \"$1\" = \"--help\" ]; then\n  printf '%s\n' 'Usage: devtool [OPTIONS]' 'Options:' '  -a, --all'\n  exit 0\nfi\n"
	if err := os.WriteFile(fake, []byte(contents), 0o755); err != nil {
		t.Fatalf("write fake devtool: %v", err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

	m := initialModel("")
	m.focusIndex = 1
	m.commandInput.SetValue("devtool -z")
	m.commandLastKeyPressTime = time.Now().Add(-time.Second)

	updated, _ := m.Update(debounceTickMsg(time.Now()))
	model := updated.(*model)
	if !model.commandValidated {
		t.Fatal("expected command validation to run")
	}
	if len(model.commandWarnings) == 0 {
		t.Fatal("expected command warning")
	}
	if view := model.View(); !strings.Contains(view, "devtool does not list \"-z\" as an option") {
		t.Fatalf("expected warning in view, got %q", view)
	}
}

func TestInteractiveCommandValidationNoWarningForValidCommand(t *testing.T) {
	m := initialModel("")
	m.focusIndex = 1
	m.commandInput.SetValue("echo hello")
	m.commandLastKeyPressTime = time.Now().Add(-time.Second)

	updated, _ := m.Update(debounceTickMsg(time.Now()))
	model := updated.(*model)
	if !model.commandValidated {
		t.Fatal("expected command validation to run")
	}
	if len(model.commandWarnings) != 0 {
		t.Fatalf("expected no command warnings, got %#v", model.commandWarnings)
	}
	if view := model.View(); strings.Contains(view, "⚠") || strings.Contains(view, "checking command") {
		t.Fatalf("expected no command warning/checking state in view, got %q", view)
	}
}
