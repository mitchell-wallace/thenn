package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInteractiveCommandValidationWarnsWhileTyping(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	tmp := t.TempDir()
	fake := filepath.Join(tmp, "devtool")
	contents := "#!/bin/sh\nif [ \"$1\" = \"--help\" ]; then\n  printf '%s\n' 'Usage: devtool [OPTIONS]' 'Options:' '  -a, --all'\n  exit 0\nfi\n"
	if err := os.WriteFile(fake, []byte(contents), 0o755); err != nil {
		t.Fatalf("write fake devtool: %v", err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

	m := initialModel("", "")
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
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := initialModel("", "")
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
	if view := model.View(); strings.Contains(view, "⚠") || strings.Contains(view, "checking command") || !strings.Contains(view, "No issues detected") {
		t.Fatalf("expected command success state in view, got %q", view)
	}
}

func TestInteractiveCommandValidationCanBeDisabledByConfig(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	configDir := filepath.Join(configHome, "thenn")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("make config dir: %v", err)
	}
	data, err := json.Marshal(UserConfig{DisableCommandChecking: true})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	m := initialModel("", "")
	m.focusIndex = 1
	m.commandInput.SetValue("missing-command")
	m.commandLastKeyPressTime = time.Now().Add(-time.Second)

	updated, _ := m.Update(debounceTickMsg(time.Now()))
	model := updated.(*model)
	if !model.commandValidated {
		t.Fatal("expected command validation to run")
	}
	if len(model.commandWarnings) != 0 {
		t.Fatalf("expected command warnings to be skipped, got %#v", model.commandWarnings)
	}
	if view := model.View(); !strings.Contains(view, "command checking disabled") {
		t.Fatalf("expected disabled state in view, got %q", view)
	}
}

func TestRenderHintTextStylesCommandSpans(t *testing.T) {
	view := renderHintText("Run `fd -z` after `thenn 1ms`")
	if strings.Contains(view, "`") {
		t.Fatalf("expected literal delimiters to be removed, got %q", view)
	}
	if !strings.Contains(view, "fd -z") || !strings.Contains(view, "thenn 1ms") {
		t.Fatalf("expected command text to remain, got %q", view)
	}
}

func TestStripHintMarkupPreservesDismissedTipCompatibility(t *testing.T) {
	marked := "Start a new Claude session: `claude \"Fix broken tests\"`; resume one: `claude -c`"
	legacy := "Start a new Claude session: claude \"Fix broken tests\"; resume one: claude -c"
	if got := stripHintMarkup(marked); got != legacy {
		t.Fatalf("expected legacy hint text %q, got %q", legacy, got)
	}
}
