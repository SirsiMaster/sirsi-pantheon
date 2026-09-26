// Package decision is Ma'at's live decision ledger: one append-only JSONL file
// per host recording every grant/refuse, conflict, guard verdict, window-gate
// block, and CI pause/resume Ma'at makes. Built to the schema the owner asked
// for (2026-09-26, via claude-io): time, host, kind, requester, resource, what
// was assessed, who it affected, determination, why, evidence link.
//
// Canonical-ledger-location note: reservations already live cross-host in the
// router store (internal/maat/schedule); this file is deliberately host-local
// and simpler, matching the convention already in use by other hosts' writers
// (m5go, the maat-window-gate hook, maat-run-guard). Promoting it to a
// cross-host store is the follow-up Ra is being asked about — this package
// does not block on that answer.
package decision

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DefaultPath is the local decision ledger, overridable via
// SIRSI_MAAT_DECISIONS_PATH (tests, alternate hosts).
func DefaultPath() string {
	if p := os.Getenv("SIRSI_MAAT_DECISIONS_PATH"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".sirsi", "maat", "decisions.jsonl")
}

// Record is one Ma'at decision. Kept as a map, not a fixed struct: other
// hosts write this same file, and a strict struct would silently drop any
// field name that doesn't match instead of displaying it.
type Record map[string]any

// New builds a Record with the standard fields, stamping time/host.
func New(kind, requester, resource, assessed, affected, determination, why, evidence string) Record {
	host, _ := os.Hostname()
	return Record{
		"time":          time.Now().UTC().Format(time.RFC3339),
		"host":          host,
		"kind":          kind,
		"requester":     requester,
		"resource":      resource,
		"assessed":      assessed,
		"affected":      affected,
		"determination": determination,
		"why":           why,
		"evidence":      evidence,
	}
}

// Append writes one decision record as a JSON line, creating the file and its
// parent directory if needed.
func Append(path string, rec Record) error {
	if path == "" {
		path = DefaultPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("maat decision dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open decisions ledger: %w", err)
	}
	defer f.Close()
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

// Line is one decision as read back: its parsed fields plus a stable
// content-derived ID (first 8 hex chars of sha256 of the raw line) so `sirsi
// maat decisions show <id>` works without the writer minting one.
type Line struct {
	ID     string
	Fields Record
}

func (l Line) str(key string) string {
	v, ok := l.Fields[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func (l Line) Time() string          { return l.str("time") }
func (l Line) Host() string          { return l.str("host") }
func (l Line) Kind() string          { return l.str("kind") }
func (l Line) Requester() string     { return l.str("requester") }
func (l Line) Resource() string      { return l.str("resource") }
func (l Line) Determination() string { return l.str("determination") }

// Read loads every decision line from path (default DefaultPath()), skipping
// blank and malformed lines rather than failing the whole read.
func Read(path string) ([]Line, error) {
	if path == "" {
		path = DefaultPath()
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []Line
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(raw), &rec); err != nil {
			continue
		}
		sum := sha256.Sum256([]byte(raw))
		lines = append(lines, Line{ID: hex.EncodeToString(sum[:])[:8], Fields: rec})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

// Filter narrows lines by exact-fold kind/host match and a since cutoff.
// Lines with an unparsable timestamp are kept rather than dropped — an
// unreadable time must not silently vanish from the view.
func Filter(lines []Line, kind, host string, since time.Time) []Line {
	out := make([]Line, 0, len(lines))
	for _, l := range lines {
		if kind != "" && !strings.EqualFold(l.Kind(), kind) {
			continue
		}
		if host != "" && !strings.EqualFold(l.Host(), host) {
			continue
		}
		if !since.IsZero() {
			if t, err := time.Parse(time.RFC3339, l.Time()); err == nil && t.Before(since) {
				continue
			}
		}
		out = append(out, l)
	}
	return out
}

// SortDesc orders lines newest-first by their time field; unparsable
// timestamps sort last.
func SortDesc(lines []Line) {
	sort.SliceStable(lines, func(i, j int) bool {
		ti, ei := time.Parse(time.RFC3339, lines[i].Time())
		tj, ej := time.Parse(time.RFC3339, lines[j].Time())
		if ei != nil {
			return false
		}
		if ej != nil {
			return true
		}
		return ti.After(tj)
	})
}
