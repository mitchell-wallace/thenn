# thenn

`thenn` is a lightweight command-line utility designed for **ad-hoc command scheduling**. It delays the execution of a command with a visible, stylized single-line countdown.

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
- **Shell-Resilient Raw Mode**: Automatically cleans up raw terminal state on interrupt (`Ctrl+C`) or exit.

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
*   **`version`**: Prints the current version.
    ```bash
    thenn version
    ```
*   **`update`**: Checks the GitHub releases page for a newer version and updates in-place.
    ```bash
    thenn update
    ```

---

## Key Bindings (Interactive Countdown)

While the countdown is active:
*   `Space`: Toggle pause/resume (freezes/unfreezes remaining time).
*   `Ctrl+C`: Abort the countdown and exit with status `130`.
