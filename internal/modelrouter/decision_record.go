package modelrouter

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	decisionRecordLimit  = 2048
	decisionJournalLimit = 256 << 10
)

// DecisionRecord is the redacted, bounded evidence for one route or completion.
// It intentionally has no prompt, output, provider error text, or credential field.
type DecisionRecord struct {
	At                time.Time    `json:"at"`
	Task              TaskClass    `json:"task"`
	Privacy           PrivacyClass `json:"privacy"`
	Latency           LatencyClass `json:"latency"`
	MinContextTokens  int          `json:"min_context_tokens"`
	RequestedOverride Lane         `json:"requested_override,omitempty"`
	InitialLane       Lane         `json:"initial_lane,omitempty"`
	FinalLane         Lane         `json:"final_lane,omitempty"`
	Provider          string       `json:"provider,omitempty"`
	Model             string       `json:"model,omitempty"`
	Reason            string       `json:"reason,omitempty"`
	Outcome           string       `json:"outcome"`
	FailureCategory   string       `json:"failure_category,omitempty"`
}

// DecisionSink receives one bounded routing or execution decision.
type DecisionSink interface {
	Record(DecisionRecord) error
}

// PrivateDecisionSink stores a small owner-local JSONL journal under ~/.sirsi.
// An explicit home is accepted for tests; production passes an empty value.
type PrivateDecisionSink struct {
	Home string
}

var decisionJournalMu sync.Mutex

func NewPrivateDecisionSink(home string) *PrivateDecisionSink {
	return &PrivateDecisionSink{Home: home}
}

func (s *PrivateDecisionSink) Record(record DecisionRecord) error {
	decisionJournalMu.Lock()
	defer decisionJournalMu.Unlock()
	if record.At.IsZero() {
		record.At = time.Now().UTC()
	}
	record.Provider = decisionLabel(record.Provider)
	record.Model = decisionLabel(record.Model)
	record.Reason = decisionReason(record.Reason)
	record.Outcome = decisionLabel(record.Outcome)
	record.FailureCategory = decisionLabel(record.FailureCategory)
	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("model-router: encode decision record")
	}
	line = append(line, '\n')
	if len(line) > decisionRecordLimit {
		return fmt.Errorf("model-router: decision record exceeds size limit")
	}
	home := s.Home
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("model-router: resolve private decision directory")
		}
	}
	dir := filepath.Join(home, ".sirsi")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("model-router: create private decision directory")
	}
	dirInfo, err := os.Lstat(dir)
	if err != nil || !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 || dirInfo.Mode().Perm() != 0o700 {
		return fmt.Errorf("model-router: decision directory must be a private 0700 directory")
	}
	path := filepath.Join(dir, "model-router-decisions.jsonl")
	if info, statErr := os.Lstat(path); statErr == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			return fmt.Errorf("model-router: decision journal must be a regular 0600 file")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("model-router: inspect private decision journal")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("model-router: open private decision journal")
	}
	info, statErr := f.Stat()
	if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		_ = f.Close()
		return fmt.Errorf("model-router: decision journal permissions are not private")
	}
	pathInfo, pathErr := os.Lstat(path)
	if pathErr != nil || !pathInfo.Mode().IsRegular() || !os.SameFile(info, pathInfo) {
		_ = f.Close()
		return fmt.Errorf("model-router: decision journal changed during secure open")
	}
	if info.Size()+int64(len(line)) > decisionJournalLimit {
		if err := f.Truncate(0); err != nil {
			_ = f.Close()
			return fmt.Errorf("model-router: bound private decision journal")
		}
	}
	_, writeErr := f.Write(line)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return fmt.Errorf("model-router: write private decision journal")
	}
	return nil
}

type DecisionRecordError struct{ err error }

func (e *DecisionRecordError) Error() string { return "model-router: decision evidence write failed" }
func (e *DecisionRecordError) Unwrap() error { return e.err }

func (rt *Router) recordDecision(req Request, d Decision, outcome string, cause error) error {
	if rt.decisionSink == nil {
		return nil
	}
	rec := DecisionRecord{
		At: time.Now().UTC(), Task: safeTask(req.Task), Privacy: safePrivacy(req.Privacy),
		Latency: safeLatency(req.Latency), MinContextTokens: max(0, req.MinContextTokens),
		RequestedOverride: safeLane(req.Override), InitialLane: safeLane(d.InitialLane),
		FinalLane: safeLane(d.Lane), Reason: d.Rationale, Outcome: outcome,
		FailureCategory: d.FailureCategory,
	}
	if d.Provider != nil {
		rec.Provider = d.Provider.Name()
	}
	if d.ProviderName != "" {
		rec.Provider = d.ProviderName
	}
	rec.Model = d.Model
	if rec.FailureCategory == "" && cause != nil {
		rec.FailureCategory = failureCategory(cause)
	}
	rec.Provider = decisionLabel(rec.Provider)
	rec.Model = decisionLabel(rec.Model)
	rec.Reason = decisionReason(rec.Reason)
	rec.Outcome = decisionLabel(rec.Outcome)
	rec.FailureCategory = decisionLabel(rec.FailureCategory)
	if err := rt.decisionSink.Record(rec); err != nil {
		d.FailureCategory = "decision_record_failure"
		return &DecisionRecordError{err: err}
	}
	return nil
}

func decisionLabel(value string) string {
	lower := strings.ToLower(value)
	if len(value) > 64 || strings.ContainsAny(value, "@=\\\r\n\t ") || strings.Contains(lower, "://") || strings.Contains(lower, "bearer") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "api_key") || strings.Contains(lower, "sk-") {
		return "redacted"
	}
	var b strings.Builder
	for _, r := range value {
		if b.Len() >= 96 {
			break
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("._:-", r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func decisionReason(value string) string {
	var b strings.Builder
	for _, r := range value {
		if b.Len() >= 160 {
			break
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune(" ._:-=+();", r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func safeTask(v TaskClass) TaskClass {
	if v == TaskGeneration || v == TaskJudgment || v == TaskExtraction || v == TaskEmbedding {
		return v
	}
	return ""
}
func safePrivacy(v PrivacyClass) PrivacyClass {
	if v == PrivacyLocalOnly || v == PrivacyShareable {
		return v
	}
	return ""
}
func safeLatency(v LatencyClass) LatencyClass {
	if v == LatencyInteractive || v == LatencyBackground {
		return v
	}
	return ""
}
func safeLane(v Lane) Lane {
	if v == LaneLocal || v == LaneRemote {
		return v
	}
	return ""
}
