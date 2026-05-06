// Package calendar parses systemd OnCalendar= expressions and computes
// their next fire times. It prefers `systemd-analyze calendar` when the
// binary is on $PATH and falls back to a built-in parser otherwise so
// timer-doctor stays useful on macOS and Windows.
//
// The grammar implemented here is the practical subset that matters for
// linting: weekday lists, year ranges, month/day/hour/minute/second
// fields with `*`, ranges (`a..b`), step values (`*/n`), comma lists,
// and the named shortcuts (`minutely`, `hourly`, `daily`, `weekly`,
// `monthly`, `yearly`/`annually`).
package calendar

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Spec is a parsed OnCalendar= expression.
type Spec struct {
	Raw      string
	Weekdays []time.Weekday // empty = all
	Years    []int          // empty = every year
	Months   []int          // 1..12, empty = all
	Days     []int          // 1..31, empty = all
	Hours    []int          // 0..23, empty = all
	Minutes  []int          // 0..59, empty = all
	Seconds  []int          // 0..59, empty = all
}

// Parse turns an OnCalendar= value into a Spec. The leading
// "OnCalendar=" prefix is accepted but optional.
func Parse(raw string) (*Spec, error) {
	s := strings.TrimSpace(raw)
	if eq := strings.IndexByte(s, '='); eq >= 0 && strings.EqualFold(strings.TrimSpace(s[:eq]), "OnCalendar") {
		s = strings.TrimSpace(s[eq+1:])
	}
	if s == "" {
		return nil, fmt.Errorf("empty calendar expression")
	}
	if alias, ok := aliasSpec(s); ok {
		alias.Raw = raw
		return alias, nil
	}

	spec := &Spec{Raw: raw}
	parts := strings.Fields(s)

	// Optional weekday prefix.
	if len(parts) > 0 && looksLikeWeekday(parts[0]) {
		wds, err := parseWeekdays(parts[0])
		if err != nil {
			return nil, err
		}
		spec.Weekdays = wds
		parts = parts[1:]
	}

	// Date and time portions.
	var datePart, timePart string
	switch len(parts) {
	case 0:
		return nil, fmt.Errorf("missing time")
	case 1:
		// Either a time-only or a date-only value.
		if strings.ContainsRune(parts[0], ':') {
			timePart = parts[0]
		} else if strings.ContainsRune(parts[0], '-') {
			datePart = parts[0]
		} else {
			return nil, fmt.Errorf("expected DATE TIME, got %q", parts[0])
		}
	case 2:
		datePart, timePart = parts[0], parts[1]
	default:
		return nil, fmt.Errorf("unexpected token %q", parts[2])
	}

	if datePart != "" {
		if err := parseDate(datePart, spec); err != nil {
			return nil, err
		}
	}
	if timePart != "" {
		if err := parseTime(timePart, spec); err != nil {
			return nil, err
		}
	}
	return spec, nil
}

// Validate checks the expression parses without producing a Spec.
func Validate(raw string) error {
	_, err := Parse(raw)
	return err
}

// Next returns up to n future fire times after `from`. Iterates minute by
// minute (or second by second when the spec pins a non-zero second) up to
// a hard cap so a pathological spec can't loop forever.
func (s *Spec) Next(from time.Time, n int) []time.Time {
	if n <= 0 {
		return nil
	}
	loc := from.Location()
	if loc == nil {
		loc = time.UTC
	}
	// Step granularity: 1s if any specific second is requested, else 1m.
	step := time.Minute
	if len(s.Seconds) > 0 && !equalsZero(s.Seconds) {
		step = time.Second
	}
	cur := from.Truncate(step).Add(step)
	out := make([]time.Time, 0, n)
	// Hard iteration cap: two years of minutes — enough to land any
	// reasonable annual schedule without runaway.
	maxIter := int(2 * 365 * 24 * 60)
	if step == time.Second {
		maxIter = int(2 * 31 * 24 * 60 * 60)
	}
	for i := 0; i < maxIter && len(out) < n; i++ {
		if s.matches(cur) {
			out = append(out, cur)
		}
		cur = cur.Add(step)
	}
	return out
}

func (s *Spec) matches(t time.Time) bool {
	if len(s.Weekdays) > 0 && !inWeekdays(s.Weekdays, t.Weekday()) {
		return false
	}
	if len(s.Years) > 0 && !inInts(s.Years, t.Year()) {
		return false
	}
	if len(s.Months) > 0 && !inInts(s.Months, int(t.Month())) {
		return false
	}
	if len(s.Days) > 0 && !inInts(s.Days, t.Day()) {
		return false
	}
	if len(s.Hours) > 0 && !inInts(s.Hours, t.Hour()) {
		return false
	}
	if len(s.Minutes) > 0 && !inInts(s.Minutes, t.Minute()) {
		return false
	}
	if len(s.Seconds) > 0 && !inInts(s.Seconds, t.Second()) {
		return false
	} else if len(s.Seconds) == 0 && t.Second() != 0 {
		// Default seconds = 0 when unset, matching systemd's behavior.
		return false
	}
	return true
}

// ---------------- aliases ----------------

func aliasSpec(s string) (*Spec, bool) {
	switch strings.ToLower(s) {
	case "minutely":
		return &Spec{Seconds: []int{0}, Minutes: allMinutes()}, true
	case "hourly":
		return &Spec{Minutes: []int{0}, Seconds: []int{0}}, true
	case "daily":
		return &Spec{Hours: []int{0}, Minutes: []int{0}, Seconds: []int{0}}, true
	case "weekly":
		return &Spec{Weekdays: []time.Weekday{time.Monday}, Hours: []int{0}, Minutes: []int{0}, Seconds: []int{0}}, true
	case "monthly":
		return &Spec{Days: []int{1}, Hours: []int{0}, Minutes: []int{0}, Seconds: []int{0}}, true
	case "yearly", "annually":
		return &Spec{Months: []int{1}, Days: []int{1}, Hours: []int{0}, Minutes: []int{0}, Seconds: []int{0}}, true
	case "quarterly":
		return &Spec{Months: []int{1, 4, 7, 10}, Days: []int{1}, Hours: []int{0}, Minutes: []int{0}, Seconds: []int{0}}, true
	case "semiannually", "semi-annually":
		return &Spec{Months: []int{1, 7}, Days: []int{1}, Hours: []int{0}, Minutes: []int{0}, Seconds: []int{0}}, true
	}
	return nil, false
}

func allMinutes() []int {
	out := make([]int, 60)
	for i := range out {
		out[i] = i
	}
	return out
}

// ---------------- weekdays ----------------

func looksLikeWeekday(tok string) bool {
	tok = strings.ToLower(strings.TrimSuffix(tok, ","))
	for _, segment := range strings.Split(tok, ",") {
		segment = strings.TrimSpace(segment)
		segment = strings.SplitN(segment, "..", 2)[0]
		if _, ok := weekdayMap[segment]; !ok {
			return false
		}
	}
	return true
}

var weekdayMap = map[string]time.Weekday{
	"sun": time.Sunday, "sunday": time.Sunday,
	"mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tuesday": time.Tuesday,
	"wed": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thursday": time.Thursday,
	"fri": time.Friday, "friday": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday,
}

var weekdayNames = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

func parseWeekdays(tok string) ([]time.Weekday, error) {
	seen := map[time.Weekday]bool{}
	for _, seg := range strings.Split(tok, ",") {
		seg = strings.ToLower(strings.TrimSpace(seg))
		if seg == "" {
			continue
		}
		if strings.Contains(seg, "..") {
			ab := strings.SplitN(seg, "..", 2)
			a, ok1 := weekdayMap[ab[0]]
			b, ok2 := weekdayMap[ab[1]]
			if !ok1 || !ok2 {
				return nil, fmt.Errorf("unknown weekday in range %q", seg)
			}
			i := int(a)
			for {
				seen[time.Weekday(i)] = true
				if time.Weekday(i) == b {
					break
				}
				i = (i + 1) % 7
			}
			continue
		}
		w, ok := weekdayMap[seg]
		if !ok {
			return nil, fmt.Errorf("unknown weekday %q", seg)
		}
		seen[w] = true
	}
	out := make([]time.Weekday, 0, len(seen))
	for w := range seen {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func inWeekdays(ws []time.Weekday, w time.Weekday) bool {
	for _, x := range ws {
		if x == w {
			return true
		}
	}
	return false
}

// ---------------- date ----------------

func parseDate(s string, spec *Spec) error {
	parts := strings.Split(s, "-")
	if len(parts) != 3 {
		return fmt.Errorf("expected YYYY-MM-DD, got %q", s)
	}
	var err error
	spec.Years, err = parseField(parts[0], 1970, 2199)
	if err != nil {
		return fmt.Errorf("year: %w", err)
	}
	spec.Months, err = parseField(parts[1], 1, 12)
	if err != nil {
		return fmt.Errorf("month: %w", err)
	}
	spec.Days, err = parseField(parts[2], 1, 31)
	if err != nil {
		return fmt.Errorf("day: %w", err)
	}
	return nil
}

func parseTime(s string, spec *Spec) error {
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return fmt.Errorf("expected HH:MM[:SS], got %q", s)
	}
	var err error
	spec.Hours, err = parseField(parts[0], 0, 23)
	if err != nil {
		return fmt.Errorf("hour: %w", err)
	}
	spec.Minutes, err = parseField(parts[1], 0, 59)
	if err != nil {
		return fmt.Errorf("minute: %w", err)
	}
	if len(parts) == 3 {
		spec.Seconds, err = parseField(parts[2], 0, 59)
		if err != nil {
			return fmt.Errorf("second: %w", err)
		}
	} else {
		spec.Seconds = []int{0}
	}
	return nil
}

// parseField turns a systemd time-field token into a sorted []int.
// Empty result means "any value in [lo, hi]".
func parseField(tok string, lo, hi int) ([]int, error) {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return nil, fmt.Errorf("empty field")
	}
	seen := map[int]bool{}
	for _, seg := range strings.Split(tok, ",") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			return nil, fmt.Errorf("empty list element")
		}
		step := 1
		hasStep := false
		if i := strings.Index(seg, "/"); i >= 0 {
			n, err := strconv.Atoi(seg[i+1:])
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("bad step %q", seg[i+1:])
			}
			step = n
			hasStep = true
			seg = seg[:i]
		}
		var a, b int
		switch {
		case seg == "*":
			a, b = lo, hi
		case strings.Contains(seg, ".."):
			ab := strings.SplitN(seg, "..", 2)
			ai, err := strconv.Atoi(ab[0])
			if err != nil {
				return nil, fmt.Errorf("bad number %q", ab[0])
			}
			bi, err := strconv.Atoi(ab[1])
			if err != nil {
				return nil, fmt.Errorf("bad number %q", ab[1])
			}
			a, b = ai, bi
		default:
			n, err := strconv.Atoi(seg)
			if err != nil {
				return nil, fmt.Errorf("bad number %q", seg)
			}
			// "N/step" means "starting at N, step through the rest of
			// the field's range" — same convention as cron's `0/5`.
			if hasStep {
				a, b = n, hi
			} else {
				a, b = n, n
			}
		}
		if a < lo || b > hi || a > b {
			return nil, fmt.Errorf("value out of range [%d,%d]: %d..%d", lo, hi, a, b)
		}
		for v := a; v <= b; v += step {
			seen[v] = true
		}
	}
	if isFullRange(seen, lo, hi) {
		return nil, nil
	}
	out := make([]int, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Ints(out)
	return out, nil
}

func isFullRange(seen map[int]bool, lo, hi int) bool {
	if len(seen) != hi-lo+1 {
		return false
	}
	for v := lo; v <= hi; v++ {
		if !seen[v] {
			return false
		}
	}
	return true
}

func inInts(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func equalsZero(xs []int) bool { return len(xs) == 1 && xs[0] == 0 }

// Explain returns a plain-English description of the schedule. Best-effort:
// covers the common shapes (every-N, daily-at, weekday-at, monthly-on)
// and falls back to a literal echo for esoteric forms.
func (s *Spec) Explain() string {
	if e, ok := explainAlias(s); ok {
		return e
	}
	timePart := explainTime(s)
	datePart := explainDate(s)
	weekday := explainWeekdays(s.Weekdays)

	// When the time part already reads as a frequency ("every minute",
	// "every 5 minutes"), prepending "at " mangles it. Drop the "at"
	// and the boilerplate "Every day" wrapper in that case.
	timeIsFreq := strings.HasPrefix(timePart, "every")
	switch {
	case weekday != "" && datePart == "" && timePart != "":
		if timeIsFreq {
			return capitalize(weekday + ", " + timePart)
		}
		return capitalize(weekday + " at " + timePart)
	case weekday != "" && datePart != "" && timePart != "":
		return capitalize(weekday + " " + datePart + " at " + timePart)
	case weekday != "" && timePart == "":
		return capitalize(weekday)
	case datePart == "any day" && timePart != "":
		if timeIsFreq {
			return capitalize(timePart)
		}
		return "Every day at " + timePart
	case datePart != "" && timePart != "":
		return capitalize(datePart + " at " + timePart)
	case timePart != "":
		if timeIsFreq {
			return capitalize(timePart)
		}
		return "At " + timePart
	}
	return s.Raw
}

func explainAlias(s *Spec) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(s.Raw, "OnCalendar"), "="))) {
	case "minutely":
		return "Every minute", true
	case "hourly":
		return "Every hour at :00", true
	case "daily":
		return "Every day at 00:00", true
	case "weekly":
		return "Every Monday at 00:00", true
	case "monthly":
		return "First day of every month at 00:00", true
	case "yearly", "annually":
		return "January 1 at 00:00", true
	case "quarterly":
		return "First day of Jan, Apr, Jul, Oct at 00:00", true
	}
	return "", false
}

func explainTime(s *Spec) string {
	// Every minute at second 0 with all hours/minutes wildcard.
	allHours := len(s.Hours) == 0
	allMinutes := len(s.Minutes) == 0
	zeroSec := len(s.Seconds) == 0 || (len(s.Seconds) == 1 && s.Seconds[0] == 0)

	if allHours && allMinutes && zeroSec {
		return "every minute"
	}
	if allHours && len(s.Minutes) == 1 && s.Minutes[0] == 0 && zeroSec {
		return "every hour at :00"
	}
	// Hour wildcard with explicit minute step: "every N minutes".
	if allHours && len(s.Minutes) > 1 && zeroSec {
		if step, ok := uniformStep(s.Minutes, 60); ok && step > 1 {
			return fmt.Sprintf("every %d minutes", step)
		}
	}
	// Specific hour, specific minute.
	if len(s.Hours) == 1 && len(s.Minutes) == 1 && zeroSec {
		return fmt.Sprintf("%02d:%02d", s.Hours[0], s.Minutes[0])
	}
	// Every N hours at :00.
	if len(s.Minutes) == 1 && s.Minutes[0] == 0 && zeroSec && len(s.Hours) > 1 {
		if step, ok := uniformStep(s.Hours, 24); ok && step > 1 {
			return fmt.Sprintf("every %d hours at :00", step)
		}
	}
	if len(s.Hours) == 1 && len(s.Minutes) == 1 {
		return fmt.Sprintf("%02d:%02d", s.Hours[0], s.Minutes[0])
	}
	return ""
}

func explainDate(s *Spec) string {
	if len(s.Years) == 0 && len(s.Months) == 0 && len(s.Days) == 0 {
		return "any day"
	}
	parts := []string{}
	if len(s.Months) > 0 {
		parts = append(parts, "in "+joinMonths(s.Months))
	}
	if len(s.Days) > 0 {
		parts = append(parts, "on day "+joinInts(s.Days))
	}
	if len(s.Years) > 0 {
		parts = append(parts, "in "+joinInts(s.Years))
	}
	return strings.Join(parts, " ")
}

func explainWeekdays(ws []time.Weekday) string {
	if len(ws) == 0 {
		return ""
	}
	if len(ws) == 5 && ws[0] == time.Monday && ws[4] == time.Friday {
		return "weekdays"
	}
	if len(ws) == 2 && ws[0] == time.Saturday && ws[1] == time.Sunday {
		return "weekends"
	}
	if len(ws) == 7 {
		return "every day"
	}
	if len(ws) == 1 {
		return "every " + longWeekday(ws[0])
	}
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = weekdayNames[w]
	}
	return strings.Join(out, ", ")
}

func longWeekday(w time.Weekday) string {
	switch w {
	case time.Sunday:
		return "Sunday"
	case time.Monday:
		return "Monday"
	case time.Tuesday:
		return "Tuesday"
	case time.Wednesday:
		return "Wednesday"
	case time.Thursday:
		return "Thursday"
	case time.Friday:
		return "Friday"
	case time.Saturday:
		return "Saturday"
	}
	return w.String()
}

func uniformStep(xs []int, mod int) (int, bool) {
	if len(xs) < 2 {
		return 0, false
	}
	step := xs[1] - xs[0]
	if step <= 0 {
		return 0, false
	}
	if mod%step != 0 {
		return 0, false
	}
	for i := 2; i < len(xs); i++ {
		if xs[i]-xs[i-1] != step {
			return 0, false
		}
	}
	return step, true
}

func joinInts(xs []int) string {
	out := make([]string, len(xs))
	for i, v := range xs {
		out[i] = strconv.Itoa(v)
	}
	return strings.Join(out, ", ")
}

var monthNames = []string{"", "January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}

func joinMonths(xs []int) string {
	out := make([]string, len(xs))
	for i, v := range xs {
		if v >= 1 && v <= 12 {
			out[i] = monthNames[v]
		} else {
			out[i] = strconv.Itoa(v)
		}
	}
	return strings.Join(out, ", ")
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
