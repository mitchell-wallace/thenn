// Package supervisor provides the process-independent scheduling core used by
// the thenn-owned job supervisor.
package supervisor

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/mitchell-wallace/thenn/internal/job"
)

// Clock supplies wall time to a Scheduler. time.Time's monotonic component is
// retained while the daemon stays alive, while persisted timestamps continue
// to work after a restart.
type Clock interface {
	Now() time.Time
}

// ClockFunc adapts a function into a Clock.
type ClockFunc func() time.Time

// Now implements Clock.
func (f ClockFunc) Now() time.Time { return f() }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

type config struct {
	clock Clock
}

// Option customizes a Scheduler.
type Option func(*config)

// WithClock injects the scheduler clock. It is the runtime counterpart to
// job.WithNow, which injects the schedule parser's reference time.
func WithClock(clock Clock) Option {
	return func(cfg *config) {
		if clock != nil {
			cfg.clock = clock
		}
	}
}

// RuntimeState is the scheduler state persisted by the daemon between runs.
type RuntimeState struct {
	LastStart    *time.Time `json:"last_start,omitempty"`
	LastFinish   *time.Time `json:"last_finish,omitempty"`
	NextDue      *time.Time `json:"next_due,omitempty"`
	LastExitCode *int       `json:"last_exit_code,omitempty"`
	Error        string     `json:"error,omitempty"`
}

// Dispatch describes a run accepted by the scheduler.
type Dispatch struct {
	ScheduledFor time.Time
	Manual       bool
}

// Scheduler is a synchronous state machine. Poll and RunNow can accept a run;
// callers must pair an accepted run with Complete before another can start.
type Scheduler struct {
	metadata job.Metadata
	clock    Clock
	state    RuntimeState
	calendar calendarSpec
	running  bool
	expired  bool
}

// New constructs a scheduler from job metadata and previously persisted state.
// Unsupported calendar expressions are retained as state errors so the daemon
// can persist and report them instead of silently guessing their meaning.
func New(metadata job.Metadata, state RuntimeState, opts ...Option) *Scheduler {
	cfg := config{clock: realClock{}}
	for _, opt := range opts {
		opt(&cfg)
	}
	s := &Scheduler{metadata: metadata, clock: cfg.clock, state: cloneState(state)}
	s.initialize()
	return s
}

// State returns a copy suitable for atomic persistence by the daemon layer.
func (s *Scheduler) State() RuntimeState { return cloneState(s.state) }

// Running reports whether a dispatch has not yet completed.
func (s *Scheduler) Running() bool { return s.running }

// Expired reports whether the schedule's until limit has been reached.
func (s *Scheduler) Expired() bool {
	s.refreshExpiry(s.clock.Now())
	return s.expired
}

// Poll accepts the scheduled run due at the clock's current time. A forward
// jump produces at most one dispatch; a backward jump cannot fire before the
// persisted due instant.
func (s *Scheduler) Poll() (Dispatch, bool) {
	if !s.canStart() || s.state.NextDue == nil {
		return Dispatch{}, false
	}
	now := s.clock.Now()
	if now.Before(*s.state.NextDue) {
		return Dispatch{}, false
	}
	return s.start(Dispatch{ScheduledFor: *s.state.NextDue})
}

// RunNow accepts an immediate manual run. It returns false while another run
// is active, implementing the supervisor's drop-on-overlap policy.
func (s *Scheduler) RunNow() (Dispatch, bool) {
	if !s.canStart() {
		return Dispatch{}, false
	}
	now := s.clock.Now()
	return s.start(Dispatch{ScheduledFor: now, Manual: true})
}

// Complete records a child result and computes the next due instant. Every
// schedules use the completion time as their new base. Calendar schedules skip
// directly to the first future slot, which collapses any clock jump or downtime
// into a single catch-up run.
func (s *Scheduler) Complete(exitCode int) bool {
	if !s.running {
		return false
	}
	now := s.clock.Now()
	s.state.LastFinish = timePtr(now)
	s.state.LastExitCode = intPtr(exitCode)
	s.running = false

	schedule := s.metadata.ParsedSchedule
	switch schedule.Kind {
	case job.ScheduleEvery:
		due := now.Add(schedule.Interval)
		s.state.NextDue = &due
	case job.ScheduleOnce:
		s.state.NextDue = nil
	default:
		due := s.calendar.next(now, false)
		s.state.NextDue = &due
	}
	s.refreshExpiry(now)
	return true
}

func (s *Scheduler) initialize() {
	schedule := s.metadata.ParsedSchedule
	now := s.clock.Now()
	s.state.Error = ""

	switch schedule.Kind {
	case job.ScheduleEvery:
		if schedule.Interval <= 0 {
			s.fail("every schedule requires a positive interval")
			return
		}
		// Interval downtime never counts. A stale persisted due time starts a
		// fresh full delay, unlike calendar persistence/catch-up.
		if s.state.NextDue == nil || !s.state.NextDue.After(now) {
			base := now
			if s.state.NextDue == nil && s.state.LastFinish != nil && s.state.LastFinish.After(now) {
				base = *s.state.LastFinish
			}
			due := base.Add(schedule.Interval)
			s.state.NextDue = &due
		}
	case job.ScheduleDaily, job.ScheduleWeekdays, job.ScheduleWeekly, job.ScheduleOnce:
		calendar, err := parseCalendar(schedule.Kind, schedule.OnCalendar, now.Location())
		if err != nil {
			s.fail(err.Error())
			return
		}
		s.calendar = calendar
		if schedule.Kind == job.ScheduleOnce && s.state.LastFinish != nil {
			s.state.NextDue = nil
			break
		}
		if s.state.NextDue == nil {
			base := s.metadata.CreatedAt
			inclusive := true
			if base.IsZero() {
				base = now
			}
			if s.state.LastFinish != nil {
				base = *s.state.LastFinish
				inclusive = false
			}
			due := calendar.next(base, inclusive)
			s.state.NextDue = &due
		}
	default:
		s.fail(fmt.Sprintf("unsupported schedule kind %q", schedule.Kind))
		return
	}
	s.refreshExpiry(now)
}

func (s *Scheduler) canStart() bool {
	now := s.clock.Now()
	s.refreshExpiry(now)
	return !s.running && !s.expired && s.state.Error == ""
}

func (s *Scheduler) start(dispatch Dispatch) (Dispatch, bool) {
	now := s.clock.Now()
	s.running = true
	s.state.LastStart = timePtr(now)
	return dispatch, true
}

func (s *Scheduler) refreshExpiry(now time.Time) {
	until := s.metadata.ParsedSchedule.Until
	if until != nil && !now.Before(*until) {
		s.expired = true
		s.state.NextDue = nil
	}
}

func (s *Scheduler) fail(message string) {
	s.state.Error = message
	s.state.NextDue = nil
}

type calendarSpec struct {
	kind    job.ScheduleKind
	hour    int
	minute  int
	second  int
	weekday time.Weekday
	once    time.Time
	loc     *time.Location
}

var (
	dailyCalendarRe    = regexp.MustCompile(`^\*-\*-\* (\d{2}):(\d{2}):(\d{2})$`)
	weekdayCalendarRe  = regexp.MustCompile(`^Mon\.\.Fri \*-\*-\* (\d{2}):(\d{2}):(\d{2})$`)
	weeklyCalendarRe   = regexp.MustCompile(`^(Mon|Tue|Wed|Thu|Fri|Sat|Sun) \*-\*-\* (\d{2}):(\d{2}):(\d{2})$`)
	absoluteCalendarRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`)
)

func parseCalendar(kind job.ScheduleKind, expression string, loc *time.Location) (calendarSpec, error) {
	expression = strings.TrimSpace(expression)
	spec := calendarSpec{kind: kind, loc: loc}
	var match []string
	switch kind {
	case job.ScheduleDaily:
		match = dailyCalendarRe.FindStringSubmatch(expression)
	case job.ScheduleWeekdays:
		match = weekdayCalendarRe.FindStringSubmatch(expression)
	case job.ScheduleWeekly:
		match = weeklyCalendarRe.FindStringSubmatch(expression)
	case job.ScheduleOnce:
		if !absoluteCalendarRe.MatchString(expression) {
			return calendarSpec{}, unsupportedCalendar(expression)
		}
		when, err := time.ParseInLocation("2006-01-02 15:04:05", expression, loc)
		if err != nil {
			return calendarSpec{}, unsupportedCalendar(expression)
		}
		spec.once = when
		return spec, nil
	}
	if match == nil {
		return calendarSpec{}, unsupportedCalendar(expression)
	}
	offset := 1
	if kind == job.ScheduleWeekly {
		weekday, ok := weekdays[match[1]]
		if !ok {
			return calendarSpec{}, unsupportedCalendar(expression)
		}
		spec.weekday = weekday
		offset = 2
	}
	if _, err := fmt.Sscanf(strings.Join(match[offset:], ":"), "%d:%d:%d", &spec.hour, &spec.minute, &spec.second); err != nil || spec.hour > 23 || spec.minute > 59 || spec.second > 59 {
		return calendarSpec{}, unsupportedCalendar(expression)
	}
	return spec, nil
}

var weekdays = map[string]time.Weekday{
	"Sun": time.Sunday, "Mon": time.Monday, "Tue": time.Tuesday,
	"Wed": time.Wednesday, "Thu": time.Thursday, "Fri": time.Friday, "Sat": time.Saturday,
}

func unsupportedCalendar(expression string) error {
	return fmt.Errorf("unsupported OnCalendar expression %q", expression)
}

func (c calendarSpec) next(after time.Time, inclusive bool) time.Time {
	if c.kind == job.ScheduleOnce {
		return c.once
	}
	after = after.In(c.loc)
	for day := 0; ; day++ {
		date := after.AddDate(0, 0, day)
		candidate := time.Date(date.Year(), date.Month(), date.Day(), c.hour, c.minute, c.second, 0, c.loc)
		if c.kind == job.ScheduleWeekdays && (candidate.Weekday() == time.Saturday || candidate.Weekday() == time.Sunday) {
			continue
		}
		if c.kind == job.ScheduleWeekly && candidate.Weekday() != c.weekday {
			continue
		}
		if candidate.After(after) || (inclusive && candidate.Equal(after)) {
			return candidate
		}
	}
}

func cloneState(state RuntimeState) RuntimeState {
	state.LastStart = cloneTime(state.LastStart)
	state.LastFinish = cloneTime(state.LastFinish)
	state.NextDue = cloneTime(state.NextDue)
	if state.LastExitCode != nil {
		value := *state.LastExitCode
		state.LastExitCode = &value
	}
	return state
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func timePtr(value time.Time) *time.Time { return &value }
func intPtr(value int) *int              { return &value }
