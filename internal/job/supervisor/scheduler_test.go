package supervisor

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mitchell-wallace/thenn/internal/job"
)

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Set(now time.Time)       { c.now = now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func TestCalendarScheduleKinds(t *testing.T) {
	t.Parallel()

	created := utcTime(2026, time.July, 6, 8, 0, 0) // Monday.
	tests := []struct {
		name       string
		phrase     string
		at         time.Time
		wantDue    bool
		wantNext   time.Time
		wantNoNext bool
	}{
		{
			name:     "daily before slot",
			phrase:   "daily at 9am",
			at:       utcTime(2026, time.July, 6, 8, 59, 59),
			wantNext: utcTime(2026, time.July, 6, 9, 0, 0),
		},
		{
			name:     "daily at slot",
			phrase:   "daily at 9am",
			at:       utcTime(2026, time.July, 6, 9, 0, 0),
			wantDue:  true,
			wantNext: utcTime(2026, time.July, 7, 9, 0, 0),
		},
		{
			name:     "weekdays skips weekend",
			phrase:   "weekdays at 9am",
			at:       utcTime(2026, time.July, 10, 9, 0, 0), // Friday.
			wantDue:  true,
			wantNext: utcTime(2026, time.July, 13, 9, 0, 0),
		},
		{
			name:     "weekly",
			phrase:   "weekly monday at 9am",
			at:       utcTime(2026, time.July, 6, 9, 0, 0),
			wantDue:  true,
			wantNext: utcTime(2026, time.July, 13, 9, 0, 0),
		},
		{
			name:       "once",
			phrase:     "once at 2026-07-06 09:00",
			at:         utcTime(2026, time.July, 6, 9, 0, 0),
			wantDue:    true,
			wantNoNext: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule := mustSchedule(t, tt.phrase, created)
			clock := &fakeClock{now: tt.at}
			scheduler := New(metadata(schedule, created), RuntimeState{}, WithClock(clock))

			dispatch, due := scheduler.Poll()
			if due != tt.wantDue {
				t.Fatalf("Poll() due = %v, want %v (state: %+v)", due, tt.wantDue, scheduler.State())
			}
			if !due {
				assertTime(t, scheduler.State().NextDue, tt.wantNext)
				return
			}
			if dispatch.Manual {
				t.Fatal("scheduled dispatch marked manual")
			}
			if !scheduler.Complete(0) {
				t.Fatal("Complete() rejected active dispatch")
			}
			if tt.wantNoNext {
				if scheduler.State().NextDue != nil {
					t.Fatalf("NextDue = %v, want nil", scheduler.State().NextDue)
				}
				return
			}
			assertTime(t, scheduler.State().NextDue, tt.wantNext)
		})
	}
}

func TestEveryUsesFullDelayAfterCompletion(t *testing.T) {
	t.Parallel()

	start := utcTime(2026, time.July, 6, 8, 0, 0)
	clock := &fakeClock{now: start}
	scheduler := New(metadata(mustSchedule(t, "every 10m", start), start), RuntimeState{}, WithClock(clock))
	assertTime(t, scheduler.State().NextDue, start.Add(10*time.Minute))

	clock.Advance(10 * time.Minute)
	if _, ok := scheduler.Poll(); !ok {
		t.Fatal("Poll() did not accept due interval run")
	}
	clock.Advance(3 * time.Minute)
	if !scheduler.Complete(7) {
		t.Fatal("Complete() rejected active interval run")
	}
	state := scheduler.State()
	assertTime(t, state.LastStart, start.Add(10*time.Minute))
	assertTime(t, state.LastFinish, start.Add(13*time.Minute))
	assertTime(t, state.NextDue, start.Add(23*time.Minute))
	if state.LastExitCode == nil || *state.LastExitCode != 7 {
		t.Fatalf("LastExitCode = %v, want 7", state.LastExitCode)
	}
}

func TestRestartCatchUpPolicy(t *testing.T) {
	t.Parallel()

	now := utcTime(2026, time.July, 9, 12, 0, 0)
	staleDue := utcTime(2026, time.July, 7, 9, 0, 0)
	tests := []struct {
		name     string
		phrase   string
		wantRun  bool
		wantNext time.Time
	}{
		{
			name:     "calendar catches up once",
			phrase:   "daily at 9am",
			wantRun:  true,
			wantNext: utcTime(2026, time.July, 10, 9, 0, 0),
		},
		{
			name:     "interval does not catch up",
			phrase:   "every 1h",
			wantRun:  false,
			wantNext: now.Add(time.Hour),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &fakeClock{now: now}
			schedule := mustSchedule(t, tt.phrase, utcTime(2026, time.July, 6, 8, 0, 0))
			scheduler := New(metadata(schedule, utcTime(2026, time.July, 6, 8, 0, 0)), RuntimeState{NextDue: timePointer(staleDue)}, WithClock(clock))
			dispatch, ok := scheduler.Poll()
			if ok != tt.wantRun {
				t.Fatalf("Poll() accepted = %v, want %v", ok, tt.wantRun)
			}
			if ok {
				if !dispatch.ScheduledFor.Equal(staleDue) {
					t.Fatalf("ScheduledFor = %v, want stale slot %v", dispatch.ScheduledFor, staleDue)
				}
				scheduler.Complete(0)
				if _, replayed := scheduler.Poll(); replayed {
					t.Fatal("Poll() replayed a second missed calendar slot")
				}
			}
			assertTime(t, scheduler.State().NextDue, tt.wantNext)
		})
	}
}

func TestUntilExpiry(t *testing.T) {
	t.Parallel()

	now := utcTime(2026, time.July, 6, 8, 0, 0)
	schedule := mustSchedule(t, "every 10m until 2026-07-06", now)
	// Date-only until values expire at the end of that date. Use an explicit
	// boundary to exercise the scheduler's inclusive expiry rule.
	until := now.Add(5 * time.Minute)
	schedule.Until = &until
	clock := &fakeClock{now: now}
	scheduler := New(metadata(schedule, now), RuntimeState{}, WithClock(clock))
	clock.Set(until)

	if _, ok := scheduler.Poll(); ok {
		t.Fatal("Poll() accepted run at the until boundary")
	}
	if _, ok := scheduler.RunNow(); ok {
		t.Fatal("RunNow() accepted run after expiry")
	}
	if !scheduler.Expired() {
		t.Fatal("Expired() = false, want true")
	}
	if scheduler.State().NextDue != nil {
		t.Fatalf("NextDue = %v, want nil after expiry", scheduler.State().NextDue)
	}
}

func TestStrictNonOverlap(t *testing.T) {
	t.Parallel()

	now := utcTime(2026, time.July, 6, 9, 0, 0)
	clock := &fakeClock{now: now}
	scheduler := New(metadata(mustSchedule(t, "daily at 9am", now.Add(-time.Hour)), now.Add(-time.Hour)), RuntimeState{}, WithClock(clock))
	if _, ok := scheduler.Poll(); !ok {
		t.Fatal("first Poll() rejected")
	}
	if _, ok := scheduler.Poll(); ok {
		t.Fatal("second Poll() overlapped active run")
	}
	if _, ok := scheduler.RunNow(); ok {
		t.Fatal("RunNow() overlapped active run")
	}
	if !scheduler.Running() {
		t.Fatal("Running() = false during active dispatch")
	}
	if !scheduler.Complete(0) {
		t.Fatal("Complete() rejected active run")
	}
	if scheduler.Complete(0) {
		t.Fatal("second Complete() accepted without active run")
	}
	if _, ok := scheduler.RunNow(); !ok {
		t.Fatal("RunNow() rejected after prior run completed")
	}
}

func TestClockJumps(t *testing.T) {
	t.Parallel()

	t.Run("backward does not fire early", func(t *testing.T) {
		start := utcTime(2026, time.July, 6, 8, 0, 0)
		clock := &fakeClock{now: start}
		scheduler := New(metadata(mustSchedule(t, "every 1h", start), start), RuntimeState{}, WithClock(clock))
		clock.Set(start.Add(-24 * time.Hour))
		if _, ok := scheduler.Poll(); ok {
			t.Fatal("Poll() fired after backward clock jump")
		}
		clock.Set(start.Add(59 * time.Minute))
		if _, ok := scheduler.Poll(); ok {
			t.Fatal("Poll() fired before original due instant")
		}
		clock.Set(start.Add(time.Hour))
		if _, ok := scheduler.Poll(); !ok {
			t.Fatal("Poll() did not fire at original due instant")
		}
	})

	t.Run("forward fires at most one", func(t *testing.T) {
		start := utcTime(2026, time.July, 6, 8, 0, 0)
		clock := &fakeClock{now: start}
		scheduler := New(metadata(mustSchedule(t, "daily at 9am", start), start), RuntimeState{}, WithClock(clock))
		clock.Set(start.Add(5 * 24 * time.Hour))
		if _, ok := scheduler.Poll(); !ok {
			t.Fatal("Poll() did not catch up after forward jump")
		}
		scheduler.Complete(0)
		if _, ok := scheduler.Poll(); ok {
			t.Fatal("Poll() replayed multiple slots after forward jump")
		}
		assertTime(t, scheduler.State().NextDue, utcTime(2026, time.July, 11, 9, 0, 0))
	})
}

func TestUnsupportedCalendarShapeBecomesStateError(t *testing.T) {
	t.Parallel()

	now := utcTime(2026, time.July, 6, 8, 0, 0)
	schedule := job.Schedule{Kind: job.ScheduleDaily, OnCalendar: "*-*-01 09:00:00"}
	scheduler := New(metadata(schedule, now), RuntimeState{}, WithClock(&fakeClock{now: now}))
	state := scheduler.State()
	if !strings.Contains(state.Error, "unsupported OnCalendar expression") {
		t.Fatalf("Error = %q, want unsupported expression error", state.Error)
	}
	if state.NextDue != nil {
		t.Fatalf("NextDue = %v, want nil", state.NextDue)
	}
	if _, ok := scheduler.Poll(); ok {
		t.Fatal("Poll() accepted errored schedule")
	}
}

func TestRuntimeStateJSONRoundTrip(t *testing.T) {
	t.Parallel()

	start := utcTime(2026, time.July, 6, 8, 0, 0)
	finish := start.Add(time.Minute)
	due := finish.Add(time.Hour)
	exitCode := 3
	want := RuntimeState{
		LastStart:    &start,
		LastFinish:   &finish,
		NextDue:      &due,
		LastExitCode: &exitCode,
		Error:        "child failed",
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got RuntimeState
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if !got.LastStart.Equal(*want.LastStart) || !got.LastFinish.Equal(*want.LastFinish) || !got.NextDue.Equal(*want.NextDue) || *got.LastExitCode != exitCode || got.Error != want.Error {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

func mustSchedule(t *testing.T, phrase string, now time.Time) job.Schedule {
	t.Helper()
	schedule, err := job.ParseScheduleString(phrase, job.WithNow(now))
	if err != nil {
		t.Fatalf("ParseScheduleString(%q): %v", phrase, err)
	}
	return schedule
}

func metadata(schedule job.Schedule, created time.Time) job.Metadata {
	return job.Metadata{ParsedSchedule: schedule, CreatedAt: created}
}

func utcTime(year int, month time.Month, day, hour, minute, second int) time.Time {
	return time.Date(year, month, day, hour, minute, second, 0, time.UTC)
}

func timePointer(value time.Time) *time.Time { return &value }

func assertTime(t *testing.T, got *time.Time, want time.Time) {
	t.Helper()
	if got == nil || !got.Equal(want) {
		t.Fatalf("time = %v, want %v", got, want)
	}
}
