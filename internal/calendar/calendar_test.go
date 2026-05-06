package calendar

import (
	"strings"
	"testing"
	"time"
)

func TestParseDailyAt0400(t *testing.T) {
	s, err := Parse("*-*-* 04:00:00")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	from := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	runs := s.Next(from, 3)
	if len(runs) != 3 {
		t.Fatalf("got %d runs, want 3", len(runs))
	}
	want := []time.Time{
		time.Date(2026, 5, 4, 4, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 5, 4, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 6, 4, 0, 0, 0, time.UTC),
	}
	for i, w := range want {
		if !runs[i].Equal(w) {
			t.Errorf("run[%d] = %s, want %s", i, runs[i], w)
		}
	}
	if got := s.Explain(); !strings.Contains(strings.ToLower(got), "every day at 04:00") {
		t.Errorf("Explain = %q, want it to mention 'every day at 04:00'", got)
	}
}

func TestParseWeekdayRange(t *testing.T) {
	s, err := Parse("Mon..Fri *-*-* 09:00:00")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(s.Weekdays) != 5 {
		t.Errorf("got %d weekdays, want 5", len(s.Weekdays))
	}
	from := time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC) // Saturday
	runs := s.Next(from, 1)
	if len(runs) == 0 {
		t.Fatal("no runs")
	}
	if runs[0].Weekday() != time.Monday {
		t.Errorf("first weekday run = %s, want Monday", runs[0].Weekday())
	}
}

func TestAliases(t *testing.T) {
	cases := []struct {
		name string
		exp  string
	}{
		{"hourly", "every hour"},
		{"daily", "every day"},
		{"weekly", "every monday"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, err := Parse(c.name)
			if err != nil {
				t.Fatalf("Parse(%q): %v", c.name, err)
			}
			if got := strings.ToLower(s.Explain()); !strings.Contains(got, c.exp) {
				t.Errorf("Explain(%q) = %q, want substring %q", c.name, s.Explain(), c.exp)
			}
		})
	}
}

func TestInvalidExpressionsReportErrors(t *testing.T) {
	cases := []string{
		"",
		"*-*-* 99:00:00",
		"Funday *-*-* 04:00:00",
		"*-*-* 04:60",
		"OnCalendar=",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			if err := Validate(raw); err == nil {
				t.Errorf("Validate(%q) returned nil, want error", raw)
			}
		})
	}
}

func TestEveryFiveMinutesExplanation(t *testing.T) {
	s, err := Parse("*:0/5")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := strings.ToLower(s.Explain()); !strings.Contains(got, "every 5 minutes") {
		t.Errorf("Explain = %q, want substring 'every 5 minutes'", s.Explain())
	}
}
