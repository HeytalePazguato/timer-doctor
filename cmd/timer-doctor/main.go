// Command timer-doctor audits systemd .timer units and prints a
// human-readable report.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/HeytalePazguato/timer-doctor/internal/audit"
	"github.com/HeytalePazguato/timer-doctor/internal/report"
)

const usage = `timer-doctor — audit systemd .timer units and OnCalendar expressions.

Usage:
  timer-doctor <path-to-timer>          Audit a single .timer file.
  timer-doctor <directory>              Audit every .timer file in a directory.
  timer-doctor --system                 Audit /etc/systemd/system + /usr/lib/systemd/system.
  timer-doctor --user                   Audit ~/.config/systemd/user + /usr/lib/systemd/user.
  timer-doctor "OnCalendar=<expr>"      Audit a single calendar expression.
  timer-doctor "*-*-* 04:00:00"         Audit a bare calendar expression.

Flags:
  --system      Scan system timer search paths.
  --user        Scan user timer search paths.
  --calendar    Print a 7-day calendar view of fire times.
  --json        Emit machine-readable JSON.
  --no-color    Disable ANSI colors even on a TTY.
  --version     Print version info and exit.
`

// Stamped at build time via -ldflags. "dev" is the default for plain
// `go build` / `go install`.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("timer-doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	system := fs.Bool("system", false, "scan system timer paths")
	user := fs.Bool("user", false, "scan user timer paths")
	cal := fs.Bool("calendar", false, "print a 7-day calendar view")
	jsonOut := fs.Bool("json", false, "emit JSON output")
	noColor := fs.Bool("no-color", false, "disable ANSI colors")
	showVersion := fs.Bool("version", false, "print version info and exit")
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Fprintln(stdout, versionString())
		return nil
	}
	pos := fs.Args()
	useColor := !*noColor && report.IsTerminal(stdout)

	switch {
	case *system:
		paths, err := collectTimers(systemPaths())
		if err != nil {
			return err
		}
		return emitBatch(stdout, paths, *jsonOut, *cal, useColor)
	case *user:
		paths, err := collectTimers(userPaths())
		if err != nil {
			return err
		}
		return emitBatch(stdout, paths, *jsonOut, *cal, useColor)
	}

	if len(pos) == 0 {
		fmt.Fprint(stderr, usage)
		return fmt.Errorf("no input")
	}

	first := strings.TrimSpace(strings.Join(pos, " "))
	if isExistingDir(pos[0]) {
		paths, err := collectTimers([]string{pos[0]})
		if err != nil {
			return err
		}
		return emitBatch(stdout, paths, *jsonOut, *cal, useColor)
	}
	if isExistingFile(pos[0]) && strings.HasSuffix(strings.ToLower(pos[0]), ".timer") {
		return emitBatch(stdout, []string{pos[0]}, *jsonOut, *cal, useColor)
	}
	// Inline calendar expression.
	t := audit.AuditExpression(first, audit.Config{})
	if *jsonOut {
		return report.JSONExpression(stdout, t)
	}
	report.Expression(stdout, t, useColor)
	return nil
}

func emitBatch(stdout io.Writer, paths []string, jsonOut, cal, useColor bool) error {
	if len(paths) == 0 {
		return fmt.Errorf("no .timer files found")
	}
	res, err := audit.AuditPaths(paths, audit.Config{})
	if err != nil {
		return err
	}
	switch {
	case jsonOut:
		return report.JSON(stdout, res)
	case cal:
		report.Calendar(stdout, res, useColor)
		return nil
	default:
		report.Text(stdout, res, useColor)
		return nil
	}
}

// collectTimers walks each input path and returns the absolute paths
// of every *.timer file beneath it. Files passed directly are kept.
func collectTimers(roots []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, root := range roots {
		st, err := os.Stat(root)
		if err != nil {
			continue
		}
		if !st.IsDir() {
			if strings.HasSuffix(strings.ToLower(root), ".timer") {
				abs, _ := filepath.Abs(root)
				if !seen[abs] {
					seen[abs] = true
					out = append(out, root)
				}
			}
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(strings.ToLower(name), ".timer") {
				continue
			}
			full := filepath.Join(root, name)
			abs, _ := filepath.Abs(full)
			if seen[abs] {
				continue
			}
			seen[abs] = true
			out = append(out, full)
		}
	}
	sort.Strings(out)
	return out, nil
}

func systemPaths() []string {
	return []string{"/etc/systemd/system", "/usr/lib/systemd/system"}
}

func userPaths() []string {
	out := []string{"/usr/lib/systemd/user"}
	if home, err := os.UserHomeDir(); err == nil {
		out = append([]string{filepath.Join(home, ".config", "systemd", "user")}, out...)
	}
	return out
}

func isExistingFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func isExistingDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func versionString() string {
	s := "timer-doctor " + version
	if commit != "" {
		s += " (" + shortCommit(commit)
		if date != "" {
			s += ", " + date
		}
		s += ")"
	}
	return s
}

func shortCommit(c string) string {
	if len(c) > 7 {
		return c[:7]
	}
	return c
}
