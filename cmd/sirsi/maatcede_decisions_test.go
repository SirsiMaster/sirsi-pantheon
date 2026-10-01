package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// fakeCedeDecisionWriter is an io.WriteCloser whose Write and Close outcomes
// are set independently, so tests can exercise a close-time failure that a
// real *os.File cannot be made to produce deterministically.
type fakeCedeDecisionWriter struct {
	writeErr error
	closeErr error
}

func (f *fakeCedeDecisionWriter) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(p), nil
}

func (f *fakeCedeDecisionWriter) Close() error { return f.closeErr }

func readJSONLines(t *testing.T, path string) []cedeDecisionRecord {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []cedeDecisionRecord
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var rec cedeDecisionRecord
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("bad decision line %q: %v", sc.Text(), err)
		}
		out = append(out, rec)
	}
	return out
}

// TestCedeDecisionLedger_RequestThenGrant runs a request through the command
// layer, then a grant, and asserts the decision ledger got exactly the two
// expected lines — a pending request must never appear as a grant.
func TestCedeDecisionLedger_RequestThenGrant(t *testing.T) {
	// This host may have SIRSI_ROUTER_URL set (live fabric spool/service);
	// Resolve() checks that BEFORE SIRSI_ROUTER_DB, so it must be cleared
	// here or this test would write real cede requests into production
	// (that happened once during development of this test — see the PR).
	t.Setenv("SIRSI_ROUTER_URL", "")
	db := filepath.Join(t.TempDir(), "router.db")
	t.Setenv("SIRSI_ROUTER_DB", db)
	decisions := filepath.Join(t.TempDir(), "decisions.jsonl")
	t.Setenv("MAAT_DECISIONS", decisions)

	oldRequester, oldHolder, oldAsk, oldMinutes, oldEarliest, oldReason :=
		cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeEarliest, cedeReason
	oldStart, oldCounter, oldJSON := cedeStart, cedeCounter, maatJSON
	t.Cleanup(func() {
		cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeEarliest, cedeReason =
			oldRequester, oldHolder, oldAsk, oldMinutes, oldEarliest, oldReason
		cedeStart, cedeCounter, maatJSON = oldStart, oldCounter, oldJSON
	})
	maatJSON = false

	cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeEarliest, cedeReason =
		"claude-io", "sne", "machine", 25, "2026-09-26T15:00:00Z", "need the box for H9"
	if err := maatCedeRequestCmd.RunE(maatCedeRequestCmd, []string{"m1"}); err != nil {
		t.Fatal(err)
	}

	st, err := routerstore.OpenPath(db)
	if err != nil {
		t.Fatal(err)
	}
	l := schedule.NewLedger(st)
	pending, err := l.ListCedes(schedule.CedeFilter{Resource: "m1", PendingOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("want 1 pending cede, got %d", len(pending))
	}
	id := pending[0].ID
	_ = st.Close()

	cedeHolder, cedeReason, cedeStart = "sne", "sure, after my run", "after my current run"
	if err := maatCedeGrantCmd.RunE(maatCedeGrantCmd, []string{id}); err != nil {
		t.Fatal(err)
	}

	lines := readJSONLines(t, decisions)
	if len(lines) != 2 {
		t.Fatalf("want 2 decision lines, got %d: %+v", len(lines), lines)
	}

	req, grant := lines[0], lines[1]
	if req.Kind != "cede-request" || req.Determination != "pending" {
		t.Fatalf("first line must be the pending request, got %+v", req)
	}
	if req.Requester != "claude-io" || req.Affected != "sne" || req.Resource != "m1" || req.Evidence != id {
		t.Fatalf("request line fields wrong: %+v", req)
	}

	if grant.Kind != "cede-grant" || grant.Determination != "grant" {
		t.Fatalf("second line must be the grant, got %+v", grant)
	}
	if grant.Requester != "sne" || grant.Affected != "claude-io" || grant.Resource != "m1" || grant.Evidence != id {
		t.Fatalf("grant line fields wrong: %+v", grant)
	}

	// The pending request must never itself appear as a grant.
	if req.Determination == "grant" {
		t.Fatal("a pending request line must never read as a grant")
	}
}

// TestCedeDecisionLedger_ProjectionFailureIsReported forces the decision
// ledger append to fail after the scheduler transition already committed,
// and asserts the CLI surfaces a clear error (naming the committed id)
// instead of reporting a durable success. SSA review 20260927-143128
// rejected the prior best-effort swallow on cmd/sirsi/maatcede.go.
func TestCedeDecisionLedger_ProjectionFailureIsReported(t *testing.T) {
	t.Setenv("SIRSI_ROUTER_URL", "")
	db := filepath.Join(t.TempDir(), "router.db")
	t.Setenv("SIRSI_ROUTER_DB", db)

	oldRequester, oldHolder, oldAsk, oldMinutes, oldEarliest, oldReason :=
		cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeEarliest, cedeReason
	oldJSON := maatJSON
	oldAppend := appendCedeDecision
	t.Cleanup(func() {
		cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeEarliest, cedeReason =
			oldRequester, oldHolder, oldAsk, oldMinutes, oldEarliest, oldReason
		maatJSON = oldJSON
		appendCedeDecision = oldAppend
	})
	maatJSON = false
	appendCedeDecision = func(cedeDecisionRecord) error {
		return os.ErrPermission
	}

	cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeEarliest, cedeReason =
		"claude-io", "sne", "machine", 10, "", "projection failure regression"
	err := maatCedeRequestCmd.RunE(maatCedeRequestCmd, []string{"m1"})
	if err == nil {
		t.Fatal("want an error when the decision ledger append fails, got nil (false success)")
	}

	// The scheduler transition must still have committed — the cede request
	// really exists — and the error must name it so a caller never retries
	// (retrying would double-apply the already-committed request).
	st, openErr := routerstore.OpenPath(db)
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer st.Close()
	l := schedule.NewLedger(st)
	pending, listErr := l.ListCedes(schedule.CedeFilter{Resource: "m1", PendingOnly: true})
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(pending) != 1 {
		t.Fatalf("want the cede request to have committed despite the projection failure, got %d pending", len(pending))
	}
	if got := pending[0].ID; got == "" || !strings.Contains(err.Error(), got) {
		t.Fatalf("error %q must name the committed cede id %q so a caller never retries it", err, got)
	}
}

// TestAppendCedeDecision_MkdirFailure forces a REAL mkdir failure (the parent
// path component is a plain file, so MkdirAll cannot descend into it) rather
// than an injected stand-in — no seam needed for this one.
func TestAppendCedeDecision_MkdirFailure(t *testing.T) {
	home := t.TempDir()
	blocker := filepath.Join(home, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MAAT_DECISIONS", filepath.Join(blocker, "nested", "decisions.jsonl"))

	err := appendCedeDecision(cedeDecisionRecord{Kind: "cede-request"})
	if err == nil || !strings.Contains(err.Error(), "mkdir") {
		t.Fatalf("want a mkdir failure, got %v", err)
	}
}

// TestAppendCedeDecision_OpenFailure forces a REAL open failure: the
// decisions path itself IS a directory, so os.OpenFile(..., O_WRONLY) fails.
func TestAppendCedeDecision_OpenFailure(t *testing.T) {
	home := t.TempDir()
	asDir := filepath.Join(home, "decisions.jsonl")
	if err := os.MkdirAll(asDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MAAT_DECISIONS", asDir)

	err := appendCedeDecision(cedeDecisionRecord{Kind: "cede-request"})
	if err == nil || !strings.Contains(err.Error(), "open") {
		t.Fatalf("want an open failure, got %v", err)
	}
}

// TestAppendCedeDecision_WriteErrorTakesPriorityOverCloseError: when both the
// write and the close fail, the write error — the more informative,
// closer-to-cause failure — must be the one returned.
func TestAppendCedeDecision_WriteErrorTakesPriorityOverCloseError(t *testing.T) {
	t.Setenv("MAAT_DECISIONS", filepath.Join(t.TempDir(), "decisions.jsonl"))
	oldOpener := cedeDecisionFileOpener
	t.Cleanup(func() { cedeDecisionFileOpener = oldOpener })
	cedeDecisionFileOpener = func(string) (io.WriteCloser, error) {
		return &fakeCedeDecisionWriter{writeErr: os.ErrInvalid, closeErr: os.ErrPermission}, nil
	}

	err := appendCedeDecision(cedeDecisionRecord{Kind: "cede-request"})
	if err == nil || !strings.Contains(err.Error(), "write failed") || !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("want the write error, got %v", err)
	}
}

// TestAppendCedeDecision_CloseErrorIsReported: codex-pantheon review of PR
// #929 (item 20261001-001503) — a `defer f.Close()` that discards its error
// can report success on a close-time I/O failure even though the write
// itself succeeded. A close-only failure must now surface.
func TestAppendCedeDecision_CloseErrorIsReported(t *testing.T) {
	t.Setenv("MAAT_DECISIONS", filepath.Join(t.TempDir(), "decisions.jsonl"))
	oldOpener := cedeDecisionFileOpener
	t.Cleanup(func() { cedeDecisionFileOpener = oldOpener })
	cedeDecisionFileOpener = func(string) (io.WriteCloser, error) {
		return &fakeCedeDecisionWriter{closeErr: os.ErrPermission}, nil
	}

	err := appendCedeDecision(cedeDecisionRecord{Kind: "cede-request"})
	if err == nil || !strings.Contains(err.Error(), "close failed") {
		t.Fatalf("want a close failure to be reported even though write succeeded, got %v", err)
	}
}

// TestCedeDecisionLedger_GrantProjectionFailureIsReported: the holder-response
// path (grant/counter/decline) must also fail the CLI, naming the committed
// cede id, when the decision-ledger projection fails — not just the request
// path (codex-pantheon review of PR #929, item 20261001-001503, required
// coverage for "request, holder response, withdraw, and floor-share paths").
func TestCedeDecisionLedger_GrantProjectionFailureIsReported(t *testing.T) {
	t.Setenv("SIRSI_ROUTER_URL", "")
	db := filepath.Join(t.TempDir(), "router.db")
	t.Setenv("SIRSI_ROUTER_DB", db)
	t.Setenv("MAAT_DECISIONS", filepath.Join(t.TempDir(), "decisions.jsonl"))

	oldRequester, oldHolder, oldAsk, oldMinutes, oldEarliest, oldReason :=
		cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeEarliest, cedeReason
	oldStart, oldJSON := cedeStart, maatJSON
	oldAppend := appendCedeDecision
	t.Cleanup(func() {
		cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeEarliest, cedeReason =
			oldRequester, oldHolder, oldAsk, oldMinutes, oldEarliest, oldReason
		cedeStart, maatJSON = oldStart, oldJSON
		appendCedeDecision = oldAppend
	})
	maatJSON = false

	cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeEarliest, cedeReason =
		"claude-io", "sne", "machine", 10, "", "grant projection failure regression"
	if err := maatCedeRequestCmd.RunE(maatCedeRequestCmd, []string{"m1"}); err != nil {
		t.Fatal(err)
	}

	st, err := routerstore.OpenPath(db)
	if err != nil {
		t.Fatal(err)
	}
	l := schedule.NewLedger(st)
	pending, err := l.ListCedes(schedule.CedeFilter{Resource: "m1", PendingOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("want 1 pending cede, got %d", len(pending))
	}
	id := pending[0].ID
	_ = st.Close()

	appendCedeDecision = func(cedeDecisionRecord) error { return os.ErrPermission }
	cedeHolder, cedeReason, cedeStart = "sne", "sure, after my run", "after my current run"
	grantErr := maatCedeGrantCmd.RunE(maatCedeGrantCmd, []string{id})
	if grantErr == nil {
		t.Fatal("want an error when the decision ledger append fails on grant, got nil (false success)")
	}
	if !strings.Contains(grantErr.Error(), id) {
		t.Fatalf("grant error %q must name the committed cede id %q", grantErr, id)
	}

	st2, err := routerstore.OpenPath(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	granted, err := schedule.NewLedger(st2).ListCedes(schedule.CedeFilter{Resource: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(granted) != 1 || granted[0].Status != schedule.CedeStatusGranted {
		t.Fatalf("the grant transition must still have committed despite the projection failure, got %+v", granted)
	}
}

// TestCedeDecisionLedger_WithdrawProjectionFailureIsReported: the withdraw
// path must also fail the CLI, naming the committed cede id, when the
// decision-ledger projection fails.
func TestCedeDecisionLedger_WithdrawProjectionFailureIsReported(t *testing.T) {
	t.Setenv("SIRSI_ROUTER_URL", "")
	db := filepath.Join(t.TempDir(), "router.db")
	t.Setenv("SIRSI_ROUTER_DB", db)
	t.Setenv("MAAT_DECISIONS", filepath.Join(t.TempDir(), "decisions.jsonl"))

	oldRequester, oldHolder, oldAsk, oldMinutes, oldEarliest, oldReason :=
		cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeEarliest, cedeReason
	oldJSON := maatJSON
	oldAppend := appendCedeDecision
	t.Cleanup(func() {
		cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeEarliest, cedeReason =
			oldRequester, oldHolder, oldAsk, oldMinutes, oldEarliest, oldReason
		maatJSON = oldJSON
		appendCedeDecision = oldAppend
	})
	maatJSON = false

	cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeEarliest, cedeReason =
		"claude-io", "sne", "machine", 10, "", "withdraw projection failure regression"
	if err := maatCedeRequestCmd.RunE(maatCedeRequestCmd, []string{"m1"}); err != nil {
		t.Fatal(err)
	}

	st, err := routerstore.OpenPath(db)
	if err != nil {
		t.Fatal(err)
	}
	l := schedule.NewLedger(st)
	pending, err := l.ListCedes(schedule.CedeFilter{Resource: "m1", PendingOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("want 1 pending cede, got %d", len(pending))
	}
	id := pending[0].ID
	_ = st.Close()

	appendCedeDecision = func(cedeDecisionRecord) error { return os.ErrPermission }
	withdrawErr := maatCedeWithdrawCmd.RunE(maatCedeWithdrawCmd, []string{id})
	if withdrawErr == nil {
		t.Fatal("want an error when the decision ledger append fails on withdraw, got nil (false success)")
	}
	if !strings.Contains(withdrawErr.Error(), id) {
		t.Fatalf("withdraw error %q must name the committed cede id %q", withdrawErr, id)
	}

	st2, err := routerstore.OpenPath(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	withdrawn, err := schedule.NewLedger(st2).ListCedes(schedule.CedeFilter{Resource: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(withdrawn) != 1 || withdrawn[0].Status != schedule.CedeStatusWithdrawn {
		t.Fatalf("the withdraw transition must still have committed despite the projection failure, got %+v", withdrawn)
	}
}
