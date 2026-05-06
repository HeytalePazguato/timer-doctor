package audit

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixedNow() time.Time { return time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC) }

func cfg() Config {
	return Config{
		Now:                func() time.Time { return fixedNow() },
		SkipSystemdAnalyze: true,
	}
}

func auditDir(t *testing.T, dir string) *Result {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("..", "..", "testdata", dir, "*.timer"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	r, err := AuditPaths(matches, cfg())
	if err != nil {
		t.Fatalf("AuditPaths: %v", err)
	}
	return r
}

func hasCode(fs []Finding, code string) bool {
	for _, f := range fs {
		if f.Code == code {
			return true
		}
	}
	return false
}

// TestCleanFixtureProducesNoErrors makes sure the happy-path fixture
// emits no error-level findings. A few warns/info notes are expected
// (ExecStart paths don't exist on the dev box, etc).
func TestCleanFixture(t *testing.T) {
	r := auditDir(t, "clean")
	if len(r.Timers) != 1 {
		t.Fatalf("got %d timers, want 1", len(r.Timers))
	}
	tm := r.Timers[0]
	if hasCode(tm.Findings, "no_persistent") {
		t.Errorf("clean fixture emitted no_persistent")
	}
	if hasCode(tm.Findings, "no_randomized_delay") {
		t.Errorf("clean fixture emitted no_randomized_delay")
	}
	if hasCode(tm.Findings, "bad_calendar") {
		t.Errorf("clean fixture emitted bad_calendar")
	}
	if len(tm.Schedules) != 1 || tm.Schedules[0].Raw != "*-*-* 04:00:00" {
		t.Errorf("schedules = %+v", tm.Schedules)
	}
	if !strings.Contains(strings.ToLower(tm.Schedules[0].Explanation), "every day at 04:00") {
		t.Errorf("Explanation = %q", tm.Schedules[0].Explanation)
	}
}

func TestMissingService(t *testing.T) {
	r := auditDir(t, "missing_service")
	if len(r.Timers) != 1 {
		t.Fatalf("want 1 timer, got %d", len(r.Timers))
	}
	if !hasCode(r.Timers[0].Findings, "missing_service") {
		t.Errorf("expected missing_service finding, got %+v", r.Timers[0].Findings)
	}
}

func TestBadCalendar(t *testing.T) {
	r := auditDir(t, "bad_calendar")
	if !hasCode(r.Timers[0].Findings, "bad_calendar") {
		t.Errorf("expected bad_calendar finding, got %+v", r.Timers[0].Findings)
	}
}

func TestEveryMinuteWithoutFlock(t *testing.T) {
	r := auditDir(t, "noisy")
	tm := r.Timers[0]
	if !hasCode(tm.Findings, "no_flock") {
		t.Errorf("expected no_flock finding, got %+v", tm.Findings)
	}
}

func TestConflictingPairs(t *testing.T) {
	r := auditDir(t, "conflicting")
	if len(r.Timers) != 2 {
		t.Fatalf("want 2 timers, got %d", len(r.Timers))
	}
	if !hasCode(r.Timers[0].Findings, "conflicting_pair") || !hasCode(r.Timers[1].Findings, "conflicting_pair") {
		t.Errorf("expected conflicting_pair on both timers; got 0=%+v 1=%+v", r.Timers[0].Findings, r.Timers[1].Findings)
	}
}

func TestOrphanService(t *testing.T) {
	r := auditDir(t, "orphan")
	found := false
	for _, f := range r.Orphans {
		if f.Code == "orphan_service" && strings.Contains(f.Message, "stranded.service") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected orphan_service finding for stranded.service, got %+v", r.Orphans)
	}
}

func TestExpressionMode(t *testing.T) {
	cases := []string{
		"OnCalendar=*-*-* 04:00:00",
		"*-*-* 04:00:00",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			tm := AuditExpression(raw, cfg())
			if len(tm.Schedules) != 1 {
				t.Fatalf("got %d schedules, want 1", len(tm.Schedules))
			}
			if hasCode(tm.Findings, "bad_calendar") {
				t.Errorf("unexpected bad_calendar finding for %q", raw)
			}
			if len(tm.Schedules[0].NextRuns) != 3 {
				t.Errorf("got %d next runs, want 3", len(tm.Schedules[0].NextRuns))
			}
		})
	}
}

func TestExpressionModeBadCalendar(t *testing.T) {
	tm := AuditExpression("*-*-* 99:00:00", cfg())
	if !hasCode(tm.Findings, "bad_calendar") {
		t.Errorf("expected bad_calendar finding, got %+v", tm.Findings)
	}
}
