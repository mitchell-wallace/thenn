# Delta Spec: Job scheduling without user-level systemd

## ADDED Requirements

### Requirement: Job commands operate without user-level systemd

When no reachable `systemd --user` manager exists and the job backend is not explicitly pinned to systemd, `thenn job` commands SHALL manage jobs through thenn's own supervisor backend instead of failing.

#### Scenario: Creating a job in a container without systemctl

- **GIVEN** a Linux environment where `systemctl` is not in PATH
- **WHEN** the user runs `thenn job every 15m --label check -- echo ok`
- **THEN** the command exits 0, persists the job, and the job's metadata records the supervisor backend as its owner

#### Scenario: systemd remains preferred when reachable

- **GIVEN** a Linux session with a reachable `systemd --user` manager and default configuration
- **WHEN** the user creates a job
- **THEN** the job SHALL be installed as systemd user units exactly as before

### Requirement: Supervisor-managed jobs honor the documented timer policy

Supervisor-executed jobs SHALL follow the same observable policy as systemd-backed jobs: `every` waits the full interval after the prior run finishes, jobs never overlap themselves, calendar schedules catch up at most once after downtime, `until` limits stop future runs, and `once` jobs disable themselves after running.

#### Scenario: Fixed delay after completion

- **GIVEN** a supervisor-managed job `every 5s` whose command takes 3 seconds
- **WHEN** the job runs
- **THEN** the next run starts no sooner than 5 seconds after the previous run finished

#### Scenario: Calendar catch-up fires once

- **GIVEN** a supervisor-managed `daily at 9pm` job and a supervisor daemon that was not running at 9pm
- **WHEN** the daemon starts later the same day
- **THEN** the job runs exactly once as catch-up and then resumes its normal schedule

### Requirement: Supervisor daemon is single-instance and survives shell exit

At most one supervisor daemon per user SHALL run at a time, it SHALL keep executing scheduled jobs after the launching terminal or session closes, and job commands SHALL start it on demand when it is not running.

#### Scenario: Daemon outlives the shell

- **GIVEN** a job created from an interactive shell using the supervisor backend
- **WHEN** the shell exits
- **THEN** the job continues to run on schedule

#### Scenario: Concurrent daemon starts collapse to one

- **WHEN** two job commands race to start the daemon
- **THEN** exactly one daemon process holds the supervisor lock and the other start attempt exits without error

### Requirement: Job logs and status are available without journalctl

For supervisor-managed jobs, `thenn job logs <label>` SHALL return recent captured stdout/stderr of the job command, and `thenn job show <label>` SHALL report scheduling state (enabled/paused, last run, next due) without requiring journalctl or systemctl.

#### Scenario: Reading logs in a minimal container

- **GIVEN** a supervisor-managed job that has run at least once in an environment without journalctl
- **WHEN** the user runs `thenn job logs <label>`
- **THEN** the command exits 0 and prints the most recent output of the job command
