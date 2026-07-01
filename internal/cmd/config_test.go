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
