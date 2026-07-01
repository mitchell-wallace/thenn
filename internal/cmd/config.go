package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

type configChoices struct {
	ShowTips              bool
	ResetIgnoredTips      bool
	EnableCommandChecking bool
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure thenn interactively",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfig()
		choices := configChoices{
			ShowTips:              !cfg.AlwaysHideHints,
			EnableCommandChecking: !cfg.DisableCommandChecking,
		}
		buttonWidth := maxLabelWidth("Show tips", "Hide tips", "Reset", "Keep", "Enable", "Disable")
		showTipsYes, showTipsNo := paddedButtonLabels(buttonWidth, "Show tips", "Hide tips")
		resetYes, resetNo := paddedButtonLabels(buttonWidth, "Reset", "Keep")
		enableYes, enableNo := paddedButtonLabels(buttonWidth, "Enable", "Disable")

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Show tips in the interactive prompt?").
					Affirmative(showTipsYes).
					Negative(showTipsNo).
					WithButtonAlignment(lipgloss.Left).
					Value(&choices.ShowTips),
				huh.NewConfirm().
					Title("Reset ignored tips?").
					Description(fmt.Sprintf("Currently ignored: %d", len(cfg.DismissedHints))).
					Affirmative(resetYes).
					Negative(resetNo).
					WithButtonAlignment(lipgloss.Left).
					Value(&choices.ResetIgnoredTips),
				huh.NewConfirm().
					Title("Enable delayed command checking?").
					Description("Warn about likely command mistakes before the timer starts.").
					Affirmative(enableYes).
					Negative(enableNo).
					WithButtonAlignment(lipgloss.Left).
					Value(&choices.EnableCommandChecking),
			),
		)
		if err := form.Run(); err != nil {
			return err
		}

		cfg = applyConfigChoices(cfg, choices)
		saveConfig(cfg)

		if !jsonOutput {
			fmt.Println("Configuration saved.")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
}

func applyConfigChoices(cfg UserConfig, choices configChoices) UserConfig {
	cfg.AlwaysHideHints = !choices.ShowTips
	cfg.DisableCommandChecking = !choices.EnableCommandChecking
	if choices.ResetIgnoredTips {
		cfg.DismissedHints = nil
	}
	return cfg
}

func paddedButtonLabels(width int, a, b string) (string, string) {
	return padButtonLabel(a, width), padButtonLabel(b, width)
}

func maxLabelWidth(labels ...string) int {
	var width int
	for _, label := range labels {
		width = max(width, len(label))
	}
	return width
}

func padButtonLabel(label string, width int) string {
	if len(label) >= width {
		return label
	}
	left := (width - len(label)) / 2
	right := width - len(label) - left
	return strings.Repeat(" ", left) + label + strings.Repeat(" ", right)
}
