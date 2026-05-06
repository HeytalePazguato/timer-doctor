package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/HeytalePazguato/timer-doctor/internal/parser"
)

// runPerTimerChecks dispatches per-timer audits. Schedules are loaded
// here so each check sees the same parsed view.
func runPerTimerChecks(t *Timer, cfg Config, now time.Time) {
	loadSchedules(t, cfg, now)

	if !cfg.SkipServiceChecks {
		t.Findings = append(t.Findings, checkMissingService(t)...)
		t.Findings = append(t.Findings, checkExecStart(t)...)
		t.Findings = append(t.Findings, checkHardening(t)...)
	}
	t.Findings = append(t.Findings, checkPersistent(t)...)
	t.Findings = append(t.Findings, checkRandomizedDelay(t)...)
	t.Findings = append(t.Findings, checkNoFlock(t)...)
	t.Findings = append(t.Findings, checkAccuracy(t)...)
}

// runBatchChecks dispatches whole-batch audits.
func runBatchChecks(r *Result, cfg Config) {
	checkConflictingPairs(r, cfg)
	r.Orphans = append(r.Orphans, checkOrphans(r)...)
}

// ---------------------------------------------------------------- service

// checkMissingService verifies the paired .service unit exists and is
// parseable. Skipped on non-Unix hosts where the service path likely
// belongs to a remote target.
func checkMissingService(t *Timer) []Finding {
	if t.ServicePath == "" {
		return nil
	}
	if _, err := os.Stat(t.ServicePath); err != nil {
		// On Windows/macOS, the unit lives on a remote server — only
		// warn instead of erroring so you can still lint the timer
		// before SCP'ing it over.
		sev := SevError
		msg := fmt.Sprintf("Paired service %s not found at %s.", t.ServiceUnit, t.ServicePath)
		if runtime.GOOS != "linux" {
			sev = SevWarn
			msg = fmt.Sprintf("Paired service %s not found locally (this OS isn't Linux; the unit likely lives on a remote host).", t.ServiceUnit)
		}
		return []Finding{{Severity: sev, Code: "missing_service", Message: msg}}
	}
	if t.ServiceFile == nil {
		return []Finding{{Severity: SevError, Code: "missing_service", Message: fmt.Sprintf("Paired service %s could not be parsed.", t.ServiceUnit)}}
	}
	return nil
}

// checkExecStart inspects the paired service's ExecStart= absolute path.
// Errors if the file is missing; warns if it exists but isn't executable.
func checkExecStart(t *Timer) []Finding {
	if t.ServiceFile == nil {
		return nil
	}
	path := t.ExecStartFirstPath()
	if path == "" {
		return nil
	}
	st, err := os.Stat(path)
	if err != nil {
		return []Finding{{Severity: SevError, Code: "missing_exec_start", Message: fmt.Sprintf("Service ExecStart %s does not exist.", path)}}
	}
	if runtime.GOOS != "windows" && st.Mode().Perm()&0o111 == 0 {
		return []Finding{{Severity: SevWarn, Code: "not_executable", Message: fmt.Sprintf("Service ExecStart %s exists but is not executable.", path)}}
	}
	return nil
}

// checkHardening flags services with no hardening directives at all.
// Pure info — many simple admin scripts legitimately don't need any.
func checkHardening(t *Timer) []Finding {
	if t.ServiceFile == nil {
		return nil
	}
	sec := t.ServiceFile.Section("Service")
	if sec == nil {
		return nil
	}
	hardeningKeys := []string{"ProtectSystem", "PrivateTmp", "NoNewPrivileges", "ProtectHome", "DynamicUser", "CapabilityBoundingSet"}
	for _, k := range hardeningKeys {
		if _, ok := sec.Get(k); ok {
			return nil
		}
	}
	return []Finding{{Severity: SevInfo, Code: "no_hardening", Message: "Service has no hardening directives (ProtectSystem=, PrivateTmp=, NoNewPrivileges=)."}}
}

// ---------------------------------------------------------------- timer

// checkPersistent warns when an OnCalendar= timer doesn't set
// Persistent=true, since missed runs (e.g. during downtime) won't fire
// on the next boot otherwise.
func checkPersistent(t *Timer) []Finding {
	hasCalendar := false
	for _, sc := range t.Schedules {
		if sc.Type == "OnCalendar" && sc.ParseError == "" {
			hasCalendar = true
			break
		}
	}
	if !hasCalendar || t.Persistent {
		return nil
	}
	return []Finding{{Severity: SevWarn, Code: "no_persistent", Message: "Persistent= is not set. Runs missed during downtime will not be retried."}}
}

// checkRandomizedDelay warns when a timer fires on a popular calendar
// moment (top of hour, daily/hourly/weekly aliases, etc.) and
// RandomizedDelaySec= is unset — the classic thundering-herd risk.
func checkRandomizedDelay(t *Timer) []Finding {
	if t.RandomizedDelay {
		return nil
	}
	for _, sc := range t.Schedules {
		if sc.Type != "OnCalendar" || sc.Spec == nil {
			continue
		}
		if moment, ok := popularMoment(sc); ok {
			return []Finding{{Severity: SevWarn, Code: "no_randomized_delay", Message: fmt.Sprintf("RandomizedDelaySec= unset on a popular calendar moment (%s); risk of thundering herd.", moment), Line: sc.Line}}
		}
	}
	return nil
}

func popularMoment(sc Schedule) (string, bool) {
	raw := strings.ToLower(strings.TrimSpace(sc.Raw))
	switch raw {
	case "hourly":
		return "hourly", true
	case "daily":
		return "daily", true
	case "weekly":
		return "weekly", true
	case "monthly":
		return "monthly", true
	}
	s := sc.Spec
	if s == nil {
		return "", false
	}
	zeroSec := len(s.Seconds) == 0 || (len(s.Seconds) == 1 && s.Seconds[0] == 0)
	if !zeroSec {
		return "", false
	}
	if len(s.Minutes) == 1 && s.Minutes[0] == 0 {
		switch len(s.Hours) {
		case 1:
			return fmt.Sprintf("%02d:00", s.Hours[0]), true
		case 0:
			return "top of every hour", true
		}
	}
	return "", false
}

// checkNoFlock warns when a high-frequency timer (more often than every
// 10 minutes) pairs with a service that has no concurrency guard
// (flock, pidof, Type=oneshot).
func checkNoFlock(t *Timer) []Finding {
	if t.MinInterval() == 0 || t.MinInterval() > 10*time.Minute {
		return nil
	}
	if t.ServiceFile != nil {
		sec := t.ServiceFile.Section("Service")
		if v, ok := sec.Get("Type"); ok && strings.EqualFold(strings.TrimSpace(v), "oneshot") {
			// Type=oneshot already serializes runs.
			if rem, ok := sec.Get("RemainAfterExit"); !ok || !strings.EqualFold(strings.TrimSpace(rem), "yes") {
				return nil
			}
		}
		for _, cmd := range t.execStartCommands() {
			if mentionsConcurrencyGuard(cmd) {
				return nil
			}
		}
	}
	return []Finding{{Severity: SevWarn, Code: "no_flock", Message: "Timer fires more often than every 10 minutes; ExecStart has no flock/pidof/Type=oneshot. Concurrent runs may pile up."}}
}

func mentionsConcurrencyGuard(cmd string) bool {
	low := strings.ToLower(cmd)
	for _, needle := range []string{"flock", "pidof", "pgrep", "lockfile"} {
		if strings.Contains(low, needle) {
			return true
		}
	}
	return false
}

// checkAccuracy warns when the timer fires every minute or more often
// and AccuracySec= is unset (default is 1 minute, which causes drift on
// sub-minute schedules).
func checkAccuracy(t *Timer) []Finding {
	if t.MinInterval() == 0 || t.MinInterval() > time.Minute {
		return nil
	}
	if t.AccuracySec != "" {
		return nil
	}
	return []Finding{{Severity: SevWarn, Code: "wrong_accuracy", Message: "Default AccuracySec=1min on a sub-minute timer can cause drift; set AccuracySec=1s explicitly."}}
}

// ---------------------------------------------------------------- batch

// checkConflictingPairs flags timers in the batch that fire within the
// configured window of each other and write to similar paths.
func checkConflictingPairs(r *Result, cfg Config) {
	type fire struct {
		t       *Timer
		when    time.Time
		writeKey string
	}
	var fires []fire
	for _, t := range r.Timers {
		if len(t.Schedules) == 0 {
			continue
		}
		key := writeKey(t)
		for _, sc := range t.Schedules {
			for _, run := range sc.NextRuns {
				fires = append(fires, fire{t: t, when: run, writeKey: key})
			}
		}
	}
	for i := 0; i < len(fires); i++ {
		for j := i + 1; j < len(fires); j++ {
			a, b := fires[i], fires[j]
			if a.t == b.t {
				continue
			}
			diff := a.when.Sub(b.when)
			if diff < 0 {
				diff = -diff
			}
			if diff > cfg.ConflictWindow {
				continue
			}
			if !pathsLookSimilar(a.writeKey, b.writeKey) {
				continue
			}
			msg := fmt.Sprintf("Timer fires within %s of %s and writes to a similar path; runs may collide.", cfg.ConflictWindow, b.t.Unit)
			if !hasFinding(a.t.Findings, "conflicting_pair", b.t.Unit) {
				a.t.Findings = append(a.t.Findings, Finding{Severity: SevWarn, Code: "conflicting_pair", Message: msg})
			}
			msg2 := fmt.Sprintf("Timer fires within %s of %s and writes to a similar path; runs may collide.", cfg.ConflictWindow, a.t.Unit)
			if !hasFinding(b.t.Findings, "conflicting_pair", a.t.Unit) {
				b.t.Findings = append(b.t.Findings, Finding{Severity: SevWarn, Code: "conflicting_pair", Message: msg2})
			}
		}
	}
}

func writeKey(t *Timer) string {
	if t.ServiceFile == nil {
		return ""
	}
	sec := t.ServiceFile.Section("Service")
	if sec == nil {
		return ""
	}
	for _, kv := range sec.All("ExecStart") {
		v := strings.TrimLeft(kv.Value, "-@:+!")
		fields := strings.Fields(v)
		for _, f := range fields {
			if strings.HasPrefix(f, "/") || strings.HasPrefix(f, "./") {
				return f
			}
		}
		if len(fields) > 0 {
			return fields[0]
		}
	}
	return ""
}

func pathsLookSimilar(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	la := strings.ToLower(a)
	lb := strings.ToLower(b)
	// Same directory + same basename root (foo.sh vs foo.py).
	if filepath.Dir(la) == filepath.Dir(lb) && rootName(la) == rootName(lb) {
		return true
	}
	return false
}

func rootName(p string) string {
	base := filepath.Base(p)
	if i := strings.LastIndexByte(base, '.'); i > 0 {
		return base[:i]
	}
	return base
}

func hasFinding(fs []Finding, code, otherUnit string) bool {
	for _, f := range fs {
		if f.Code == code && strings.Contains(f.Message, otherUnit) {
			return true
		}
	}
	return false
}

// checkOrphans scans every directory that produced a parsed timer for
// .service files that look timer-driven (WantedBy=timers.target or no
// [Install] block) but have no matching .timer.
func checkOrphans(r *Result) []Finding {
	dirs := map[string]bool{}
	timerNames := map[string]map[string]bool{} // dir -> set of services declared by Unit= or default
	for _, t := range r.Timers {
		if t.Path == "" || t.TimerFile == nil {
			continue
		}
		dir := filepath.Dir(t.Path)
		dirs[dir] = true
		if timerNames[dir] == nil {
			timerNames[dir] = map[string]bool{}
		}
		timerNames[dir][t.ServiceUnit] = true
	}
	var out []Finding
	for dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".service") {
				continue
			}
			if timerNames[dir][name] {
				continue
			}
			full := filepath.Join(dir, name)
			uf, err := parser.ParseFile(full)
			if err != nil {
				continue
			}
			if !looksTimerDriven(uf) {
				continue
			}
			out = append(out, Finding{
				Severity: SevWarn,
				Code:     "orphan_service",
				Message:  fmt.Sprintf("Service %s looks timer-driven but no matching .timer was found.", name),
			})
		}
	}
	return out
}

func looksTimerDriven(uf *parser.UnitFile) bool {
	install := uf.Section("Install")
	if install != nil {
		if v, ok := install.Get("WantedBy"); ok && strings.Contains(v, "timers.target") {
			return true
		}
		// An Install section with multi-user.target is not timer-driven.
		return false
	}
	// No [Install] and a non-system-shaped name → likely meant to be
	// kicked off by a sibling .timer. Not perfect, but a reasonable hint.
	return true
}
