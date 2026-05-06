// Package audit runs lint-style checks on parsed timer/service pairs.
//
// Each check lives in its own function in checks.go and accepts a
// *Timer plus, when needed, surrounding context. Adding a new check
// means adding one function and one entry to perTimerChecks (or
// wholeBatchChecks for cross-timer analyses).
package audit

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/HeytalePazguato/timer-doctor/internal/calendar"
	"github.com/HeytalePazguato/timer-doctor/internal/parser"
)

// Severity classifies the urgency of a finding.
type Severity string

const (
	SevError Severity = "error"
	SevWarn  Severity = "warn"
	SevInfo  Severity = "info"
)

// Finding is a single audit result attached to a timer.
type Finding struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	Line     int      `json:"line,omitempty"`
}

// Schedule is one fire-time directive (OnCalendar, OnBootSec, etc).
type Schedule struct {
	Type        string      `json:"type"`
	Raw         string      `json:"raw"`
	Line        int         `json:"line"`
	Explanation string      `json:"explanation,omitempty"`
	Spec        *calendar.Spec `json:"-"`
	NextRuns    []time.Time `json:"next_runs,omitempty"`
	ParseError  string      `json:"parse_error,omitempty"`
}

// Timer is a parsed .timer file with its paired .service (if any) and
// the audit results attached.
type Timer struct {
	Path        string
	Unit        string
	ServicePath string
	ServiceUnit string

	TimerFile   *parser.UnitFile
	ServiceFile *parser.UnitFile

	Schedules []Schedule

	// Cached values derived from the timer file for the checks below.
	Persistent          bool
	RandomizedDelay     bool
	WakeSystem          bool
	AccuracySec         string

	Findings []Finding
}

// Result is a batch of audited timers plus any orphan service findings.
type Result struct {
	Timers   []*Timer
	Orphans  []Finding
	Now      time.Time
}

// Config tunes audit behavior.
type Config struct {
	// Now lets tests pin the clock; defaults to time.Now().
	Now func() time.Time
	// SkipServiceChecks turns off filesystem inspection of ExecStart
	// targets. Used in expression mode and tests.
	SkipServiceChecks bool
	// SkipSystemdAnalyze forces the built-in calendar parser even when
	// `systemd-analyze` is on PATH. Used in tests for determinism.
	SkipSystemdAnalyze bool
	// ConflictWindow is the maximum time gap between two timer fire
	// times that counts as a conflicting pair. Defaults to 60s.
	ConflictWindow time.Duration
}

// AuditPaths parses every .timer file in `paths` and runs all checks.
func AuditPaths(paths []string, cfg Config) (*Result, error) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.ConflictWindow == 0 {
		cfg.ConflictWindow = 60 * time.Second
	}
	res := &Result{Now: cfg.Now()}
	for _, p := range paths {
		t, err := loadTimer(p, cfg)
		if err != nil {
			res.Timers = append(res.Timers, &Timer{
				Path: p,
				Unit: filepath.Base(p),
				Findings: []Finding{{
					Severity: SevError,
					Code:     "parse_error",
					Message:  err.Error(),
				}},
			})
			continue
		}
		res.Timers = append(res.Timers, t)
	}
	for _, t := range res.Timers {
		if t.TimerFile == nil {
			continue
		}
		runPerTimerChecks(t, cfg, res.Now)
	}
	runBatchChecks(res, cfg)
	for _, t := range res.Timers {
		sort.SliceStable(t.Findings, func(i, j int) bool {
			return sevRank(t.Findings[i].Severity) < sevRank(t.Findings[j].Severity)
		})
	}
	return res, nil
}

// AuditExpression runs only the schedule-related checks for a bare
// OnCalendar= string.
func AuditExpression(raw string, cfg Config) *Timer {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	now := cfg.Now()
	t := &Timer{Unit: "(expression)"}
	sch := Schedule{Type: "OnCalendar", Raw: stripPrefix(raw)}
	spec, err := calendar.Parse(raw)
	if err != nil {
		sch.ParseError = err.Error()
		t.Findings = append(t.Findings, Finding{
			Severity: SevError,
			Code:     "bad_calendar",
			Message:  "OnCalendar parse failed: " + err.Error(),
		})
	} else {
		sch.Spec = spec
		sch.Explanation = spec.Explain()
		sch.NextRuns = computeNext(spec, raw, now, 3, cfg)
	}
	t.Schedules = append(t.Schedules, sch)
	return t
}

func loadTimer(path string, cfg Config) (*Timer, error) {
	uf, err := parser.ParseFile(path)
	if err != nil {
		return nil, err
	}
	t := &Timer{
		Path:      path,
		Unit:      filepath.Base(path),
		TimerFile: uf,
	}
	for _, pe := range uf.ParseErrors {
		t.Findings = append(t.Findings, Finding{
			Severity: SevError,
			Code:     "parse_error",
			Message:  pe.Message,
			Line:     pe.Line,
		})
	}
	timerSec := uf.Section("Timer")
	if timerSec == nil {
		t.Findings = append(t.Findings, Finding{
			Severity: SevError,
			Code:     "parse_error",
			Message:  "missing [Timer] section",
		})
		return t, nil
	}

	if v, ok := timerSec.Get("Persistent"); ok {
		t.Persistent = strings.EqualFold(strings.TrimSpace(v), "true") || v == "yes" || v == "1"
	}
	if _, ok := timerSec.Get("RandomizedDelaySec"); ok {
		t.RandomizedDelay = true
	}
	if v, ok := timerSec.Get("WakeSystem"); ok {
		t.WakeSystem = strings.EqualFold(strings.TrimSpace(v), "true")
	}
	if v, ok := timerSec.Get("AccuracySec"); ok {
		t.AccuracySec = v
	}

	t.ServicePath = parser.PairedServicePath(path, uf)
	t.ServiceUnit = filepath.Base(t.ServicePath)
	if !cfg.SkipServiceChecks {
		if sf, err := parser.ParseFile(t.ServicePath); err == nil {
			t.ServiceFile = sf
		}
	}
	return t, nil
}

func sevRank(s Severity) int {
	switch s {
	case SevError:
		return 0
	case SevWarn:
		return 1
	case SevInfo:
		return 2
	}
	return 3
}

// Summary tallies findings across an entire result set.
type Summary struct {
	Timers   int `json:"timers"`
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`
}

// Summarize counts findings by severity across the batch.
func Summarize(r *Result) Summary {
	s := Summary{Timers: len(r.Timers)}
	count := func(f Finding) {
		switch f.Severity {
		case SevError:
			s.Errors++
		case SevWarn:
			s.Warnings++
		case SevInfo:
			s.Info++
		}
	}
	for _, t := range r.Timers {
		for _, f := range t.Findings {
			count(f)
		}
	}
	for _, f := range r.Orphans {
		count(f)
	}
	return s
}

func stripPrefix(raw string) string {
	s := strings.TrimSpace(raw)
	if eq := strings.IndexByte(s, '='); eq >= 0 && strings.EqualFold(strings.TrimSpace(s[:eq]), "OnCalendar") {
		return strings.TrimSpace(s[eq+1:])
	}
	return s
}
