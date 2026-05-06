package audit

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/HeytalePazguato/timer-doctor/internal/calendar"
)

// loadSchedules pulls every OnCalendar / OnBootSec / OnStartupSec /
// OnUnitActiveSec / OnUnitInactiveSec line out of the [Timer] section.
func loadSchedules(t *Timer, cfg Config, now time.Time) {
	timerSec := t.TimerFile.Section("Timer")
	if timerSec == nil {
		return
	}
	monoTypes := []string{"OnBootSec", "OnStartupSec", "OnUnitActiveSec", "OnUnitInactiveSec", "OnActiveSec"}
	for _, kind := range monoTypes {
		for _, kv := range timerSec.All(kind) {
			t.Schedules = append(t.Schedules, Schedule{
				Type:        kind,
				Raw:         kv.Value,
				Line:        kv.Line,
				Explanation: explainMonotonic(kind, kv.Value),
			})
		}
	}
	for _, kv := range timerSec.All("OnCalendar") {
		s := Schedule{Type: "OnCalendar", Raw: kv.Value, Line: kv.Line}
		spec, err := calendar.Parse(kv.Value)
		if err != nil {
			s.ParseError = err.Error()
			t.Findings = append(t.Findings, Finding{
				Severity: SevError,
				Code:     "bad_calendar",
				Message:  fmt.Sprintf("OnCalendar=%s — %s", kv.Value, err.Error()),
				Line:     kv.Line,
			})
			t.Schedules = append(t.Schedules, s)
			continue
		}
		s.Spec = spec
		s.Explanation = spec.Explain()
		s.NextRuns = computeNext(spec, kv.Value, now, 3, cfg)
		t.Schedules = append(t.Schedules, s)
	}
}

// computeNext returns up to n future fire times for a calendar spec.
// Tries `systemd-analyze calendar --iterations=n` first when available
// (and not skipped), falls back to the built-in calculator on any
// failure.
func computeNext(spec *calendar.Spec, raw string, from time.Time, n int, cfg Config) []time.Time {
	if !cfg.SkipSystemdAnalyze {
		if ts, ok := analyzeNext(raw, n); ok {
			return ts
		}
	}
	return spec.Next(from, n)
}

func analyzeNext(raw string, n int) ([]time.Time, bool) {
	bin, err := exec.LookPath("systemd-analyze")
	if err != nil {
		return nil, false
	}
	expr := stripPrefix(raw)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "calendar", fmt.Sprintf("--iterations=%d", n), expr).Output()
	if err != nil {
		return nil, false
	}
	return parseAnalyzeOutput(string(out)), true
}

// parseAnalyzeOutput pulls "Next elapse: ..." lines out of
// `systemd-analyze calendar` output. Only "(in UTC):" lines are
// considered to keep the format stable.
func parseAnalyzeOutput(s string) []time.Time {
	var out []time.Time
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		const prefix = "Next elapse: "
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		ts := strings.TrimSpace(line[len(prefix):])
		// Format: "Mon 2026-05-04 04:00:00 UTC".
		fields := strings.Fields(ts)
		if len(fields) < 4 {
			continue
		}
		t, err := time.Parse("2006-01-02 15:04:05 MST", fields[1]+" "+fields[2]+" "+fields[3])
		if err != nil {
			continue
		}
		out = append(out, t.UTC())
	}
	return out
}

func explainMonotonic(kind, raw string) string {
	d, err := parseSystemdDuration(raw)
	if err != nil {
		return raw
	}
	switch kind {
	case "OnBootSec":
		return fmt.Sprintf("%s after boot", durationWords(d))
	case "OnStartupSec":
		return fmt.Sprintf("%s after systemd start", durationWords(d))
	case "OnUnitActiveSec":
		return fmt.Sprintf("%s after the unit was last activated", durationWords(d))
	case "OnUnitInactiveSec":
		return fmt.Sprintf("%s after the unit was last deactivated", durationWords(d))
	case "OnActiveSec":
		return fmt.Sprintf("%s after the timer activates", durationWords(d))
	}
	return raw
}

// parseSystemdDuration accepts the systemd time-span syntax as documented
// in `systemd.time(7)`: "1h", "30m", "1h30m", "1d 6h", bare seconds, etc.
func parseSystemdDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	units := map[string]time.Duration{
		"us": time.Microsecond, "µs": time.Microsecond, "usec": time.Microsecond,
		"ms": time.Millisecond, "msec": time.Millisecond,
		"s": time.Second, "sec": time.Second, "second": time.Second, "seconds": time.Second,
		"m": time.Minute, "min": time.Minute, "minute": time.Minute, "minutes": time.Minute,
		"h": time.Hour, "hr": time.Hour, "hour": time.Hour, "hours": time.Hour,
		"d": 24 * time.Hour, "day": 24 * time.Hour, "days": 24 * time.Hour,
		"w": 7 * 24 * time.Hour, "week": 7 * 24 * time.Hour, "weeks": 7 * 24 * time.Hour,
	}
	var total time.Duration
	cur := strings.ReplaceAll(s, " ", "")
	for len(cur) > 0 {
		// Number portion.
		i := 0
		for i < len(cur) && (cur[i] == '.' || (cur[i] >= '0' && cur[i] <= '9')) {
			i++
		}
		if i == 0 {
			return 0, fmt.Errorf("no number at %q", cur)
		}
		num := cur[:i]
		cur = cur[i:]
		// Unit portion.
		j := 0
		for j < len(cur) && ((cur[j] >= 'a' && cur[j] <= 'z') || (cur[j] >= 'A' && cur[j] <= 'Z') || cur[j] == 'µ') {
			j++
		}
		unit := strings.ToLower(cur[:j])
		cur = cur[j:]
		if unit == "" {
			unit = "s"
		}
		mul, ok := units[unit]
		if !ok {
			return 0, fmt.Errorf("unknown unit %q", unit)
		}
		// Use float intermediate so "0.5h" works.
		var f float64
		_, err := fmt.Sscanf(num, "%f", &f)
		if err != nil {
			return 0, err
		}
		total += time.Duration(f * float64(mul))
	}
	return total, nil
}

func durationWords(d time.Duration) string {
	if d <= 0 {
		return "immediately"
	}
	parts := []string{}
	weeks := d / (7 * 24 * time.Hour)
	d -= weeks * 7 * 24 * time.Hour
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	mins := d / time.Minute
	d -= mins * time.Minute
	secs := d / time.Second
	if weeks > 0 {
		parts = append(parts, plural(int(weeks), "week", "weeks"))
	}
	if days > 0 {
		parts = append(parts, plural(int(days), "day", "days"))
	}
	if hours > 0 {
		parts = append(parts, plural(int(hours), "hour", "hours"))
	}
	if mins > 0 {
		parts = append(parts, plural(int(mins), "minute", "minutes"))
	}
	if secs > 0 || len(parts) == 0 {
		parts = append(parts, plural(int(secs), "second", "seconds"))
	}
	return strings.Join(parts, " ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// MinInterval returns the shortest interval between consecutive fire
// times across all of a timer's schedules, looking at the next 5 fires.
// Returns 0 when no calendar fire times are available.
func (t *Timer) MinInterval() time.Duration {
	var min time.Duration
	for _, sc := range t.Schedules {
		if sc.Spec == nil || len(sc.NextRuns) < 2 {
			continue
		}
		for i := 1; i < len(sc.NextRuns); i++ {
			d := sc.NextRuns[i].Sub(sc.NextRuns[i-1])
			if d > 0 && (min == 0 || d < min) {
				min = d
			}
		}
	}
	return min
}

// firstFire returns the earliest of the timer's next-run times across
// all of its schedules, or zero if none are computed.
func (t *Timer) firstFire() time.Time {
	var first time.Time
	for _, sc := range t.Schedules {
		for _, r := range sc.NextRuns {
			if first.IsZero() || r.Before(first) {
				first = r
			}
		}
	}
	return first
}

// execStartCommands returns every ExecStart= command from the paired
// service's [Service] section. Empty when the service is unparseable.
func (t *Timer) execStartCommands() []string {
	if t.ServiceFile == nil {
		return nil
	}
	sec := t.ServiceFile.Section("Service")
	if sec == nil {
		return nil
	}
	var out []string
	for _, kv := range sec.All("ExecStart") {
		v := kv.Value
		// Strip leading systemd prefix characters: -, @, :, +, !, !!.
		v = strings.TrimLeft(v, "-@:+!")
		out = append(out, strings.TrimSpace(v))
	}
	return out
}

// ExecStartFirstPath returns the first absolute path token across the
// paired service's ExecStart= lines, or "" when none exists.
func (t *Timer) ExecStartFirstPath() string {
	for _, cmd := range t.execStartCommands() {
		fields := strings.Fields(cmd)
		if len(fields) > 0 && strings.HasPrefix(fields[0], "/") {
			return fields[0]
		}
	}
	return ""
}

