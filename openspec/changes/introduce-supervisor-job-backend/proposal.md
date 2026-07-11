# Proposal: Introduce a thenn-owned supervisor job backend

## Intent

`thenn job` is currently unusable anywhere user-level systemd is absent — hardened containers, minimal images, WSL without a user bus, and every non-Linux platform. The user-facing promise is "resilient user jobs," but the implementation is a thin shell over `systemctl --user`, and the repo's own verification notes show every public job command exiting 1 in a container. thenn should ship its own persistent background service (a per-user supervisor daemon) that fulfils the existing job contract when systemd is unavailable, while systemd remains the preferred backend where it is reachable.

## Evidence

- `internal/job/systemd.go:13-19` — `ErrUnsupported`, `ErrSystemctlNotFound`, `ErrUserSystemdUnavailable`: three hard-failure modes with no fallback.
- `internal/cmd/job.go:346-367` — `newJobStoreAndBackend`/`newAvailableJobStoreAndBackend` return the concrete `*job.SystemdBackend`; there is no backend interface, so today a second backend cannot even be plugged in.
- `internal/cmd/job_tui.go:44-68` — the TUI model also holds `backend *job.SystemdBackend` and refuses to start when `CheckAvailable` fails.
- `internal/job/systemd_unsupported.go` — every operation on non-Linux is a stub returning `ErrUnsupported`.
- `tmp/job-verification-2026-07-10.md` — verification in this repo's own container: all eleven public job commands exit 1 with "systemctl was not found in PATH".
- `README.md` ("Headless Jobs on Linux") — "Minimal containers, WSL environments, and headless sessions without a systemd user bus are not supported unless a user manager has been configured."
- `internal/cmd/job.go:337-342` (`job syntax` output) — the documented timer policy (fixed delay after completion, no self-overlap, one calendar catch-up, no hardware wake) is a backend-independent contract the supervisor must honor.
- `internal/job/store.go` — job definitions already live in backend-neutral JSON metadata under the user config dir; the store needs no change to feed a second backend.

## Scope

In scope:

- A `job.Backend` interface extracted from the operation surface `internal/cmd` actually uses (`CheckAvailable`, `Install`, `EnableNow`, `DisableNow`, `StartService`, `Remove`, `RollbackInstall`, `Status`, `Journal`), with `SystemdBackend` as the first implementation.
- A new `SupervisorBackend` plus a per-user `thenn job daemon` process: single-instance scheduler, per-job runtime state, per-job log files, detach-from-terminal survival semantics.
- Backend selection: systemd when reachable, supervisor otherwise, with a config override; each job records its owning backend in metadata so mixed fleets stay consistent.
- CLI/TUI wording updates where output is currently systemd-specific (e.g. `job_tui.go` "Systemd:" detail lines, error strings in `internal/job/systemd.go`).
- Linux-first delivery; the supervisor core must build on all unix platforms even if only Linux is documented initially.

Out of scope:

- Windows job support (daemonization model differs; unlocked later by this seam).
- macOS launchd integration and boot-time autostart of the supervisor (documented limitation; see design).
- Changing schedule grammar, metadata format migrations beyond one additive field, or the systemd unit rendering.

## Proposed path

1. Extract the backend seam behind an interface without behavior change, with characterization tests over the existing fake-systemctl suite (`internal/job/systemd_test.go`, `systemd_linux_test.go`).
2. Build the supervisor scheduler as a pure, clock-injected core (`internal/job/supervisor`) that consumes the existing `Metadata`/`Schedule` model and enforces the documented timer policy: fixed-delay `every`, single calendar catch-up via persisted stamps, strict non-overlap, disable-after-`until`/`once` (reusing `thenn job exec` semantics).
3. Wrap the core in a daemon process (`thenn job daemon run`, hidden) with flock-based single-instance guarantee, lazy auto-spawn from job commands, file-based desired state and run-now requests, and per-job logs with rotation.
4. Wire selection (`auto` → systemd preferred, supervisor fallback), a `job_backend` config key in the existing `UserConfig`, and a `Backend` field on `Metadata`.
5. Verify end-to-end inside plain CI containers — something the systemd backend can never offer (see `stabilize-systemd-job-verification` for the systemd side).

## Expected payoff

- `thenn job` works in hardened containers, minimal images, and WSL — the environments agents actually run in, and the environment this repo itself is developed in.
- The job feature becomes testable in ordinary CI containers end-to-end, ending the current state where the core product path is unverifiable at PR time.
- The backend seam unlocks macOS/Windows job support later without touching command code.
- systemd users lose nothing: reachable user managers keep the current battle-tested path.

## Risks and unknowns

- Scheduler correctness (clock jumps, suspend, crash mid-run) is now thenn's problem instead of systemd's. De-risked by a pure clock-injected core with table-driven tests, persisted last-start/next-due stamps, and by keeping systemd preferred where present.
- Duplicate execution if a job could be claimed by both backends. De-risked by recording the owning backend in job metadata at creation; operations always route to the recorded owner.
- Daemon liveness across reboots without systemd is unsolvable in general; the design makes the limitation explicit (lazy respawn on next thenn job invocation, `job daemon status` visibility) rather than pretending.
- Single-instance races. De-risked with an OS-level `flock` on a lock file, not PID-file heuristics alone.

## Spec impact

Behavior delta in the job-scheduling domain: job commands SHALL work without user-level systemd via the supervisor backend; survival, logging, and status semantics for the supervisor are new observable contracts. Delta spec included under `specs/job-scheduling/spec.md`.
