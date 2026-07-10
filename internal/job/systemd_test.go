package job

import (
	"strings"
	"testing"
	"time"
)

func TestRenderUnits(t *testing.T) {
	metadata := testMetadata(t)
	metadata.CWD = `/home/test/$workspace/with space % done`
	metadata.CommandArgv = []string{"sh", "-c", `printf '%s\n' "hello world"`}
	units, err := RenderUnits(metadata, `/opt/$TOOLS/thenn "bin"`)
	if err != nil {
		t.Fatalf("RenderUnits() error = %v", err)
	}

	serviceChecks := []string{
		"Description=thenn job backup-daily",
		"Type=oneshot",
		`WorkingDirectory="/home/test/$workspace/with space %% done"`,
		`ExecStart="/opt/$$TOOLS/thenn \"bin\"" job exec backup-daily`,
	}
	if strings.Contains(units.Service, "hello world") {
		t.Fatalf("service unit embedded command argv instead of using stored metadata:\n%s", units.Service)
	}
	for _, check := range serviceChecks {
		if !strings.Contains(units.Service, check) {
			t.Fatalf("service unit missing %q:\n%s", check, units.Service)
		}
	}

	timerChecks := []string{
		"Description=Run thenn job backup-daily",
		"OnCalendar=*-*-* 02:00:00",
		"Persistent=true",
		"AccuracySec=1s",
		"RandomizedDelaySec=0",
		"WakeSystem=false",
		"Unit=thenn-job-backup-daily.service",
		"WantedBy=timers.target",
	}
	for _, check := range timerChecks {
		if !strings.Contains(units.Timer, check) {
			t.Fatalf("timer unit missing %q:\n%s", check, units.Timer)
		}
	}
}

func TestRenderUnitsWithIntervalSchedule(t *testing.T) {
	metadata := testMetadata(t)
	metadata.ParsedSchedule = Schedule{
		Kind:            ScheduleEvery,
		Interval:        15 * time.Minute,
		OnUnitActiveSec: "15m",
	}
	units, err := RenderUnits(metadata, "/usr/local/bin/thenn")
	if err != nil {
		t.Fatalf("RenderUnits() error = %v", err)
	}
	if !strings.Contains(units.Timer, "OnUnitInactiveSec=15m") {
		t.Fatalf("timer unit missing interval trigger:\n%s", units.Timer)
	}
	if !strings.Contains(units.Timer, "OnActiveSec=15m") {
		t.Fatalf("timer unit missing initial interval trigger:\n%s", units.Timer)
	}
	if strings.Contains(units.Timer, "OnUnitActiveSec=") {
		t.Fatalf("timer unit uses fixed-rate rather than fixed-delay semantics:\n%s", units.Timer)
	}
	if strings.Contains(units.Timer, "OnCalendar=") {
		t.Fatalf("timer unit unexpectedly contains OnCalendar:\n%s", units.Timer)
	}
	for _, policy := range []string{"AccuracySec=1s", "RandomizedDelaySec=0", "WakeSystem=false"} {
		if !strings.Contains(units.Timer, policy) {
			t.Fatalf("timer unit missing explicit policy %q:\n%s", policy, units.Timer)
		}
	}
	if strings.Contains(units.Timer, "Persistent=") {
		t.Fatalf("monotonic timer unexpectedly claims persistence:\n%s", units.Timer)
	}
}

func testMetadata(t *testing.T) Metadata {
	t.Helper()
	schedule, err := NewParsedSchedule("*-*-* 02:00:00")
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := NewMetadata("backup-daily", "daily at 2am", schedule, []string{"/usr/bin/backup", "--fast"}, "/home/test", time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return metadata
}
