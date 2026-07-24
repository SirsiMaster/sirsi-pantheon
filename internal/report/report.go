// Package report is the owner-facing run-report contract (owner directive
// 2026-07-23): what the fabric DID — heals, escalations, outcomes — surfaced
// through Pantheon's own surfaces (sirsi report, menubar "Last check"), not
// buried in agent-facing journals a human never opens.
//
// One producer contract, many writers: the resident supervisor writes a run
// per tick here; the cloud conduit writes the same schema from its 15-minute
// runs. Renderers (CLI verb, menubar) read the one file. Pattern: Fabric
// Board Producer (PR #204) — ONE JSON contract → thin renderers.
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SchemaVersion gates renderers, same discipline as the supervisor board.
const SchemaVersion = "1.0.0"

// MaxRuns caps the file so it never grows unbounded (newest first).
const MaxRuns = 50

// Outcome values. Renderers treat unknown values as "degraded" (fail loud).
const (
	OutcomeGreen    = "green"    // nothing needed doing
	OutcomeHealed   = "healed"   // something was broken; the fabric fixed it itself
	OutcomeDegraded = "degraded" // something is wrong that the fabric could NOT fix
)

// Run is one supervision/conduit pass, owner-readable.
type Run struct {
	TS      string `json:"ts"`      // RFC3339 UTC
	Source  string `json:"source"`  // "supervisor" | "conduit" | "watchdog"
	Outcome string `json:"outcome"` // green | healed | degraded
	// Heals: completed self-repairs, plain English ("local AI restarted (bounded)").
	Heals []string `json:"heals,omitempty"`
	// Escalations: conditions the fabric could NOT fix — each needs the owner
	// or a cloud agent. Plain English.
	Escalations []string `json:"escalations,omitempty"`
	// Next: what the pass queued for later, when worth telling the owner.
	Next []string `json:"next,omitempty"`
	// APIReachable: whether the Anthropic API answered the reachability probe
	// this pass (nil = not probed). The sovereignty signal: false means the
	// local fabric is on its own.
	APIReachable *bool `json:"api_reachable,omitempty"`
}

// File is the on-disk shape at Path().
type File struct {
	SchemaVersion string `json:"schema_version"`
	Runs          []Run  `json:"runs"` // newest first
}

// Path is the canonical report location (shared with the conduit's writer).
func Path() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".sirsi", "conduit-report.json")
}

// Load reads the report file; a missing file is an empty report, not an error.
func Load(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return File{SchemaVersion: SchemaVersion}, nil
		}
		return File{}, err
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return File{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return f, nil
}

// Append prepends a run (newest first), trims to MaxRuns, and writes
// atomically (temp + rename) so a renderer never reads a torn file.
func Append(path string, r Run) error {
	f, err := Load(path)
	if err != nil {
		// A corrupt file must not block reporting forever — start fresh but
		// keep the evidence beside it.
		_ = os.Rename(path, path+".corrupt")
		f = File{SchemaVersion: SchemaVersion}
	}
	f.SchemaVersion = SchemaVersion
	f.Runs = append([]Run{r}, f.Runs...)
	if len(f.Runs) > MaxRuns {
		f.Runs = f.Runs[:MaxRuns]
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
		return mkErr
	}
	tmp, err := os.CreateTemp(dir, ".report-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// Sentence renders one run as the plain-English line every surface shows —
// one implementation so the CLI and menubar never drift.
func Sentence(r Run) string {
	t := r.TS
	if parsed, err := time.Parse(time.RFC3339, r.TS); err == nil {
		t = parsed.Local().Format("15:04")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s — ", t)
	switch r.Outcome {
	case OutcomeGreen:
		b.WriteString("all green")
	case OutcomeHealed:
		if len(r.Heals) > 0 {
			b.WriteString(strings.Join(r.Heals, "; "))
		} else {
			b.WriteString("self-healed")
		}
	default:
		if len(r.Escalations) > 0 {
			b.WriteString("needs attention: " + strings.Join(r.Escalations, "; "))
		} else {
			b.WriteString("degraded")
		}
	}
	if r.APIReachable != nil && !*r.APIReachable {
		b.WriteString(" · cloud unreachable, local AI holding the fort")
	}
	return b.String()
}
