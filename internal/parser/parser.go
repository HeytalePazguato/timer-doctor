// Package parser reads systemd .timer and .service unit files (an INI
// subset) into typed structs. It is intentionally small: only the keys
// timer-doctor cares about are captured; everything else is preserved
// raw so a future check can read it without a parser change.
package parser

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Section is one [Section] block in a unit file.
type Section struct {
	Name string
	// Keys preserves multiplicity (a single timer can have several
	// OnCalendar= lines) and the line number of the first occurrence
	// for error reporting.
	Keys []KeyValue
}

// KeyValue is one Key= line.
type KeyValue struct {
	Key   string
	Value string
	Line  int
}

// UnitFile is the result of parsing a .timer or .service file.
type UnitFile struct {
	Path     string
	Sections []*Section
	// ParseErrors collects per-line failures (unterminated continuation,
	// keys outside any [Section], etc). Parsing continues past them so
	// downstream checks can still report on what was readable.
	ParseErrors []ParseError
}

// ParseError is a per-line failure in a unit file.
type ParseError struct {
	Line    int
	Message string
}

func (e ParseError) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Message) }

// ParseFile reads and parses a unit file from disk.
func ParseFile(path string) (*UnitFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	uf, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	uf.Path = path
	return uf, nil
}

// Parse reads a unit file from any io.Reader.
func Parse(r io.Reader) (*UnitFile, error) {
	uf := &UnitFile{}
	var current *Section
	// Continuation: a line ending in `\` is joined with the next line.
	var pending strings.Builder
	var pendingKey string
	var pendingLine int

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		raw := sc.Text()
		// Strip BOM on the first line.
		if lineNo == 1 {
			raw = strings.TrimPrefix(raw, "\ufeff")
		}
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)

		// Comments and blank lines.
		if pending.Len() == 0 && (trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";")) {
			continue
		}

		// Active continuation: append, check for further continuation.
		if pending.Len() > 0 {
			val, more := stripContinuation(line)
			pending.WriteString(" ")
			pending.WriteString(strings.TrimSpace(val))
			if more {
				continue
			}
			if current == nil {
				uf.ParseErrors = append(uf.ParseErrors, ParseError{pendingLine, "key outside any [Section]"})
			} else {
				current.Keys = append(current.Keys, KeyValue{pendingKey, strings.TrimSpace(pending.String()), pendingLine})
			}
			pending.Reset()
			pendingKey = ""
			pendingLine = 0
			continue
		}

		// New section header.
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			name := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			if name == "" {
				uf.ParseErrors = append(uf.ParseErrors, ParseError{lineNo, "empty section header"})
				continue
			}
			current = &Section{Name: name}
			uf.Sections = append(uf.Sections, current)
			continue
		}

		// Key=Value line.
		eq := strings.IndexByte(trimmed, '=')
		if eq <= 0 {
			uf.ParseErrors = append(uf.ParseErrors, ParseError{lineNo, "expected Key=Value"})
			continue
		}
		key := strings.TrimSpace(trimmed[:eq])
		val := trimmed[eq+1:]

		stripped, more := stripContinuation(val)
		if more {
			pending.WriteString(strings.TrimSpace(stripped))
			pendingKey = key
			pendingLine = lineNo
			continue
		}
		if current == nil {
			uf.ParseErrors = append(uf.ParseErrors, ParseError{lineNo, "key outside any [Section]"})
			continue
		}
		current.Keys = append(current.Keys, KeyValue{key, strings.TrimSpace(stripped), lineNo})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if pending.Len() > 0 {
		uf.ParseErrors = append(uf.ParseErrors, ParseError{pendingLine, "unterminated line continuation"})
	}
	return uf, nil
}

func stripContinuation(s string) (rest string, more bool) {
	t := strings.TrimRight(s, " \t")
	if strings.HasSuffix(t, "\\") {
		return strings.TrimSuffix(t, "\\"), true
	}
	return s, false
}

// Section returns the first section with the given name, or nil.
func (u *UnitFile) Section(name string) *Section {
	for _, s := range u.Sections {
		if strings.EqualFold(s.Name, name) {
			return s
		}
	}
	return nil
}

// Get returns the last value for a key in the section (the systemd
// convention: later assignments win for non-list keys), and a bool
// indicating presence.
func (s *Section) Get(key string) (string, bool) {
	if s == nil {
		return "", false
	}
	var v string
	var found bool
	for _, kv := range s.Keys {
		if strings.EqualFold(kv.Key, key) {
			v = kv.Value
			found = true
		}
	}
	return v, found
}

// All returns every value for a key in section order. Used for keys
// that may appear multiple times (OnCalendar=, ExecStart=).
func (s *Section) All(key string) []KeyValue {
	if s == nil {
		return nil
	}
	out := make([]KeyValue, 0, len(s.Keys))
	for _, kv := range s.Keys {
		if strings.EqualFold(kv.Key, key) {
			out = append(out, kv)
		}
	}
	return out
}

// PairedServicePath returns the on-disk path of the .service unit
// associated with a .timer file. The default rule: `foo.timer` →
// `foo.service` in the same directory. If `[Timer] Unit=` is set, that
// name overrides the default.
func PairedServicePath(timerPath string, timer *UnitFile) string {
	dir := filepath.Dir(timerPath)
	base := filepath.Base(timerPath)
	name := strings.TrimSuffix(base, filepath.Ext(base)) + ".service"
	if t := timer.Section("Timer"); t != nil {
		if v, ok := t.Get("Unit"); ok && v != "" {
			name = v
		}
	}
	return filepath.Join(dir, name)
}
