package parser

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseUnitFile(t *testing.T) {
	uf, err := ParseFile(filepath.Join("..", "..", "testdata", "clean", "backup.timer"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if got := uf.Section("Timer"); got == nil {
		t.Fatalf("missing [Timer] section")
	}
	cal, ok := uf.Section("Timer").Get("OnCalendar")
	if !ok {
		t.Fatalf("missing OnCalendar")
	}
	if cal != "*-*-* 04:00:00" {
		t.Errorf("OnCalendar = %q, want %q", cal, "*-*-* 04:00:00")
	}
	if got, _ := uf.Section("Timer").Get("Persistent"); got != "true" {
		t.Errorf("Persistent = %q, want true", got)
	}
}

func TestParseMultipleOnCalendar(t *testing.T) {
	src := strings.NewReader("[Timer]\nOnCalendar=daily\nOnCalendar=hourly\nPersistent=true\n")
	uf, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	all := uf.Section("Timer").All("OnCalendar")
	if len(all) != 2 {
		t.Fatalf("got %d OnCalendar lines, want 2", len(all))
	}
	if all[0].Value != "daily" || all[1].Value != "hourly" {
		t.Errorf("values = %v, want [daily hourly]", []string{all[0].Value, all[1].Value})
	}
}

func TestParseMalformed(t *testing.T) {
	src := strings.NewReader("not-a-section\n[Timer]\nOnCalendar=daily\nbroken line with no equals\n")
	uf, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(uf.ParseErrors) != 2 {
		t.Errorf("ParseErrors = %d, want 2 (got %v)", len(uf.ParseErrors), uf.ParseErrors)
	}
}

func TestPairedServicePath(t *testing.T) {
	uf, _ := ParseFile(filepath.Join("..", "..", "testdata", "clean", "backup.timer"))
	got := PairedServicePath(filepath.Join("..", "..", "testdata", "clean", "backup.timer"), uf)
	want := filepath.Join("..", "..", "testdata", "clean", "backup.service")
	if got != want {
		t.Errorf("PairedServicePath = %q, want %q", got, want)
	}
}
