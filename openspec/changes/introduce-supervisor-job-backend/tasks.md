## 1. Extract the backend seam

- [ ] 1.1 Add the `job.Backend` interface for the command-facing operation surface and make `SystemdBackend` identify itself as `systemd`.
- [ ] 1.2 Change the job CLI and TUI to depend on `job.Backend` without changing backend construction or selection.
- [ ] 1.3 Run the existing test and lint gates and confirm `systemd_test.go`, `systemd_linux_test.go`, and `store_test.go` remain unmodified.

## 2. Build the supervisor scheduler core

- [ ] 2.1 Add a pure, clock-injected scheduler under `internal/job/supervisor` for fixed-delay `every` schedules and the normalized daily, weekdays, weekly, and once calendar shapes.
- [ ] 2.2 Add persisted runtime-state modeling for last start, last finish, next due, exit status, and scheduler errors.
- [ ] 2.3 Enforce until expiry, one calendar catch-up after downtime, no interval catch-up, strict non-overlap, and safe behavior across wall-clock jumps.
- [ ] 2.4 Add table-driven tests covering every schedule kind, until expiry, catch-up-once, non-overlap, unsupported calendar shapes, and forward/backward clock jumps.

## 3. Add the supervisor daemon and backend

- [ ] 3.1 Add the XDG state-rooted supervisor layout, atomic desired/run state operations, run-now requests, and size-rotated per-job logs.
- [ ] 3.2 Add the flock-guarded daemon loop, child execution through `thenn job exec`, signal handling, periodic rescans, and graceful shutdown.
- [ ] 3.3 Add hidden daemon execution and visible daemon status commands, detached lazy spawning, and liveness checks.
- [ ] 3.4 Implement `SupervisorBackend` operations over the file-based control and observation plane.
- [ ] 3.5 Add a plain-container end-to-end test covering repeated execution, pause, resume, run-now, removal, logging, and single-instance behavior.

## 4. Wire backend ownership and selection

- [ ] 4.1 Add the optional metadata backend owner field, treating absent values as systemd-owned without rewriting existing files.
- [ ] 4.2 Add the `job_backend` user configuration key with `auto`, `systemd`, and `supervisor` validation.
- [ ] 4.3 Select systemd first in auto mode and fall back to the supervisor only when systemd is unavailable; preserve current failures for explicit systemd selection.
- [ ] 4.4 Route existing-job operations and `job exec` disable behavior strictly through the metadata-recorded backend.
- [ ] 4.5 Add selection, compatibility, and mixed-backend ownership tests.

## 5. Complete rollout and documentation

- [ ] 5.1 Update CLI/TUI status wording and help so backend-neutral behavior is clear while backend-specific detail remains accurate.
- [ ] 5.2 Document supervisor survival semantics, reboot limitations, state/log locations, and backend override behavior in the README.
- [ ] 5.3 Run `just test`, `just lint`, and the plain-container verification matrix, expecting all public job operations to work through the fallback backend.
