package cmd

import "testing"

func TestApplyConfigChoices(t *testing.T) {
	cfg := UserConfig{
		AlwaysHideHints:        true,
		DismissedHints:         []string{"one", "two"},
		DisableCommandChecking: true,
	}

	cfg = applyConfigChoices(cfg, configChoices{
		ShowTips:              true,
		ResetIgnoredTips:      true,
		EnableCommandChecking: true,
	})

	if cfg.AlwaysHideHints {
		t.Fatal("expected tips to be enabled")
	}
	if len(cfg.DismissedHints) != 0 {
		t.Fatalf("expected dismissed tips to be reset, got %#v", cfg.DismissedHints)
	}
	if cfg.DisableCommandChecking {
		t.Fatal("expected command checking to be enabled")
	}
}

func TestApplyConfigChoicesKeepsIgnoredTipsWhenNotReset(t *testing.T) {
	cfg := UserConfig{DismissedHints: []string{"one"}}

	cfg = applyConfigChoices(cfg, configChoices{
		ShowTips:              false,
		ResetIgnoredTips:      false,
		EnableCommandChecking: false,
	})

	if !cfg.AlwaysHideHints {
		t.Fatal("expected tips to be disabled")
	}
	if len(cfg.DismissedHints) != 1 || cfg.DismissedHints[0] != "one" {
		t.Fatalf("expected dismissed tips to be kept, got %#v", cfg.DismissedHints)
	}
	if !cfg.DisableCommandChecking {
		t.Fatal("expected command checking to be disabled")
	}
}

func TestPaddedButtonLabels(t *testing.T) {
	width := maxLabelWidth("Show tips", "Hide tips", "Reset", "Keep", "Enable", "Disable")
	if width != len("Show tips") {
		t.Fatalf("unexpected max width: %d", width)
	}

	a, b := paddedButtonLabels(width, "Reset", "Keep")
	if len(a) != len(b) {
		t.Fatalf("expected equal widths, got %q (%d) and %q (%d)", a, len(a), b, len(b))
	}
	if len(a) != width || len(b) != width {
		t.Fatalf("expected labels to use shared width %d, got %q (%d) and %q (%d)", width, a, len(a), b, len(b))
	}
	if a != "  Reset  " || b != "  Keep   " {
		t.Fatalf("unexpected padding: %q %q", a, b)
	}

	a, b = paddedButtonLabels(width, "Show tips", "Hide tips")
	if len(a) != len(b) {
		t.Fatalf("expected equal widths, got %q (%d) and %q (%d)", a, len(a), b, len(b))
	}
	if a != "Show tips" || b != "Hide tips" {
		t.Fatalf("unexpected padding: %q %q", a, b)
	}
}
