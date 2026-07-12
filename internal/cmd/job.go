package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/mitchell-wallace/thenn/internal/job"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const defaultJobLogLines = 80

var jobCmd = &cobra.Command{
	Use:   "job",
	Short: "Manage scheduled jobs",
	Long: `Manage scriptable scheduled jobs backed by user-level systemd timers.

Requires Linux with systemctl installed and a reachable systemd user service manager.

Run "thenn job syntax" for creation examples.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) {
			return runJobTUI(cmd)
		}
		return cmd.Help()
	},
}

func init() {
	jobCmd.AddCommand(
		newJobCreateCmd("every", "every <duration> [until <date-or-time>] --label <label> -- <command...>"),
		newJobCreateCmd("daily", "daily at <time> [until <date-or-time>] --label <label> -- <command...>"),
		newJobCreateCmd("weekdays", "weekdays at <time> [until <date-or-time>] --label <label> -- <command...>"),
		newJobCreateCmd("weekly", "weekly <weekday> at <time> [until <date-or-time>] --label <label> -- <command...>"),
		newJobCreateCmd("once", "once at <time-or-date-or-date-time> --label <label> -- <command...>"),
		jobListCmd,
		jobShowCmd,
		jobLogsCmd,
		jobPauseCmd,
		jobResumeCmd,
		jobRemoveCmd,
		jobRunCmd,
		jobExecCmd,
		jobSyntaxCmd,
	)
	rootCmd.AddCommand(jobCmd)
}

func newJobCreateCmd(verb, use string) *cobra.Command {
	var label string
	cmd := &cobra.Command{
		Use:   use,
		Short: "Create a scheduled job",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return createJob(cmd, verb, label, args)
		},
	}
	cmd.Flags().StringVar(&label, "label", "", "job label")
	_ = cmd.MarkFlagRequired("label")
	return cmd
}

func createJob(cmd *cobra.Command, verb, label string, args []string) error {
	scheduleArgs, commandArgv, err := splitJobCreateArgs(cmd, verb, args)
	if err != nil {
		return err
	}
	store, backend, err := newAvailableJobStoreAndBackend(cmd.Context())
	if err != nil {
		return err
	}
	if err := validateJobCommand(commandArgv); err != nil {
		return err
	}
	if _, _, ok := shellCommand(commandArgv); !ok {
		commandArgv = expandCommandAlias(commandArgv)
	}
	if commandCheckingEnabled() {
		emitCommandWarnings(checkCommand(commandArgv))
	}

	now := time.Now()
	schedule, err := job.ParseSchedule(scheduleArgs, job.WithNow(now))
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	metadata, err := job.NewMetadata(label, strings.Join(scheduleArgs, " "), schedule, commandArgv, cwd, now)
	if err != nil {
		return err
	}
	if _, err := store.Load(metadata.Label); err == nil {
		return fmt.Errorf("job %q already exists; remove it first or choose a different --label", metadata.Label)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := store.Save(metadata); err != nil {
		return err
	}
	if err := backend.Install(cmd.Context(), metadata); err != nil {
		_ = store.Delete(metadata.Label)
		return err
	}
	if err := backend.EnableNow(cmd.Context(), metadata.Label); err != nil {
		rollbackJobCreate(cmd.Context(), store, backend, metadata.Label)
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "created job %s\n", metadata.Label)
	return nil
}

func splitJobCreateArgs(cmd *cobra.Command, verb string, args []string) ([]string, []string, error) {
	dashAt := cmd.Flags().ArgsLenAtDash()
	if dashAt < 0 {
		return nil, nil, fmt.Errorf("job creation requires -- before the command")
	}
	if dashAt > len(args) {
		return nil, nil, fmt.Errorf("invalid -- position")
	}
	scheduleArgs := append([]string{verb}, args[:dashAt]...)
	commandArgv := append([]string(nil), args[dashAt:]...)
	return scheduleArgs, commandArgv, nil
}

func validateJobCommand(argv []string) error {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return fmt.Errorf("command is required after --")
	}
	for _, arg := range argv {
		if strings.ContainsRune(arg, '\x00') {
			return fmt.Errorf("command arguments cannot contain NUL bytes")
		}
	}
	return nil
}

var jobListCmd = &cobra.Command{
	Use:   "list",
	Short: "List scheduled jobs",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, _, err := newAvailableJobStoreAndBackend(cmd.Context())
		if err != nil {
			return err
		}
		jobs, err := store.List()
		if err != nil {
			return err
		}
		if len(jobs) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No jobs.")
			return nil
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "LABEL\tSCHEDULE\tCOMMAND")
		for _, metadata := range jobs {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", metadata.Label, metadata.OriginalPhrase, formatJobCommand(metadata.CommandArgv))
		}
		return nil
	},
}

var jobShowCmd = &cobra.Command{
	Use:   "show <label>",
	Short: "Show job metadata and timer status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		metadata, backend, err := loadAvailableJob(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		printJobMetadata(cmd, &metadata)
		status, err := backend.Status(cmd.Context(), metadata.Label)
		if err != nil {
			if status != "" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Timer status:")
				_, _ = fmt.Fprint(cmd.OutOrStdout(), status)
				if !strings.HasSuffix(status, "\n") {
					_, _ = fmt.Fprintln(cmd.OutOrStdout())
				}
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Timer status unavailable: %v\n", err)
			return nil
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Timer status:")
		_, _ = fmt.Fprint(cmd.OutOrStdout(), status)
		if status != "" && !strings.HasSuffix(status, "\n") {
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
		}
		return nil
	},
}

var jobLogsCmd = &cobra.Command{
	Use:   "logs <label>",
	Short: "Show recent job logs",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		metadata, backend, err := loadAvailableJob(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		logs, err := backend.Journal(cmd.Context(), metadata.Label, defaultJobLogLines)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(cmd.OutOrStdout(), logs)
		if logs != "" && !strings.HasSuffix(logs, "\n") {
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
		}
		return nil
	},
}

var jobPauseCmd = &cobra.Command{
	Use:   "pause <label>",
	Short: "Disable and stop a job timer",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		metadata, backend, err := loadAvailableJob(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if err := backend.DisableNow(cmd.Context(), metadata.Label); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "paused job %s\n", metadata.Label)
		return nil
	},
}

var jobResumeCmd = &cobra.Command{
	Use:   "resume <label>",
	Short: "Enable and start a job timer",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		metadata, backend, err := loadAvailableJob(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if err := backend.EnableNow(cmd.Context(), metadata.Label); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "resumed job %s\n", metadata.Label)
		return nil
	},
}

var jobRemoveCmd = &cobra.Command{
	Use:   "remove <label>",
	Short: "Remove a scheduled job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, backend, metadata, err := loadAvailableJobWithStore(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if err := backend.Remove(cmd.Context(), metadata.Label); err != nil {
			return err
		}
		if err := store.Delete(metadata.Label); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "removed job %s\n", metadata.Label)
		return nil
	},
}

func rollbackJobCreate(ctx context.Context, store *job.Store, backend job.Backend, label string) {
	_ = backend.RollbackInstall(ctx, label)
	_ = store.Delete(label)
}

var jobRunCmd = &cobra.Command{
	Use:   "run <label>",
	Short: "Start a job service now",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		metadata, backend, err := loadAvailableJob(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if err := backend.StartService(cmd.Context(), metadata.Label); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "started job %s\n", metadata.Label)
		return nil
	},
}

var jobExecCmd = &cobra.Command{
	Use:    "exec <label>",
	Short:  "Execute a scheduled job command",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return execJob(cmd.Context(), args[0])
	},
}

var jobSyntaxCmd = &cobra.Command{
	Use:   "syntax",
	Short: "Show job syntax",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		_, _ = fmt.Fprint(cmd.OutOrStdout(), `Job creation syntax:
  thenn job every <duration> [until <date-or-time>] --label <label> -- <command...>
  thenn job daily at <time> [until <date-or-time>] --label <label> -- <command...>
  thenn job weekdays at <time> [until <date-or-time>] --label <label> -- <command...>
  thenn job weekly <weekday> at <time> [until <date-or-time>] --label <label> -- <command...>
  thenn job once at <time-or-date-or-date-time> --label <label> -- <command...>

Examples:
  thenn job every 15m --label check-mail -- mbsync -a
  thenn job every 3h until 2026-07-23 --label check-api -- curl https://example.com
  thenn job daily at 9pm --label backup -- restic backup ~/Documents
  thenn job weekdays at 08:30 --label standup -- ./standup.sh
  thenn job weekly monday at 08:30 --label report -- ./report.sh
  thenn job once at 2026-07-23 21:00 --label migration -- ./migrate.sh

Dates:
  Use year-first dates to avoid ambiguity: 2026-07-23, 2026/07/23, or 2026.07.23

Times:
  Use 9pm, 9:30pm, 21:00, or 09:30

Timer policy:
  "every" waits the full duration after the prior command finishes. Suspend,
  shutdown, and user-manager downtime do not count toward that duration.
  Calendar schedules catch up once after downtime. Jobs never overlap
  themselves, add no randomized delay, and do not wake suspended hardware.
`)
	},
}

func newJobStoreAndBackend() (*job.Store, job.Backend, error) {
	store, err := job.NewStore()
	if err != nil {
		return nil, nil, err
	}
	backend, err := job.NewSystemdBackend()
	if err != nil {
		return nil, nil, err
	}
	return store, backend, nil
}

func newAvailableJobStoreAndBackend(ctx context.Context) (*job.Store, job.Backend, error) {
	store, backend, err := newJobStoreAndBackend()
	if err != nil {
		return nil, nil, err
	}
	if err := backend.CheckAvailable(ctx); err != nil {
		return nil, nil, err
	}
	return store, backend, nil
}

func loadAvailableJob(ctx context.Context, label string) (job.Metadata, job.Backend, error) {
	_, backend, metadata, err := loadAvailableJobWithStore(ctx, label)
	return metadata, backend, err
}

func loadAvailableJobWithStore(ctx context.Context, label string) (*job.Store, job.Backend, job.Metadata, error) {
	store, backend, err := newAvailableJobStoreAndBackend(ctx)
	if err != nil {
		return nil, nil, job.Metadata{}, err
	}
	metadata, err := store.Load(label)
	if err != nil {
		return nil, nil, job.Metadata{}, fmt.Errorf("load job %q: %w", label, err)
	}
	return store, backend, metadata, nil
}

func printJobMetadata(cmd *cobra.Command, metadata *job.Metadata) {
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Label: %s\n", metadata.Label)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Schedule: %s\n", metadata.OriginalPhrase)
	if metadata.ParsedSchedule.OnCalendar != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "OnCalendar: %s\n", metadata.ParsedSchedule.OnCalendar)
	}
	if metadata.ParsedSchedule.OnUnitActiveSec != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "OnUnitInactiveSec: %s\n", metadata.ParsedSchedule.OnUnitActiveSec)
	}
	if metadata.ParsedSchedule.Until != nil {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Until: %s\n", metadata.ParsedSchedule.Until.Format(time.RFC3339))
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Command: %s\n", formatJobCommand(metadata.CommandArgv))
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "CWD: %s\n", metadata.CWD)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Created: %s\n", metadata.CreatedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Updated: %s\n", metadata.UpdatedAt.Format(time.RFC3339))
}

func formatJobCommand(argv []string) string {
	return strings.Join(argv, " ")
}

func execJob(ctx context.Context, label string) error {
	// This path is invoked by the service unit itself. Do not block the stored
	// command if the user bus becomes transiently unavailable after activation.
	store, backend, err := newJobStoreAndBackend()
	if err != nil {
		return err
	}
	metadata, err := store.Load(label)
	if err != nil {
		return fmt.Errorf("load job %q: %w", label, err)
	}
	if metadata.ParsedSchedule.Until != nil && !metadata.ParsedSchedule.Until.After(time.Now()) {
		return backend.DisableNow(ctx, metadata.Label)
	}

	cmd := exec.CommandContext(ctx, metadata.CommandArgv[0], metadata.CommandArgv[1:]...)
	cmd.Dir = metadata.CWD
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	runErr := cmd.Run()

	var disableErr error
	if metadata.ParsedSchedule.Kind == job.ScheduleOnce {
		disableErr = backend.DisableNow(ctx, metadata.Label)
	}
	if runErr != nil {
		if disableErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "thenn: failed to disable job timer: %v\n", disableErr)
		}
		return returnChildExitStatus(runErr)
	}
	return disableErr
}

func returnChildExitStatus(err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		if code < 0 {
			code = 1
		}
		panic(&exitError{code: code})
	}
	return err
}
