package maat

// The decision journal is a local, append-only read model for Ma'at.  It
// deliberately records what the scheduler assessed; it does not make policy
// decisions for another component, and it does not claim that a local record
// is a cross-host attestation.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Decision is the stable, surface-neutral explanation of one Ma'at outcome.
// It is intentionally a fact record: consumers render it, but never infer an
// authorization absent from its determination and evidence reference.
type Decision struct {
	Time          string `json:"time"`
	Host          string `json:"host"`
	Kind          string `json:"kind"`
	Requester     string `json:"requester"`
	Resource      string `json:"resource,omitempty"`
	Assessed      string `json:"assessed"`
	Affected      string `json:"affected,omitempty"`
	Determination string `json:"determination"`
	Why           string `json:"why"`
	Evidence      string `json:"evidence,omitempty"`
}

// DecisionJournal persists the local decision projection. Implementations may
// be supplied by tests or by a different replicated transport later.
type DecisionJournal interface {
	Append(Decision) error
	Recent(limit int) ([]Decision, error)
}

// FileDecisionJournal stores newline-delimited JSON beneath the user's Sirsi
// state. It is a projection, not the reservation scheduler's authority.
type FileDecisionJournal struct {
	Path string
}

func DefaultDecisionPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("maat decision path: resolve home: %w", err)
	}
	return filepath.Join(home, ".sirsi", "maat", "decisions.jsonl"), nil
}

func NewDefaultDecisionJournal() (*FileDecisionJournal, error) {
	path, err := DefaultDecisionPath()
	if err != nil {
		return nil, err
	}
	return &FileDecisionJournal{Path: path}, nil
}

func (j *FileDecisionJournal) Append(decision Decision) error {
	if j == nil || strings.TrimSpace(j.Path) == "" {
		return fmt.Errorf("maat decision journal: empty path")
	}
	if err := normalizeDecision(&decision); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(j.Path), 0o700); err != nil {
		return fmt.Errorf("maat decision journal: create parent: %w", err)
	}
	encoded, err := json.Marshal(decision)
	if err != nil {
		return fmt.Errorf("maat decision journal: encode: %w", err)
	}
	f, err := os.OpenFile(j.Path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("maat decision journal: open: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("maat decision journal: append: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("maat decision journal: sync: %w", err)
	}
	return nil
}

func (j *FileDecisionJournal) Recent(limit int) ([]Decision, error) {
	if j == nil || strings.TrimSpace(j.Path) == "" {
		return nil, fmt.Errorf("maat decision journal: empty path")
	}
	if limit <= 0 {
		limit = 50
	}
	f, err := os.Open(j.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Decision{}, nil
		}
		return nil, fmt.Errorf("maat decision journal: open: %w", err)
	}
	defer f.Close()

	var decisions []Decision
	scanner := bufio.NewScanner(f)
	// A decision is deliberately bounded; raising Scanner's default is for
	// structured evidence links, not an invitation to treat this as a trace log.
	scanner.Buffer(make([]byte, 4096), 256*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var decision Decision
		if err := json.Unmarshal([]byte(line), &decision); err != nil {
			return nil, fmt.Errorf("maat decision journal: malformed record: %w", err)
		}
		if err := validateDecision(decision); err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("maat decision journal: read: %w", err)
	}
	sort.SliceStable(decisions, func(i, k int) bool { return decisions[i].Time > decisions[k].Time })
	if len(decisions) > limit {
		decisions = decisions[:limit]
	}
	return decisions, nil
}

func normalizeDecision(decision *Decision) error {
	if decision.Time == "" {
		decision.Time = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if decision.Host == "" {
		host, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("maat decision journal: resolve host: %w", err)
		}
		decision.Host = host
	}
	return validateDecision(*decision)
}

func validateDecision(decision Decision) error {
	for field, value := range map[string]string{
		"time": decision.Time, "host": decision.Host, "kind": decision.Kind,
		"requester": decision.Requester, "assessed": decision.Assessed,
		"determination": decision.Determination, "why": decision.Why,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("maat decision journal: %s is required", field)
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, decision.Time); err != nil {
		return fmt.Errorf("maat decision journal: invalid time: %w", err)
	}
	return nil
}

// ReadAll is a narrow helper for an already-open trusted stream. It exists for
// future descriptor-bound transport adapters; the file journal itself owns the
// on-disk path lifecycle.
func ReadAll(r io.Reader) ([]byte, error) { return io.ReadAll(r) }
