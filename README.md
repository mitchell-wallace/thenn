# thenn

`thenn` is a lightweight command-line utility designed for **ad-hoc command scheduling**. It delays the execution of a command with a visible, stylized single-line countdown, and can also manage resilient Linux user jobs through `systemd --user`.

If you hit a rate limit or need to delay execution, `thenn` is the perfect tool. For example:
> Hit a rate limit with codex? Run:
> ```bash
> thenn 3h && codex exec "fix my code"
> ```

---

## Features

- **Human-Friendly Durations**: Supports whitespace-separated durations like `10s`, `5m`, `2h 15m`, `1d 2h`, etc.
- **Spacebar Pausing**: Pressing the `Space` bar during the countdown will pause the timer, freezing the duration and dynamically shifting the end time forward. Press `Space` again to resume.
- **Dynamic Clock Target**: Displays the 12-hour local time when the countdown will finish (e.g. `2h 13m 55s -> 7:12p today`, or `tomorrow` or `YYYY.MM.DD` for other days).
- **Graceful Command Execution**: Directly routes standard I/O (stdin/stdout/stderr) and propagates the child process exit code transparently.
- **Warning-Only Command Preflight**: Checks delayed commands for shell syntax issues, optional ShellCheck findings, missing executables, and obvious invalid agent CLI subcommands before the timer starts. Warnings never block execution.
- **Shell-Resilient Raw Mode**: Automatically cleans up raw terminal state on interrupt (`Ctrl+C`) or exit.
- **Headless Job Manager for Linux**: Create labelled, recurring, systemd-backed user jobs without remembering systemd timer syntax.
- **Full-Screen Job TUI**: Run `thenn job` in a terminal to list, inspect, pause, resume, run, remove, and view syntax help for managed jobs.

---

## Installation

### Unix (Linux & macOS)

Run the following command to download and install to `$HOME/.local/bin`:

```bash
curl -fsSL https://raw.githubusercontent.com/mitchell-wallace/thenn/main/install.sh | bash
```

### Windows (PowerShell)

Run the following command in PowerShell:

```powershell
irm https://raw.githubusercontent.com/mitchell-wallace/thenn/main/install.ps1 | iex
```

---

## Command Reference

### Delaying and Chaining

To run a countdown and exit `0` when completed (standard chaining):
```bash
thenn <duration> && <command>
```

To execute a command directly through `thenn` (propagates exit status and inherits stdin):
```bash
thenn <duration> -- <command> [args...]
# or without double-dash:
thenn <duration> <command> [args...]
```

### Options & Subcommands

*   **`-q, --quiet`**: Disables the visible countdown output (runs silently).
    ```bash
    thenn 10m -q -- echo "Silent delay complete!"
    ```
*   **`-c, --command`**: Executes a command inside the default shell (`sh` on Unix, `cmd.exe` on Windows) when the countdown finishes.
    ```bash
    thenn 2s -c "echo 'Delayed output!'"
    ```
*   **Command checking config**: To disable warning-only command preflight, set `disable_command_checking` to `true` in the user config file at `$XDG_CONFIG_HOME/thenn/config.json` or the platform default config directory.
    ```json
    { "disable_command_checking": true }
    ```
*   **`config`**: Opens an interactive configuration form for toggling tips, resetting ignored tips, and toggling command checking.
    ```bash
    thenn config
    ```
*   **`job`**: Opens a full-screen job management TUI for `thenn`-managed systemd user jobs on Linux.
    ```bash
    thenn job
    ```
*   **`job syntax`**: Shows the supported job scheduling grammar.
    ```bash
    thenn job syntax
    ```
*   **`version`**: Prints the current version.
    ```bash
    thenn version
    ```
*   **`update`**: Checks the GitHub releases page for a newer version and updates in-place.
    ```bash
    thenn update
    ```

### Headless Jobs on Linux

`thenn job` requires Linux, the `systemctl` client, and a reachable `systemd --user` service manager. Minimal containers, WSL environments, and headless sessions without a systemd user bus are not supported unless a user manager has been configured. Jobs are labelled, stored under the user's `thenn` config directory, and installed as managed user units named like `thenn-job-<label>.timer` and `thenn-job-<label>.service`.

Create jobs with verb-first scheduling commands:

```bash
thenn job every 3h --label check-api -- curl https://example.com
thenn job every 1d until 2026-07-23 --label daily-review -- codex exec "review"
thenn job daily at 9pm --label backup -- restic backup ~/Documents
thenn job weekdays at 08:30 --label standup -- ./standup.sh
thenn job weekly monday at 10am --label report -- ./report.sh
thenn job once at 2026-07-23 21:00 --label migration -- ./migrate.sh
```

Manage jobs from the CLI:

```bash
thenn job list
thenn job show check-api
thenn job logs check-api
thenn job pause check-api
thenn job resume check-api
thenn job run check-api
thenn job remove check-api
```

Supported schedule forms:

```text
every <duration> [until <date-or-time>]
daily at <time> [until <date-or-time>]
weekdays at <time> [until <date-or-time>]
weekly <weekday> at <time> [until <date-or-time>]
once at <time-or-date-or-date-time>
```

Dates are year-first to avoid US/international ambiguity: `2026-07-23`, `2026/07/23`, or `2026.07.23`.

Times can be `9pm`, `9:30pm`, `21:00`, or `09:30`.

Notes:

*   `pause` disables and stops the timer; it does not kill a currently running service command.
*   `resume` enables and starts the timer again.
*   `run` asks systemd to start the service immediately.
*   `remove` stops both the timer and any currently running service command before deleting the job.
*   Logs come from `journalctl --user-unit thenn-job-<label>.service`.
*   On systems where user services should run while logged out, you may need to enable lingering with `loginctl enable-linger $USER`.

---

## Key Bindings (Interactive Countdown)

While the countdown is active:
*   `Space`: Toggle pause/resume (freezes/unfreezes remaining time).
*   `Ctrl+C`: Abort the countdown and exit with status `130`.
