# Wake-timer semantics verification — 2026-07-10

## Scope

Verified lap `then-e79d` (`define reliable systemd timer semantics`, commit
`e9834ad`) against the follow-up acceptance criteria in `then-2e29`.

## Results

| Check | Result | Evidence |
| --- | --- | --- |
| Build | PASS | `just build` completed and produced `bin/thenn`. |
| Unit suite | PASS | `just test` completed: `internal/cmd`, `internal/job`, and `internal/timer` passed. |
| Lint | PASS | `just lint` completed with `0 issues`; it emitted only existing golangci-lint configuration warnings. |
| Rendered `every` serialization | PASS (unit/static) | `renderTimerTriggers` emits `OnActiveSec=<interval>` for the initial run and `OnUnitInactiveSec=<interval>` for later runs; `TestRenderUnitsWithIntervalSchedule` asserts this and rejects `OnUnitActiveSec=`. |
| Explicit timer policy | PASS (unit/static) | Both timer paths render `AccuracySec=1s`, `RandomizedDelaySec=0`, and `WakeSystem=false`; `TestRenderUnits` and `TestRenderUnitsWithIntervalSchedule` assert those fields. |
| Persistent-state handling | PASS (unit/static) | `every` rendering contains no `Persistent=`; calendar rendering contains `Persistent=true`; removal calls `systemctl --user clean --what=state <timer>`, asserted by `TestSystemdBackendWithFakeSystemctl`. This removes calendar timestamp state before a label is reused. |
| Documentation policy | PASS | README states fixed-delay/non-overlap behavior for `every`, active monotonic-time behavior during suspend/power-off/user-manager downtime, one calendar catch-up activation, one-second accuracy, no randomized delay, no hardware wake, and lingering setup/limits. `thenn job syntax` states the same concise policy. |
| Real-systemd integration | NOT VERIFIED | The required opt-in command was executed, but its sole test skipped before installing units because `systemctl` is absent in this container. |

## Commands run

```text
just build
just test
just lint
THENN_SYSTEMD_INTEGRATION=1 go test -v -tags=integration ./internal/job -run TestRealSystemd
```

The final command exited successfully because the test deliberately skipped:

```text
systemd_integration_test.go:25: systemctl is unavailable: exec: "systemctl": executable file not found in $PATH
--- SKIP: TestRealSystemdEveryWaitsAfterOneshotCompletion
```

Host preflight also found no `systemctl` or `loginctl`, and both
`XDG_RUNTIME_DIR` and `DBUS_SESSION_BUS_ADDRESS` are unset. This is not a host
with a reachable `systemd --user` manager, so it cannot establish the required
real-manager proof of serialization/timing or cleanup.

## Acceptance conclusion

**PARTIAL / BLOCKED BY ENVIRONMENT.** The source, unit tests, documentation,
and ordinary build/test/lint gates meet the specified semantics. Real-systemd
acceptance remains unverified and should not be reported as passed until the
opt-in test runs without a skip on a Linux user session with an active user
manager.

## Residual risk and follow-up

Run the quoted integration command unchanged on a Linux host where all of the
following succeed: `command -v systemctl`, `systemctl --user show-environment`,
and a non-empty `XDG_RUNTIME_DIR`. The test intentionally creates runtime user
units briefly and cleans them up, so it should run under the intended target
user rather than against a system service.
