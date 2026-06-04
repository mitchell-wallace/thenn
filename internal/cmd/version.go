package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the thenn version",
	Run: func(cmd *cobra.Command, args []string) {
		if jsonOutput {
			printJSON(map[string]interface{}{"version": version})
		} else {
			fmt.Println(version)
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
