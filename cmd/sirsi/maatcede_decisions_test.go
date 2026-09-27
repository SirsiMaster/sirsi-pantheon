package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

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
