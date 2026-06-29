package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// sampleNodeStatus returns a deterministic NodeStatus for testing.
// No real router state is read; this is a pure fixture (A16).
func sampleNodeStatus() *router.NodeStatus {
	return &router.NodeStatus{
		SchemaVersion:    router.NodeStatusSchemaVersion,
		RouterHome:       "/tmp/router",
		RepoRoot:         "/tmp/repo",
		RegisteredAgents: []string{"claude-home", "claude-pantheon", "codex-pantheon"},
		AgentCount:       3,
		PendingByAgent:   map[string][]string{"claude-pantheon": {"item-1", "item-2"}},
		TotalPending:     2,
		ActiveTopics:     []string{"adr-026"},
		CompletedCount:   42,
		WorkItemSummary:  map[string]int{"open": 2, "closed": 42},
		LiveThreads: []router.ThreadSummary{
			{ThreadID: "thr-aaa", AgentID: "claude-home", Surface: "claude", Status: router.ThreadStatusActive, PID: 100},
			{ThreadID: "thr-bbb", AgentID: "claude-pantheon", Surface: "claude", Status: router.ThreadStatusActive, PID: 200},
		},
		StaleThreads:    []router.ThreadSummary{{ThreadID: "thr-ccc", AgentID: "codex-pantheon", Status: router.ThreadStatusStale, PID: 300}},
		LiveThreadCount: 2,
		AgentHealth: []router.AgentHealthCheck{
			{AgentType: "claude", CLIFound: true, AuthOK: true},
			{AgentType: "codex", CLIFound: true, AuthOK: false, NeedsLogin: true, AuthError: "not authenticated"},
		},
	}
}

func TestNodeStatus_Full_ServesContractType(t *testing.T) {
	cfg := Config{NodeStatusFn: func() (*router.NodeStatus, error) { return sampleNodeStatus(), nil }}
	s := New(cfg)

	rec := httptest.NewRecorder()
	s.apiNodeStatus(rec, httptest.NewRequest(http.MethodGet, "/api/node-status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got router.NodeStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body as router.NodeStatus: %v", err)
	}
	if got.SchemaVersion != router.NodeStatusSchemaVersion {
		t.Errorf("schema_version = %q, want %q", got.SchemaVersion, router.NodeStatusSchemaVersion)
	}
	if got.LiveThreadCount != 2 || len(got.LiveThreads) != 2 || len(got.StaleThreads) != 1 {
		t.Errorf("live/stale counts = (%d,%d,%d), want (2,2,1)", got.LiveThreadCount, len(got.LiveThreads), len(got.StaleThreads))
	}
	if got.TotalPending != 2 {
		t.Errorf("TotalPending = %d, want 2", got.TotalPending)
	}
}

func TestNodeStatus_Summary_BoundedAndDerived(t *testing.T) {
	cfg := Config{NodeStatusFn: func() (*router.NodeStatus, error) { return sampleNodeStatus(), nil }}
	s := New(cfg)

	rec := httptest.NewRecorder()
	s.apiNodeStatus(rec, httptest.NewRequest(http.MethodGet, "/api/node-status?view=summary", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got OpsSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body as OpsSummary: %v", err)
	}
	if got.SchemaVersion != router.NodeStatusSchemaVersion {
		t.Errorf("summary missed schema_version mirror: %q", got.SchemaVersion)
	}
	// Roll-ups reduced from the same source — NO independent fields.
	if got.LiveThreadCount != 2 || got.StaleThreadCount != 1 || got.QueueOpenItems != 2 {
		t.Errorf("rollups = (live=%d stale=%d queue=%d), want (2,1,2)", got.LiveThreadCount, got.StaleThreadCount, got.QueueOpenItems)
	}
	// Drift flag set because codex CLI is needs_login.
	if !got.HasDriftOrAuthIssue {
		t.Error("HasDriftOrAuthIssue must be true (codex needs_login)")
	}
	if got.WorstIcon != "🔴" {
		t.Errorf("WorstIcon = %q, want 🔴", got.WorstIcon)
	}
	if got.MoreAgents != 0 {
		t.Errorf("MoreAgents = %d, want 0 (3 agents < 12 cap)", got.MoreAgents)
	}
	// codex-pantheon should be marked NeedsLogin (its AgentID contains "codex").
	var sawCodex bool
	for _, a := range got.Agents {
		if a.AgentID == "codex-pantheon" {
			sawCodex = true
			if !a.NeedsLogin {
				t.Error("codex-pantheon row must have NeedsLogin=true")
			}
		}
	}
	if !sawCodex {
		t.Error("codex-pantheon agent row missing from summary")
	}
}

// TestNodeStatus_StaleRegistrations_DoNotAlarm locks the fix for the long-standing
// "Horus is permanently yellow and no click clears it" bug: stale (old-session)
// thread registrations are cruft, not a current problem, so they must NOT turn
// the ops surface yellow. The count is still surfaced for transparency.
func TestNodeStatus_StaleRegistrations_DoNotAlarm(t *testing.T) {
	ns := &router.NodeStatus{
		SchemaVersion: router.NodeStatusSchemaVersion,
		StaleThreads: []router.ThreadSummary{
			{ThreadID: "thr-old1", AgentID: "claude-home", Status: router.ThreadStatusStale, PID: 200},
			{ThreadID: "thr-old2", AgentID: "claude-home", Status: router.ThreadStatusStale, PID: 201},
		},
	}
	cfg := Config{NodeStatusFn: func() (*router.NodeStatus, error) { return ns, nil }}
	s := New(cfg)
	rec := httptest.NewRecorder()
	s.apiNodeStatus(rec, httptest.NewRequest(http.MethodGet, "/api/node-status?view=summary", nil))

	var got OpsSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.StaleThreadCount != 2 {
		t.Fatalf("StaleThreadCount = %d, want 2 (count still surfaced)", got.StaleThreadCount)
	}
	if got.WorstIcon != "🟢" {
		t.Errorf("WorstIcon = %q, want 🟢 — stale registrations alone must not alarm", got.WorstIcon)
	}
}

func TestNodeStatus_Summary_Bounded_Truncates(t *testing.T) {
	ns := sampleNodeStatus()
	// Inflate registered list past the cap to prove truncation + MoreAgents.
	for i := 0; i < DefaultOpsSummaryMax+5; i++ {
		ns.RegisteredAgents = append(ns.RegisteredAgents, "filler-"+string(rune('a'+i)))
	}
	got := Summarize(ns, DefaultOpsSummaryMax)
	if len(got.Agents) != DefaultOpsSummaryMax {
		t.Errorf("Agents len = %d, want exactly %d (cap)", len(got.Agents), DefaultOpsSummaryMax)
	}
	if got.MoreAgents == 0 {
		t.Error("MoreAgents must be > 0 when truncating")
	}
}

func TestNodeStatus_NotWired_503(t *testing.T) {
	s := New(Config{}) // no NodeStatusFn
	rec := httptest.NewRecorder()
	s.apiNodeStatus(rec, httptest.NewRequest(http.MethodGet, "/api/node-status", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("unwired collector code = %d, want 503", rec.Code)
	}
}

func TestNodeStatus_CollectorErrorPropagates(t *testing.T) {
	s := New(Config{NodeStatusFn: func() (*router.NodeStatus, error) {
		return nil, errFake
	}})
	rec := httptest.NewRecorder()
	s.apiNodeStatus(rec, httptest.NewRequest(http.MethodGet, "/api/node-status", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("collector error code = %d, want 500", rec.Code)
	}
}

// errFake is a sentinel for the error-propagation test (no host side effect).
var errFake = &fakeErr{}

type fakeErr struct{}

func (*fakeErr) Error() string { return "fake collector failure" }
