package cmd

import (
	"fmt"

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

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Show tips in the interactive prompt?").
					Affirmative("Show tips").
					Negative("Hide tips").
					WithButtonAlignment(lipgloss.Left).
					Value(&choices.ShowTips),
				huh.NewConfirm().
					Title("Reset ignored tips?").
					Description(fmt.Sprintf("Currently ignored: %d", len(cfg.DismissedHints))).
					Affirmative("Reset").
					Negative("Keep").
					WithButtonAlignment(lipgloss.Left).
					Value(&choices.ResetIgnoredTips),
				huh.NewConfirm().
					Title("Enable delayed command checking?").
					Description("Warn about likely command mistakes before the timer starts.").
					Affirmative("Enable").
					Negative("Disable").
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
