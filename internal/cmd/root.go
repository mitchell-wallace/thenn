package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/mitchell-wallace/thenn/internal/timer"
	"github.com/spf13/cobra"
)

var (
	version    string
	quietFlag  bool
	jsonOutput bool
)

var durationArgRegex = regexp.MustCompile(`^\d+[a-zA-Z]+$`)

func init() {
	rootCmd.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false, "disable countdown visual output")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json-output", false, "emit structured JSON output")
}

var rootCmd = &cobra.Command{
	Use:   "thenn [duration] [command...]",
	Short: "thenn delays command execution with a visible countdown",
	Long: `thenn is a command-line tool that delays the start of a command with a visible countdown.
It displays a single-line countdown showing the remaining duration and the 12-hour target time.
Pressing the spacebar while running will pause the countdown, freezing the duration and delaying the end time.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("a duration must be specified (e.g. 10s, 5m, 2h)")
		}

		var durationParts []string
		var commandPart []string

		// Detect if "--" separator was used in raw arguments
		dashDashIdx := -1
		for i, arg := range os.Args {
			if arg == "--" {
				dashDashIdx = i
				break
			}
		}

		if dashDashIdx != -1 {
			// Extract raw command elements after "--"
			rawCmdParts := os.Args[dashDashIdx+1:]
			if len(rawCmdParts) > 0 {
				// Find where the first command argument occurs in Cobra's positional args
				firstCmdArg := rawCmdParts[0]
				cmdStartIdx := -1
				for i, a := range args {
					if a == firstCmdArg {
						cmdStartIdx = i
						break
					}
				}
				if cmdStartIdx != -1 {
					durationParts = args[:cmdStartIdx]
					commandPart = args[cmdStartIdx:]
				} else {
					durationParts = args
				}
			} else {
				durationParts = args
			}
		} else {
			// No "--" separator, scan positional args to separate duration from command
			for i, arg := range args {
				isDur := false
				if _, err := timer.ParseDuration(arg); err == nil {
					isDur = true
				} else if durationArgRegex.MatchString(arg) {
					isDur = true
				}

				if isDur {
					durationParts = append(durationParts, arg)
				} else {
					commandPart = args[i:]
					break
				}
			}
		}

		durationStr := strings.Join(durationParts, " ")
		d, err := timer.ParseDuration(durationStr)
		if err != nil {
			return fmt.Errorf("invalid duration: %w", err)
		}

		runner := timer.NewRunner(d, commandPart, quietFlag)
		return runner.Run()
	},
}

type exitError struct {
	code int
}

func (e *exitError) Error() string { return fmt.Sprintf("exit %d", e.code) }

func exit(code int, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if jsonOutput {
		b, _ := json.Marshal(map[string]interface{}{
			"error":    msg,
			"exitCode": code,
		})
		fmt.Fprintln(os.Stderr, string(b))
	} else {
		fmt.Fprintf(os.Stderr, "thenn: %s\n", msg)
	}
	panic(&exitError{code: code})
}

// Execute is the main entry point for the command execution
func Execute(v string) error {
	version = v
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("{{.Version}}\n")
	defer func() {
		if r := recover(); r != nil {
			if ee, ok := r.(*exitError); ok {
				os.Exit(ee.code)
			}
			panic(r)
		}
	}()

	// Detect json output flag early for Cobra commands
	for _, a := range os.Args[1:] {
		if a == "--json-output" {
			jsonOutput = true
		}
	}

	// Intercept version flag before Cobra so we can output custom format if required
	for _, a := range os.Args[1:] {
		if a == "--version" || a == "-v" {
			if jsonOutput {
				printJSON(map[string]interface{}{"version": version})
			} else {
				fmt.Println(version)
			}
			return nil
		}
	}

	return rootCmd.Execute()
}

func printJSON(v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		exit(2, "json marshal: %v", err)
	}
	fmt.Println(string(b))
}
