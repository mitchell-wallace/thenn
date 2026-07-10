# `thenn job` verification — 2026-07-10

## Scope inspected

- Lap verified: `then-2c00` (`harden systemd job lifecycle`, commit `7811941`;
  completion marker `34974d1`).
- Reviewed `git show 7811941` and the changed job command, systemd backend,
  tests, and README.
- Diff hygiene check: `git diff 1e2260f..HEAD --check` passed.

## Commands run and observed results

| Command | Result |
| --- | --- |
| `just build` | PASS — built `bin/thenn`. |
| `just test` | PASS — all packages passed; `internal/cmd` 11.337s and `internal/job` passed. |
| `just lint` | PASS — `golangci-lint run ./...` reported `0 issues` (only existing linter-configuration warnings). |
| `go build -o tmp/thenn-job-verify ./cmd/thenn` | PASS — standalone temporary binary built. |
| `command -v systemctl` | `not found`; this is the intended no-systemd-client container condition. |
| `go test -v ./internal/job -run '^(TestRenderUnits|TestRenderUnitsWithIntervalSchedule|TestCheckAvailable|TestSystemdBackendWithFakeSystemctl|TestInstallRejectsExistingUnitsWithoutOverwriting)$'` | PASS — availability, collision-safe install, and calendar/interval unit rendering seams passed. |

The temporary binary was exercised with its normal environment and an isolated
`XDG_CONFIG_HOME`. Each public operational command below exited `1`, wrote no
normal output, and wrote exactly one line to stderr:

```
thenn: thenn job requires user-level systemd (systemctl --user), but systemctl was not found in PATH; install systemd to use job commands
```

| Commands checked | Exit status | Result |
| --- | ---: | --- |
| `every 5h --label verify-every -- echo ok` | 1 | PASS |
| `daily at 9pm --label verify-daily -- echo ok` | 1 | PASS |
| `once at 2099-01-01 --label verify-once -- echo ok` | 1 | PASS |
| `weekly monday at 9pm --label verify-weekly -- echo ok` | 1 | PASS |
| `weekdays at 9pm --label verify-weekdays -- echo ok` | 1 | PASS |
| `list`, `show missing`, `run missing`, `pause missing`, `resume missing`, `remove missing`, `logs missing` | 1 each | PASS |

Documentation-only rendering remained available without systemd:

- `thenn job` exited `0` and rendered job help, including the systemd
  requirement and all public subcommands.
- `thenn job --help` exited `0` and rendered the same help.
- `thenn job syntax` exited `0` and rendered all five creation forms and
  examples.

## Requirement result

**PASS.** The preceding lap meets its stated acceptance criteria: public job
operations preflight the user manager and give a single actionable error when
`systemctl` is absent; the detection seam separately tests an unreachable user
manager; lifecycle tests cover collision protection, failed-create cleanup, and
stopping a running service on removal; unit rendering/escaping is tested; and
the README/help documents the requirement. The required build, test, and lint
gates pass.

## Wake-timer fitness

For an always-running, reachable user-level systemd manager, `thenn job every
5h -- <command>` installs a `Type=oneshot` service plus a timer with
`OnActiveSec=5h` and `OnUnitActiveSec=5h`, then enables it immediately. It is
therefore suitable for best-effort five-hour periodic execution while the user
manager remains active. It is **not yet reliable enough to claim wake-timer
semantics across the requested edge cases**.

Residual risks:

- The interval timer has no `Persistent=true`; unlike the calendar paths,
  interval schedules render only monotonic `OnActiveSec`/
  `OnUnitActiveSec` triggers. A suspension/offline interval is not tested as a
  catch-up event. `Persistent=true` is present for `OnCalendar` timers but is
  not rendered for `every`.
- No `RandomizedDelaySec` is rendered. That avoids an intentional jitter, but
  the timing policy is implicit rather than documented/tested.
- Long-running commands have no explicit overlap policy. If the service is
  still active at the next timer event, systemd will not launch an independent
  second instance of the same service; the occurrence can be skipped or
  deferred rather than providing a guaranteed every-five-hours execution.
- The user manager must remain available (including user lingering when logged
  out); the README calls this out, but it is not exercised on a real systemd
  host here.
- No real-systemd suspend/resume or long-running-service integration test was
  possible in this container.

Recommended follow-up: decide and document the desired missed-fire and overlap
policy, then add a real-systemd integration test before presenting this as a
reliable agent wake timer.
