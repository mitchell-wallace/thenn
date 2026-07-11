# Design: Introduce a thenn-owned supervisor job backend

## Current shape

- `internal/cmd/job.go` and `internal/cmd/job_tui.go` construct `(*job.Store, *job.SystemdBackend)` pairs via `newJobStoreAndBackend()`/`newAvailableJobStoreAndBackend()` (`internal/cmd/job.go:346-367`). All eleven job subcommands and the TUI hold the concrete `*job.SystemdBackend`.
- The operation surface `internal/cmd` uses is exactly: `CheckAvailable`, `Install`, `EnableNow`, `DisableNow`, `StartService`, `Remove`, `RollbackInstall`, `Status`, `Journal` (grep of `backend.` in `job.go`/`job_tui.go`). `StopService`, `Enable`, `Disable`, `Start`, `Stop`, `DaemonReload` are systemd-internal helpers.
- Job definitions are backend-neutral JSON (`internal/job/metadata.go`, `internal/job/store.go`) under `<UserConfigDir>/thenn/jobs/<label>.json`. The systemd backend renders those into unit files (`internal/job/systemd.go:RenderUnits`) and shells out to `systemctl --user`.
- Execution runs back through the binary: the service unit's `ExecStart=<thenn> job exec <label>` re-loads metadata, enforces `until`, runs the argv, and self-disables `once` jobs (`internal/cmd/job.go:408-441`).
- Documented timer policy (`job syntax`, `internal/cmd/job.go:337-342`): `every` waits the full interval after the prior run finishes; downtime does not count toward the interval; calendar schedules catch up once after downtime; no self-overlap; no randomized delay; no hardware wake.

## Target shape

### 1. Backend interface (the seam)

```go
// internal/job/backend.go
type Backend interface {
    Name() string // "systemd" | "supervisor"
    CheckAvailable(ctx context.Context) error
    Install(ctx context.Context, metadata Metadata) error
    EnableNow(ctx context.Context, label string) error
    DisableNow(ctx context.Context, label string) error
    StartService(ctx context.Context, label string) error
    Remove(ctx context.Context, label string) error
    RollbackInstall(ctx context.Context, label string) error
    Status(ctx context.Context, label string) (string, error)
    Journal(ctx context.Context, label string, lines int) (string, error)
}
```

`SystemdBackend` already satisfies this modulo `Name()`. `internal/cmd` switches to the interface; the TUI model field becomes `backend job.Backend`.

### 2. Backend selection and per-job ownership

- `Metadata` gains `Backend string \`json:"backend,omitempty"\``; empty is read as `"systemd"` (every pre-existing job was systemd-managed), so old metadata files stay valid with no migration step.
- `job.SelectBackend(ctx, cfg)` for creation: honor `cfg.JobBackend` (`"auto"` default, `"systemd"`, `"supervisor"`); in `auto`, probe `SystemdBackend.CheckAvailable` and fall back to the supervisor. The chosen backend name is stamped into the new job's metadata.
- `job.BackendFor(metadata)` for operations on existing jobs: route strictly by the recorded owner. This is the duplicate-execution guard — a job can never be claimed by both schedulers.
- `job list` continues to read only the store; `show`/TUI annotate each job with its backend name.

### 3. Supervisor state and store

Rooted at `os.UserStateDir()`-equivalent (`$XDG_STATE_HOME/thenn/supervisor`, falling back to `~/.local/state/thenn/supervisor`), deliberately separate from the config-dir metadata store:

```
supervisor/
  lock                 # flock target; daemon holds an exclusive lock while alive
  daemon.json          # pid, start time, thenn version (informational; lock is the truth)
  desired/<label>.json # {"enabled": bool, "updated_at": ...} written by CLI
  runs/<label>.json    # {"last_start", "last_exit_code", "last_finish", "next_due"} written by daemon
  run-now/<label>      # empty file: run-once request, consumed by daemon
  logs/<label>.log     # per-job stdout+stderr, size-rotated to <label>.log.1
```

All CLI→daemon communication is file-based plus a `SIGHUP` poke at the daemon PID; the daemon also rescans every few seconds as a fallback. No sockets: nothing to version, nothing to secure, and `Status`/`Journal` are pure file reads that work even while the daemon is down.

### 4. Scheduler core (pure, clock-injected)

`internal/job/supervisor` package. The core is a synchronous state machine driven by an injected clock (aligned with the existing `job.WithNow` pattern in `schedule.go`), fully unit-testable without processes or sleeping:

- `every`: next due = last **finish** + interval (matches `OnUnitInactiveSec` semantics). Monotonic where possible; wall-clock jumps backward never fire early, jumps forward fire at most one catch-up.
- Calendar kinds (`daily`, `weekdays`, `weekly`, `once`): compute next fire from `Schedule.OnCalendar`'s already-normalized forms. The parser only ever emits three shapes (`*-*-* HH:MM:SS`, `Mon..Fri *-*-* HH:MM:SS`, `DOW *-*-* HH:MM:SS`, plus absolute `once` datetimes — see `parseDaily`/`parseWeekdays`/`parseWeekly`/`parseOnce` in `internal/job/schedule.go`), so the supervisor needs a next-occurrence function for exactly those shapes, not a general OnCalendar engine. Stop condition: if a metadata file ever contains an OnCalendar string outside these shapes, the job is marked errored in `runs/<label>.json` rather than guessed at.
- Persistence/catch-up: `runs/<label>.json` records the last completed calendar fire; on daemon start, a calendar job whose slot passed while the daemon was down fires exactly once (mirrors `Persistent=true`). `every` jobs do not catch up (mirrors current monotonic triggers).
- Non-overlap: one goroutine per job, next dispatch computed only after the child exits. Run-now requests while running are dropped with a log line (systemd `start` on an active oneshot behaves comparably).
- Execution: the daemon invokes `<thenn binary> job exec <label>` as a child with stdout/stderr appended to the job log — reusing the exact `until`-check/`once`-disable path systemd uses, so business rules live in one place. `execJob`'s disable call routes through `BackendFor(metadata)` and therefore hits the supervisor's desired-state file, not systemctl.

### 5. Daemon lifecycle and survival semantics

- `thenn job daemon run` (hidden, like `job exec`): acquires the flock or exits 0 immediately ("already running"); double-forks/`setsid` via re-exec so it survives shell and SSH session exit; closes stdio onto `supervisor/daemon.log`.
- Lazy spawn: every supervisor-routed mutating command (`create`, `resume`, `run`, and TUI equivalents) calls `EnsureDaemon(ctx)` — try flock probe, spawn if absent, wait briefly for liveness. `job list`/`show`/`logs` never spawn.
- `thenn job daemon status` (visible): reports running/not-running, pid, job counts, and — critically — prints the reboot caveat.
- Reboot: without an init system there is no portable resurrection. Contract: the daemon revives on the next thenn job interaction, and calendar catch-up then covers missed fires (once). The README documents this as the survival delta versus systemd. Autostart hooks (shell profile, cron `@reboot`, launchd) are explicitly deferred.
- Shutdown: SIGTERM → stop dispatching, wait up to a grace period for running children, persist `runs/*`, release lock.

## Alternatives considered

- **Per-job detached runner processes (no daemon)**: spawn one long-sleeping process per job. Rejected: N processes to supervise, pause/resume becomes kill/respawn, non-overlap and run-now race the sleeping runner, and crash recovery logic is duplicated per process.
- **Cron backend as the fallback**: rejected as primary fallback — hardened containers usually lack a cron daemon too, and cron cannot express fixed-delay-after-completion, `until` limits, or non-overlap without exactly the wrapper state this design builds anyway.
- **Unix-socket RPC control plane**: rejected for v1 — a protocol to version and secure, needed only for run-now/reload, both of which file-drop + SIGHUP handle with at-least-once semantics and zero deps.
- **Making the supervisor the only backend (drop systemd)**: rejected — systemd gives free boot survival, lingering, and journald; the user direction explicitly keeps systemd preferred where present.

## Migration and rollout

1. Land the interface extraction alone (behavior-preserving; all existing tests must pass unchanged).
2. Land the supervisor core + daemon behind selection logic that still hard-prefers systemd; on systemd hosts nothing changes.
3. Existing metadata files (no `backend` field) are read as systemd-owned; no rewrite, no migration command.
4. Flip the container/WSL experience: `CheckAvailable` failure in `auto` mode now selects the supervisor instead of erroring; the old error strings remain for explicit `job_backend: systemd` config.
5. README/`job syntax`/TUI help updated last, in the same change, so docs never advertise an unlanded capability.

Rollback at any step is deleting the new package and re-pinning selection to systemd; the metadata format change is additive-only.

## Verification strategy

- `just test` / `just lint` gates (existing CI, `.github/workflows/ci.yml`).
- Scheduler core: table-driven tests with a fake clock covering every schedule kind, `until` expiry, catch-up-once, non-overlap, and clock jumps (pattern already established by `schedule_test.go`).
- Daemon: an E2E test that creates `every 1s` jobs against a temp `XDG_STATE_HOME`/`XDG_CONFIG_HOME`, observes at least two log-file appends, pauses, resumes, run-nows, and removes — runnable in the plain CI container, which the systemd path can never be. This becomes the first true end-to-end job test at PR time.
- Interface extraction: existing fake-systemctl suites (`systemd_test.go`, `systemd_linux_test.go`, `store_test.go`) pass unmodified.
- Manual: run the tmp/job-verification-2026-07-10.md command matrix in a systemctl-less container and expect success instead of eleven exit-1 failures.

## Dependencies and ordering

- Independent of, but synergistic with, `stabilize-systemd-job-verification` (that change proves the preferred backend; this one makes the fallback provable in stock CI).
- Unlocks (not included): macOS/Windows job support through the same `Backend` interface; supervisor autostart integrations.
