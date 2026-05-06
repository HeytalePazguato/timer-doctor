package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/HeytalePazguato/timer-doctor/internal/audit"
)

// Calendar renders a 7-day ASCII heatmap of timer fire times.
// Each row is a day, each column an hour; cells show the number of
// timer fires landing in that hour. Cells with two or more fires are
// listed below the grid for collision investigation.
func Calendar(w io.Writer, r *audit.Result, color bool) {
	p := newPalette(color)
	start := r.Now
	if start.IsZero() {
		start = time.Now()
	}
	day0 := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())

	// counts[day][hour] -> int; collisions[day][hour] -> []unit.
	counts := make([][24]int, 7)
	collisions := make([]map[int][]string, 7)
	for i := range collisions {
		collisions[i] = map[int][]string{}
	}

	for _, t := range r.Timers {
		if len(t.Schedules) == 0 {
			continue
		}
		// Collect every fire time that falls in the 7-day window.
		seen := map[[2]int]bool{}
		for _, sc := range t.Schedules {
			fires := sc.NextRuns
			// Compute extra fires when the schedule has its own Spec —
			// the default 3 runs aren't enough for a heatmap.
			if sc.Spec != nil {
				fires = sc.Spec.Next(day0, 7*24)
			}
			for _, fire := range fires {
				if fire.Before(day0) || !fire.Before(day0.AddDate(0, 0, 7)) {
					continue
				}
				dayIdx := int(fire.Sub(day0).Hours()) / 24
				if dayIdx < 0 || dayIdx >= 7 {
					continue
				}
				hour := fire.Hour()
				key := [2]int{dayIdx, hour}
				if seen[key] {
					continue
				}
				seen[key] = true
				counts[dayIdx][hour]++
				collisions[dayIdx][hour] = append(collisions[dayIdx][hour], t.Unit)
			}
		}
	}

	fmt.Fprintf(w, "7-day calendar starting %s\n\n", day0.Format("2006-01-02"))
	fmt.Fprint(w, "        ")
	for h := 0; h < 24; h++ {
		fmt.Fprintf(w, "%2d ", h)
	}
	fmt.Fprintln(w)
	for d := 0; d < 7; d++ {
		day := day0.AddDate(0, 0, d)
		fmt.Fprintf(w, "%s %02d  ", day.Format("Mon"), day.Day())
		for h := 0; h < 24; h++ {
			n := counts[d][h]
			cell := "."
			if n > 0 {
				cell = fmt.Sprintf("%d", n)
				if n >= 9 {
					cell = "9"
				}
			}
			switch {
			case n == 0:
				fmt.Fprintf(w, "%s%s", p.dim(cell), strings.Repeat(" ", 2))
			case n == 1:
				fmt.Fprintf(w, "%s%s", p.cyan(cell), strings.Repeat(" ", 2))
			default:
				fmt.Fprintf(w, "%s%s", p.yellow(cell), strings.Repeat(" ", 2))
			}
		}
		fmt.Fprintln(w)
	}

	// Collision section.
	type coll struct {
		day, hour int
		units     []string
	}
	var colls []coll
	for d := 0; d < 7; d++ {
		hours := make([]int, 0, len(collisions[d]))
		for h := range collisions[d] {
			hours = append(hours, h)
		}
		sort.Ints(hours)
		for _, h := range hours {
			units := collisions[d][h]
			if len(units) < 2 {
				continue
			}
			colls = append(colls, coll{d, h, units})
		}
	}
	if len(colls) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, p.bold("Collisions:"))
	for _, c := range colls {
		day := day0.AddDate(0, 0, c.day)
		fmt.Fprintf(w, "  %s %02d %02d:00 — %s\n", day.Format("Mon"), day.Day(), c.hour, strings.Join(uniqStable(c.units), ", "))
	}
}

func uniqStable(xs []string) []string {
	seen := map[string]bool{}
	out := xs[:0]
	for _, x := range xs {
		if seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	return out
}
