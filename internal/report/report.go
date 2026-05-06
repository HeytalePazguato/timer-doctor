// Package report renders audit results as text, JSON, or a 7-day
// calendar heatmap. ANSI color is added when the writer is a terminal;
// piped output gets clean plain text.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/HeytalePazguato/timer-doctor/internal/audit"
)

// IsTerminal returns true when the writer is a TTY. Used to decide
// whether to emit ANSI escapes.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return (st.Mode() & os.ModeCharDevice) != 0
}

// Palette wraps strings in ANSI escapes when color is enabled.
type Palette struct{ enabled bool }

func newPalette(on bool) Palette { return Palette{enabled: on} }

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiGreen  = "\x1b[32m"
	ansiCyan   = "\x1b[36m"
	ansiBlue   = "\x1b[34m"
	ansiMag    = "\x1b[35m"
)

func (p Palette) wrap(code, s string) string {
	if !p.enabled {
		return s
	}
	return code + s + ansiReset
}

func (p Palette) bold(s string) string   { return p.wrap(ansiBold, s) }
func (p Palette) dim(s string) string    { return p.wrap(ansiDim, s) }
func (p Palette) red(s string) string    { return p.wrap(ansiRed, s) }
func (p Palette) yellow(s string) string { return p.wrap(ansiYellow, s) }
func (p Palette) green(s string) string  { return p.wrap(ansiGreen, s) }
func (p Palette) cyan(s string) string   { return p.wrap(ansiCyan, s) }

// Text renders a batch result as the default human-readable report.
func Text(w io.Writer, r *audit.Result, color bool) {
	p := newPalette(color)
	for i, t := range r.Timers {
		if i > 0 {
			fmt.Fprintln(w)
		}
		writeTimer(w, p, t)
	}
	if len(r.Orphans) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, p.bold("Orphan services:"))
		for _, f := range r.Orphans {
			fmt.Fprintf(w, "  %s %s\n", severityLabel(p, f.Severity), f.Message)
		}
	}
	fmt.Fprintln(w)
	writeSummary(w, p, audit.Summarize(r))
}

func writeTimer(w io.Writer, p Palette, t *audit.Timer) {
	fmt.Fprintln(w, p.bold(t.Unit))
	if t.ServiceUnit != "" {
		fmt.Fprintf(w, "  Service: %s\n", t.ServiceUnit)
	}
	for _, sc := range t.Schedules {
		writeSchedule(w, p, sc, "  ")
	}
	for _, f := range t.Findings {
		fmt.Fprintf(w, "  %s %s\n", severityLabel(p, f.Severity), f.Message)
	}
	// "Service ExecStart exists and is executable" success line, only
	// when no service-level errors were emitted.
	if t.ServiceFile != nil && !hasFindingCode(t.Findings, "missing_exec_start", "not_executable", "missing_service") {
		if path := t.ExecStartFirstPath(); path != "" {
			fmt.Fprintf(w, "  %s Service ExecStart %s exists and is executable.\n", p.green("✓"), path)
		}
	}
}

func writeSchedule(w io.Writer, p Palette, sc audit.Schedule, indent string) {
	header := fmt.Sprintf("%sSchedule (line %d): %s=%s", indent, sc.Line, sc.Type, sc.Raw)
	fmt.Fprintln(w, header)
	if sc.ParseError != "" {
		fmt.Fprintf(w, "%s  %s parse error: %s\n", indent, p.red("✗"), sc.ParseError)
		return
	}
	if sc.Explanation != "" {
		fmt.Fprintf(w, "%s  %s\n", indent, sc.Explanation+".")
	}
	if len(sc.NextRuns) > 0 {
		runs := make([]string, len(sc.NextRuns))
		for i, t := range sc.NextRuns {
			runs[i] = t.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "%s  Next: %s.\n", indent, strings.Join(runs, ", "))
	}
}

// Expression renders the schedule-only output for a bare OnCalendar input.
func Expression(w io.Writer, t *audit.Timer, color bool) {
	p := newPalette(color)
	for _, sc := range t.Schedules {
		fmt.Fprintf(w, "%s %s\n", p.bold("Schedule:"), sc.Raw)
		if sc.ParseError != "" {
			fmt.Fprintf(w, "  %s %s\n", p.red("✗"), sc.ParseError)
			continue
		}
		if sc.Explanation != "" {
			fmt.Fprintf(w, "  %s\n", sc.Explanation+".")
		}
		if len(sc.NextRuns) > 0 {
			runs := make([]string, len(sc.NextRuns))
			for i, tm := range sc.NextRuns {
				runs[i] = tm.Format("2006-01-02 15:04")
			}
			fmt.Fprintf(w, "  Next: %s.\n", strings.Join(runs, ", "))
		}
	}
	for _, f := range t.Findings {
		fmt.Fprintf(w, "  %s %s\n", severityLabel(p, f.Severity), f.Message)
	}
}

func writeSummary(w io.Writer, p Palette, s audit.Summary) {
	plural := func(n int, one, many string) string {
		if n == 1 {
			return fmt.Sprintf("%d %s", n, one)
		}
		return fmt.Sprintf("%d %s", n, many)
	}
	fmt.Fprintf(w, "%s — %s, %s, %s.\n",
		plural(s.Timers, "timer audited", "timers audited"),
		colored(p, s.Errors, "error"),
		colored(p, s.Warnings, "warning"),
		colored(p, s.Info, "info note"),
	)
}

func colored(p Palette, n int, label string) string {
	plural := label + "s"
	if n == 1 {
		plural = label
	}
	if n == 0 {
		return fmt.Sprintf("%d %s", n, plural)
	}
	switch label {
	case "error":
		return p.red(fmt.Sprintf("%d %s", n, plural))
	case "warning":
		return p.yellow(fmt.Sprintf("%d %s", n, plural))
	}
	return fmt.Sprintf("%d %s", n, plural)
}

func severityLabel(p Palette, s audit.Severity) string {
	switch s {
	case audit.SevError:
		return p.red("✗ ERROR:")
	case audit.SevWarn:
		return p.yellow("⚠ WARN:")
	case audit.SevInfo:
		return p.cyan("ⓘ INFO:")
	}
	return string(s)
}

func hasFindingCode(fs []audit.Finding, codes ...string) bool {
	for _, f := range fs {
		for _, c := range codes {
			if f.Code == c {
				return true
			}
		}
	}
	return false
}

// JSON output structures.

type jsonSchedule struct {
	Type        string   `json:"type"`
	Raw         string   `json:"raw"`
	Line        int      `json:"line,omitempty"`
	Explanation string   `json:"explanation,omitempty"`
	NextRuns    []string `json:"next_runs,omitempty"`
	ParseError  string   `json:"parse_error,omitempty"`
}

type jsonTimer struct {
	Path      string          `json:"path,omitempty"`
	Unit      string          `json:"unit"`
	Service   string          `json:"service,omitempty"`
	Schedules []jsonSchedule  `json:"schedules"`
	Findings  []audit.Finding `json:"findings"`
}

type jsonOutput struct {
	Timers   []jsonTimer     `json:"timers"`
	Orphans  []audit.Finding `json:"orphans,omitempty"`
	Summary  audit.Summary   `json:"summary"`
}

// JSON writes the full result set as machine-readable JSON.
func JSON(w io.Writer, r *audit.Result) error {
	out := jsonOutput{Summary: audit.Summarize(r)}
	for _, t := range r.Timers {
		jt := jsonTimer{
			Path:     t.Path,
			Unit:     t.Unit,
			Service:  t.ServiceUnit,
			Findings: append([]audit.Finding(nil), t.Findings...),
		}
		for _, sc := range t.Schedules {
			js := jsonSchedule{
				Type:        sc.Type,
				Raw:         sc.Raw,
				Line:        sc.Line,
				Explanation: sc.Explanation,
				ParseError:  sc.ParseError,
			}
			for _, run := range sc.NextRuns {
				js.NextRuns = append(js.NextRuns, run.UTC().Format(time.RFC3339))
			}
			jt.Schedules = append(jt.Schedules, js)
		}
		if jt.Schedules == nil {
			jt.Schedules = []jsonSchedule{}
		}
		if jt.Findings == nil {
			jt.Findings = []audit.Finding{}
		}
		out.Timers = append(out.Timers, jt)
	}
	if out.Timers == nil {
		out.Timers = []jsonTimer{}
	}
	if len(r.Orphans) > 0 {
		out.Orphans = r.Orphans
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// JSONExpression writes a single schedule-only result.
func JSONExpression(w io.Writer, t *audit.Timer) error {
	r := &audit.Result{Timers: []*audit.Timer{t}}
	return JSON(w, r)
}
