package neith

import (
	"testing"
)

// ── Net (Neith) Test Suite ─────────────────────────────────────────
// Tests for the Net Weaver module — plan alignment & tapestry.

func TestWeave_AssessLogs(t *testing.T) {
	t.Parallel()

	w := &Weave{
		SessionID: "test-001",
		Plan:      []string{"Build pulse engine", "Tag v1.0.0-rc1"},
	}

	score, err := w.AssessLogs("## Sprint 15: Built pulse engine")
	if err != nil {
		t.Fatalf("AssessLogs() error: %v", err)
	}
	if score < 0 || score > 1.0 {
		t.Errorf("AssessLogs() score = %f, want between 0.0 and 1.0", score)
	}
}

func TestWeave_EmptyPlan(t *testing.T) {
	t.Parallel()

	w := &Weave{}
	score, err := w.AssessLogs("")
	if err != nil {
		t.Fatalf("AssessLogs(empty) error: %v", err)
	}
	if score != 1.0 {
		t.Errorf("AssessLogs(empty) = %f, want 1.0 (no plan = no drift)", score)
	}
}

func TestTapestry_Align_Consistent(t *testing.T) {
	t.Parallel()

	tap := &Tapestry{
		MaatConsistent: true,
		AnubisCorrect:  true,
		HygieneClean:   true,
		ThothAccurate:  true,
		IsisHardened:   true,
	}

	err := tap.Align()
	if err != nil {
		t.Errorf("Align() should pass when all checks true, got: %v", err)
	}
}

func TestTapestry_Align_Inconsistent(t *testing.T) {
	t.Parallel()

	tap := &Tapestry{
		MaatConsistent: false,
		AnubisCorrect:  true,
		HygieneClean:   true,
		ThothAccurate:  true,
		IsisHardened:   true,
	}

	err := tap.Align()
	if err == nil {
		t.Error("Align() should fail when Ma'at is inconsistent")
	}
}

func TestTapestry_Align_EmptyTapestry(t *testing.T) {
	t.Parallel()

	tap := &Tapestry{}
	err := tap.Align()
	if err == nil {
		t.Error("Align() should fail with zero-value tapestry (MaatConsistent=false)")
	}
}

func TestWeave_FieldAccess(t *testing.T) {
	t.Parallel()

	w := &Weave{
		SessionID:    "session-42",
		Plan:         []string{"A", "B", "C"},
		Achievements: []string{"A"},
		DriftFound:   true,
	}

	if w.SessionID != "session-42" {
		t.Errorf("SessionID = %q", w.SessionID)
	}
	if len(w.Plan) != 3 {
		t.Errorf("Plan len = %d, want 3", len(w.Plan))
	}
	if len(w.Achievements) != 1 {
		t.Errorf("Achievements len = %d, want 1", len(w.Achievements))
	}
	if !w.DriftFound {
		t.Error("DriftFound should be true")
	}
}

// ── AssessLogs real logic ───────────────────────────────────────────

func TestAssessLogs_FullMatch(t *testing.T) {
	t.Parallel()
	w := &Weave{Plan: []string{"build scanner", "fix tests"}}
	score, err := w.AssessLogs("We build the scanner module and fix tests across the board.")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if score < 0.99 {
		t.Errorf("score = %f, want ~1.0 (all keywords present)", score)
	}
}

func TestAssessLogs_NoMatch(t *testing.T) {
	t.Parallel()
	w := &Weave{Plan: []string{"deploy kubernetes cluster"}}
	score, err := w.AssessLogs("This log talks about nothing related at all xyz.")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if score > 0.01 {
		t.Errorf("score = %f, want ~0.0 (no keywords match)", score)
	}
}

func TestAssessLogs_PartialMatch(t *testing.T) {
	t.Parallel()
	w := &Weave{Plan: []string{"build pulse engine", "deploy to production"}}
	score, err := w.AssessLogs("We build the engine for local testing.")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if score < 0.2 || score > 0.8 {
		t.Errorf("score = %f, want partial match between 0.2 and 0.8", score)
	}
}

// ── Tapestry.Align — all deity checks ───────────────────────────────

func TestAlign_AnubisIncorrect(t *testing.T) {
	t.Parallel()
	tap := &Tapestry{MaatConsistent: true, AnubisCorrect: false, HygieneClean: true, ThothAccurate: true, IsisHardened: true}
	err := tap.Align()
	if err == nil {
		t.Fatal("expected error for AnubisCorrect=false")
	}
	if !contains(err.Error(), "Anubis") {
		t.Errorf("error should mention Anubis: %v", err)
	}
}

func TestAlign_HygieneNotClean(t *testing.T) {
	t.Parallel()
	tap := &Tapestry{MaatConsistent: true, AnubisCorrect: true, HygieneClean: false, ThothAccurate: true, IsisHardened: true}
	err := tap.Align()
	if err == nil {
		t.Fatal("expected error for HygieneClean=false")
	}
	if !contains(err.Error(), "hygiene") {
		t.Errorf("error should mention hygiene: %v", err)
	}
}

func TestAlign_ThothInaccurate(t *testing.T) {
	t.Parallel()
	tap := &Tapestry{MaatConsistent: true, AnubisCorrect: true, HygieneClean: true, ThothAccurate: false, IsisHardened: true}
	err := tap.Align()
	if err == nil {
		t.Fatal("expected error for ThothAccurate=false")
	}
	if !contains(err.Error(), "Thoth") {
		t.Errorf("error should mention Thoth: %v", err)
	}
}

func TestAlign_IsisNotHardened(t *testing.T) {
	t.Parallel()
	tap := &Tapestry{MaatConsistent: true, AnubisCorrect: true, HygieneClean: true, ThothAccurate: true, IsisHardened: false}
	err := tap.Align()
	if err == nil {
		t.Fatal("expected error for IsisHardened=false")
	}
	if !contains(err.Error(), "Isis") {
		t.Errorf("error should mention Isis: %v", err)
	}
}

// ── CheckDrift ──────────────────────────────────────────────────────

func TestCheckDrift_NoDrift(t *testing.T) {
	t.Parallel()
	w := &Weave{
		Plan:         []string{"Build scanner", "Fix tests"},
		Achievements: []string{"build scanner", "fix tests"},
	}
	w.CheckDrift()
	if w.DriftFound {
		t.Error("expected no drift when all plan items achieved (case-insensitive)")
	}
}

func TestCheckDrift_HasDrift(t *testing.T) {
	t.Parallel()
	w := &Weave{
		Plan:         []string{"Build scanner", "Deploy to prod"},
		Achievements: []string{"Build scanner"},
	}
	w.CheckDrift()
	if !w.DriftFound {
		t.Error("expected drift when plan items are unachieved")
	}
}

func TestCheckDrift_EmptyPlan(t *testing.T) {
	t.Parallel()
	w := &Weave{DriftFound: true}
	w.CheckDrift()
	if w.DriftFound {
		t.Error("expected no drift for empty plan")
	}
}

// helper
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || findSubstring(s, substr))
}

func findSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
