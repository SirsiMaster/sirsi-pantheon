package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
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

func validCedeDecisionRecord() cedeDecisionRecord {
	return cedeDecisionRecord{
		Time: "2026-10-31T12:00:00Z", Host: "fixture", Kind: "cede-request",
		Determination: "pending", Requester: "fixture-actor", Resource: "fixture-resource",
		Assessed: "fixture assessment", Affected: "fixture-peer", Why: "fixture event", Evidence: "fixture-evidence",
	}
}

// Exercise the actual cede projection writer without a cede or reservation
// transition. Only the owned temporary journal is written.
func TestCedeDecisionJournalStrictCompatibility(t *testing.T) {
	for _, kind := range []string{"cede-request", "cede-grant", "cede-counter", "cede-decline", "cede-withdraw"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "decisions.jsonl")
			t.Setenv("MAAT_DECISIONS", path)
			if err := logCedeDecision(kind, "fixture-outcome", "fixture-actor", "fixture-resource", "inert projection", "fixture-peer", "fixture explanation", "fixture-evidence"); err != nil {
				t.Fatal(err)
			}
			rows, err := (&maat.FileDecisionJournal{Path: path}).Recent(10)
			if err != nil {
				t.Fatalf("generated cede record must satisfy strict reader: %v", err)
			}
			if len(rows) != 1 {
				t.Fatalf("got %d records, want 1", len(rows))
			}
			if got := rows[0]; got.Kind != kind || got.Requester != "fixture-actor" || got.Affected != "fixture-peer" || got.Evidence != "fixture-evidence" {
				t.Fatalf("actor/evidence changed: %+v", got)
			}
		})
	}
}

func TestCedeDecisionWhyIsFactualWhenOptionalReasonIsEmpty(t *testing.T) {
	for _, kind := range []string{"cede-request", "cede-grant", "cede-counter", "cede-decline", "cede-withdraw"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "decisions.jsonl")
			t.Setenv("MAAT_DECISIONS", path)
			counter := ""
			if kind == "cede-counter" {
				counter = "cores:2"
			}
			if err := logCedeDecision(kind, "fixture-outcome", "fixture-actor", "fixture-resource", "fixture assessment", "fixture-peer", " \t ", "fixture-id"); err != nil {
				t.Fatal(err)
			}
			rows, err := (&maat.FileDecisionJournal{Path: path}).Recent(10)
			if err != nil || len(rows) != 1 {
				t.Fatalf("strict cede history = %+v, err=%v", rows, err)
			}
			if strings.TrimSpace(rows[0].Why) == "" || !strings.Contains(rows[0].Why, "No nonblank reason was supplied") {
				t.Fatalf("empty reason did not produce a factual event description: %q", rows[0].Why)
			}
			if kind == "cede-counter" {
				why, err := cedeDecisionWhy(kind, "fixture-actor", "fixture-resource", "fixture-id", " \t ", counter)
				if err != nil || !strings.Contains(why, "cores:2") {
					t.Fatalf("counter response lost its recorded offer: %q, err=%v", why, err)
				}
			}
		})
	}
}

func TestCedeDecisionWhyPreservesSuppliedReasonWithoutInventingWithdrawalMotive(t *testing.T) {
	for _, kind := range []string{"cede-request", "cede-grant", "cede-counter", "cede-decline", "cede-withdraw"} {
		t.Run(kind, func(t *testing.T) {
			why, err := cedeDecisionWhy(kind, "fixture-actor", "fixture-resource", "fixture-id", "  owner supplied context  ", "")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(why, "owner supplied context") {
				t.Fatalf("supplied reason was lost: %q", why)
			}
			if kind == "cede-withdraw" && !strings.Contains(why, "Original request reason") {
				t.Fatalf("request context was misrepresented as withdrawal motive: %q", why)
			}
		})
	}
}

func TestCedeCommandsProjectOptionalResponseReasons(t *testing.T) {
	t.Setenv("SIRSI_ROUTER_URL", "")
	oldRequester, oldHolder, oldAsk, oldMinutes, oldEarliest, oldReason :=
		cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeEarliest, cedeReason
	oldStart, oldCounter, oldJSON := cedeStart, cedeCounter, maatJSON
	t.Cleanup(func() {
		cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeEarliest, cedeReason =
			oldRequester, oldHolder, oldAsk, oldMinutes, oldEarliest, oldReason
		cedeStart, cedeCounter, maatJSON = oldStart, oldCounter, oldJSON
	})
	maatJSON = false

	for _, operation := range []string{"grant", "counter", "decline", "withdraw"} {
		t.Run(operation, func(t *testing.T) {
			db := filepath.Join(t.TempDir(), "router.db")
			decisions := filepath.Join(t.TempDir(), "decisions.jsonl")
			t.Setenv("SIRSI_ROUTER_DB", db)
			t.Setenv("MAAT_DECISIONS", decisions)
			cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeEarliest, cedeReason =
				"fixture-requester", "fixture-holder", "machine", 10, "", "original request context"
			cedeStart, cedeCounter = "after my current run", "cores:2 for 20m"
			if err := maatCedeRequestCmd.RunE(maatCedeRequestCmd, []string{"fixture-resource"}); err != nil {
				t.Fatal(err)
			}

			store, err := routerstore.OpenPath(db)
			if err != nil {
				t.Fatal(err)
			}
			pending, err := schedule.NewLedger(store).ListCedes(schedule.CedeFilter{Resource: "fixture-resource", PendingOnly: true})
			if closeErr := store.Close(); err == nil {
				err = closeErr
			}
			if err != nil || len(pending) != 1 {
				t.Fatalf("pending request = %+v, err=%v", pending, err)
			}
			id := pending[0].ID

			cedeReason = " \t "
			var runErr error
			switch operation {
			case "grant":
				cedeHolder = "fixture-holder"
				runErr = maatCedeGrantCmd.RunE(maatCedeGrantCmd, []string{id})
			case "counter":
				cedeHolder = "fixture-holder"
				runErr = maatCedeCounterCmd.RunE(maatCedeCounterCmd, []string{id})
			case "decline":
				cedeHolder = "fixture-holder"
				runErr = maatCedeDeclineCmd.RunE(maatCedeDeclineCmd, []string{id})
			case "withdraw":
				cedeRequester = "fixture-requester"
				runErr = maatCedeWithdrawCmd.RunE(maatCedeWithdrawCmd, []string{id})
			}
			if runErr != nil {
				t.Fatal(runErr)
			}

			rows, err := (&maat.FileDecisionJournal{Path: decisions}).Recent(10)
			if err != nil || len(rows) != 2 {
				t.Fatalf("strict decision history = %+v, err=%v", rows, err)
			}
			var request, response maat.Decision
			for _, row := range rows {
				if row.Evidence != id {
					continue
				}
				switch row.Kind {
				case "cede-request":
					if request.Kind != "" {
						t.Fatalf("duplicate request projection for %s", id)
					}
					request = row
				case "cede-" + operation:
					if response.Kind != "" {
						t.Fatalf("duplicate %s projection for %s", operation, id)
					}
					response = row
				}
			}
			if request.Kind != "cede-request" || response.Kind != "cede-"+operation {
				t.Fatalf("request/response projections not uniquely found: request=%+v response=%+v", request, response)
			}
			if strings.TrimSpace(response.Why) == "" || !strings.Contains(response.Why, "No ") {
				t.Fatalf("empty response reason was not projected as a factual event: %+v", response)
			}
			if request.Why != "original request context" {
				t.Fatalf("supplied request context changed: %q", request.Why)
			}
			if operation == "withdraw" && (!strings.Contains(response.Why, "No withdrawal reason was supplied") || !strings.Contains(response.Why, "Original request reason: original request context")) {
				t.Fatalf("withdrawal confused request context with withdrawal motive: %q", response.Why)
			}
			if operation == "counter" && !strings.Contains(response.Why, "cores:2 for 20m") {
				t.Fatalf("counter projection lost its recorded offer: %q", response.Why)
			}
		})
	}
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

	var req, grant cedeDecisionRecord
	for _, row := range lines {
		if row.Evidence != id {
			continue
		}
		switch row.Kind {
		case "cede-request":
			if req.Kind != "" {
				t.Fatalf("duplicate request projection for %s", id)
			}
			req = row
		case "cede-grant":
			if grant.Kind != "" {
				t.Fatalf("duplicate grant projection for %s", id)
			}
			grant = row
		}
	}
	if req.Kind != "cede-request" || req.Determination != "pending" {
		t.Fatalf("request projection must be pending, got %+v", req)
	}
	if req.Requester != "claude-io" || req.Affected != "sne" || req.Resource != "m1" || req.Evidence != id {
		t.Fatalf("request line fields wrong: %+v", req)
	}

	if grant.Kind != "cede-grant" || grant.Determination != "grant" {
		t.Fatalf("grant projection must record a grant, got %+v", grant)
	}
	if grant.Requester != "sne" || grant.Affected != "claude-io" || grant.Resource != "m1" || grant.Evidence != id {
		t.Fatalf("grant line fields wrong: %+v", grant)
	}
	if req.Why != "need the box for H9" || grant.Why != "sure, after my run" {
		t.Fatalf("supplied cede reasons changed: request=%q grant=%q", req.Why, grant.Why)
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

	err := appendCedeDecision(validCedeDecisionRecord())
	if err == nil || !strings.Contains(err.Error(), "create parent") {
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

	err := appendCedeDecision(validCedeDecisionRecord())
	if err == nil || !strings.Contains(err.Error(), "open") {
		t.Fatalf("want an open failure, got %v", err)
	}
}

func TestAppendCedeDecisionRejectsInvalidCanonicalRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	t.Setenv("MAAT_DECISIONS", path)
	rec := validCedeDecisionRecord()
	rec.Why = " \t "
	if err := appendCedeDecision(rec); err == nil || !strings.Contains(err.Error(), "why is required") {
		t.Fatalf("canonical validation did not reject empty why: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid projection created a journal: stat err=%v", err)
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

func TestCedeDecisionLedger_CounterAndDeclineProjectionFailuresDoNotRetry(t *testing.T) {
	t.Setenv("SIRSI_ROUTER_URL", "")
	oldRequester, oldHolder, oldAsk, oldMinutes, oldEarliest, oldReason :=
		cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeEarliest, cedeReason
	oldStart, oldCounter, oldJSON, oldAppend := cedeStart, cedeCounter, maatJSON, appendCedeDecision
	t.Cleanup(func() {
		cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeEarliest, cedeReason =
			oldRequester, oldHolder, oldAsk, oldMinutes, oldEarliest, oldReason
		cedeStart, cedeCounter, maatJSON, appendCedeDecision = oldStart, oldCounter, oldJSON, oldAppend
	})
	maatJSON = false

	for _, operation := range []struct {
		name   string
		cmd    *cobra.Command
		status schedule.CedeStatus
	}{
		{name: "counter", cmd: maatCedeCounterCmd, status: schedule.CedeStatusCountered},
		{name: "decline", cmd: maatCedeDeclineCmd, status: schedule.CedeStatusDeclined},
	} {
		t.Run(operation.name, func(t *testing.T) {
			db := filepath.Join(t.TempDir(), "router.db")
			decisions := filepath.Join(t.TempDir(), "decisions.jsonl")
			t.Setenv("SIRSI_ROUTER_DB", db)
			t.Setenv("MAAT_DECISIONS", decisions)
			cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeEarliest, cedeReason =
				"fixture-requester", "fixture-holder", "machine", 10, "", "projection failure fixture"
			cedeCounter = "cores:2 for 20m"
			oldAppend := appendCedeDecision
			appendCedeDecision = func(record cedeDecisionRecord) error {
				if record.Kind == "cede-request" {
					return oldAppend(record)
				}
				return os.ErrPermission
			}
			defer func() { appendCedeDecision = oldAppend }()
			if err := maatCedeRequestCmd.RunE(maatCedeRequestCmd, []string{"fixture-resource"}); err != nil {
				t.Fatal(err)
			}
			store, err := routerstore.OpenPath(db)
			if err != nil {
				t.Fatal(err)
			}
			pending, err := schedule.NewLedger(store).ListCedes(schedule.CedeFilter{Resource: "fixture-resource", PendingOnly: true})
			if closeErr := store.Close(); err == nil {
				err = closeErr
			}
			if err != nil || len(pending) != 1 {
				t.Fatalf("pending request = %+v, err=%v", pending, err)
			}
			id := pending[0].ID
			cedeHolder = "fixture-holder"
			err = operation.cmd.RunE(operation.cmd, []string{id})
			if err == nil || !strings.Contains(err.Error(), id) || !strings.Contains(err.Error(), "do not retry") {
				t.Fatalf("post-commit projection failure did not retain no-retry identity: %v", err)
			}
			store, err = routerstore.OpenPath(db)
			if err != nil {
				t.Fatal(err)
			}
			rows, listErr := schedule.NewLedger(store).ListCedes(schedule.CedeFilter{Resource: "fixture-resource"})
			if closeErr := store.Close(); listErr == nil {
				listErr = closeErr
			}
			if listErr != nil || len(rows) != 1 || rows[0].Status != operation.status {
				t.Fatalf("scheduler transition was not preserved: rows=%+v err=%v", rows, listErr)
			}
		})
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
