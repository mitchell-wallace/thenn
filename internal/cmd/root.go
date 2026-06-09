package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/mitchell-wallace/thenn/internal/timer"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	version     string
	quietFlag   bool
	jsonOutput  bool
	commandFlag string
)

var durationArgRegex = regexp.MustCompile(`^\d+[a-zA-Z]+$`)

func init() {
	rootCmd.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false, "disable countdown visual output")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json-output", false, "emit structured JSON output")
	rootCmd.Flags().StringVarP(&commandFlag, "command", "c", "", "command to execute when the countdown finishes")
}

var rootCmd = &cobra.Command{
	Use:   "thenn [duration] [command...]",
	Short: "thenn delays command execution with a visible countdown",
	Long: `thenn is a command-line tool that delays the start of a command with a visible countdown.
It displays a single-line countdown showing the remaining duration and the 12-hour target time.
Pressing the spacebar while running will pause the countdown, freezing the duration and delaying the end time.`,
	Args:          cobra.ArbitraryArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Start update check in background
		updateNoticeChan := make(chan string, 1)
		if version != "" && version != "dev" {
			go func() {
				client := &http.Client{Timeout: 2 * time.Second}
				req, err := http.NewRequest("GET", "https://api.github.com/repos/mitchell-wallace/thenn/releases/latest", nil)
				if err != nil {
					return
				}
				req.Header.Set("Accept", "application/vnd.github+json")
				resp, err := client.Do(req)
				if err != nil {
					return
				}
				defer func() { _ = resp.Body.Close() }()
				if resp.StatusCode != http.StatusOK {
					return
				}
				var payload struct {
					TagName string `json:"tag_name"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil {
					latest := strings.TrimPrefix(payload.TagName, "v")
					cmp, err := compareVersions(version, latest)
					if err == nil && cmp < 0 {
						updateNoticeChan <- fmt.Sprintf("\n✨ A new version of thenn is available: v%s (current: v%s). Run 'thenn update' to update.", latest, version)
					}
				}
			}()
		}

		var durationParts []string
		var commandPart []string

		if len(args) == 0 {
			if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
				return fmt.Errorf("a duration must be specified (e.g. 10s, 5m, 2h)")
			}

			durationInput, cmdPart, err := runInteractive(commandFlag)
			if err != nil {
				if errors.Is(err, timer.ErrInterrupted) {
					if jsonOutput {
						exit(130, "interrupted")
					} else {
						panic(&exitError{code: 130})
					}
				}
				return err
			}
			durationParts = []string{durationInput}
			commandPart = cmdPart
		} else {
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
				n := len(rawCmdParts)
				if n > 0 && len(args) >= n {
					durationParts = args[:len(args)-n]
					commandPart = args[len(args)-n:]
				} else {
					durationParts = args
				}
			} else {
				// No "--" separator, scan positional args to separate duration from command
				// The first argument is ALWAYS part of the duration.
				durationParts = append(durationParts, args[0])
				for i := 1; i < len(args); i++ {
					arg := args[i]
					isDur := false
					if _, err := timer.ParseDurationOrTarget(arg, time.Now()); err == nil {
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

			if cmd.Flags().Changed("command") {
				if len(commandPart) > 0 {
					return fmt.Errorf("cannot specify both -c/--command and positional command arguments")
				}
				var shell string
				var shellArgs []string
				if runtime.GOOS == "windows" {
					shell = os.Getenv("COMSPEC")
					if shell == "" {
						shell = "cmd.exe"
					}
					shellArgs = []string{"/c", commandFlag}
				} else {
					shell = os.Getenv("SHELL")
					if shell == "" {
						shell = "sh"
					}
					shellArgs = []string{"-c", commandFlag}
				}
				commandPart = append([]string{shell}, shellArgs...)
			}
		}

		durationStr := strings.Join(durationParts, " ")
		d, err := timer.ParseDurationOrTarget(durationStr, time.Now())
		if err != nil {
			return fmt.Errorf("invalid duration: %w", err)
		}

		runner := timer.NewRunner(d, commandPart, quietFlag)
		err = runner.Run()

		// Print update notice if one was detected
		select {
		case notice := <-updateNoticeChan:
			fmt.Fprintln(os.Stderr, notice)
		default:
		}

		if err != nil {
			if errors.Is(err, timer.ErrInterrupted) {
				if jsonOutput {
					exit(130, "interrupted")
				} else {
					panic(&exitError{code: 130})
				}
			}
			exitCode := timer.ExtractExitCode(err)
			exit(exitCode, "command failed: %v", err)
		}
		return nil
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

	err := rootCmd.Execute()
	if err != nil {
		exit(1, "%v", err)
	}
	return nil
}

func printJSON(v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		exit(2, "json marshal: %v", err)
	}
	fmt.Println(string(b))
}
