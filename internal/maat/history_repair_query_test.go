package maat

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func historyRepairFixture(t *testing.T, terminal bool) (*FileDecisionJournal, Decision, Decision) {
	t.Helper()
	journal := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	screen := historyFixtureScreen(t, "2026-09-01T00:00:00Z", "screen:binary-history")
	appendHistoryFixtures(t, journal, screen)
	for index := 0; index < 25; index++ {
		appendHistoryFixtures(t, journal, Decision{Time: "2027-01-01T00:00:00Z", Host: "fixture", Kind: "reservation", Requester: "fixture", Assessed: "noise", Determination: "grant", Why: "unrelated", Evidence: "noise:" + strconv.Itoa(index)})
	}
	link := Decision{Time: "2026-09-01T00:01:00Z", Host: "fixture", Kind: "diagnostic repair attempt", Requester: "fixture", Resource: "binary-drift", Assessed: "inert updater", Determination: "in_progress", Why: "inert attempt", Evidence: "attempt:binary", OriginEvidence: screen.Evidence}
	appendHistoryFixtures(t, journal, link)
	if terminal {
		older := link
		older.Kind, older.Determination, older.Evidence, older.Time = "diagnostic verified repair", "resolved", "receipt:older", "2026-09-01T00:00:30Z"
		appendHistoryFixtures(t, journal, older)
		link.Kind, link.Determination, link.Evidence, link.Time = "diagnostic verified repair", "failed", "receipt:binary", "2026-09-01T00:02:00Z"
		appendHistoryFixtures(t, journal, link)
		duplicateTime := link
		duplicateTime.Evidence, duplicateTime.Determination = "receipt:later-physical-tie", "resolved"
		appendHistoryFixtures(t, journal, duplicateTime)
	}
	wrong := link
	wrong.Time, wrong.Resource, wrong.Evidence = "2028-01-01T00:00:00Z", "liveness-watch", "receipt:wrong-resource"
	appendHistoryFixtures(t, journal, wrong)
	wrong.Resource, wrong.OriginEvidence, wrong.Evidence = "binary-drift", "screen:wrong", "receipt:wrong-origin"
	appendHistoryFixtures(t, journal, wrong)
	return journal, screen, link
}

func TestQueryHistoryResumesBinaryRepairStateBeforeWindow(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(strconv.FormatBool(terminal), func(t *testing.T) {
			journal, screen, expected := historyRepairFixture(t, terminal)
			query := HistoryQuery{Scope: HistoryScopeSystemOne, Evidence: screen.Evidence, QueryKey: "repair-filter", Limit: 1, ByteBudget: 512, RowBudget: 2, TimeBudget: time.Second,
				NeedsState: func(Decision) bool { return true },
				MatchState: func(_ Decision, linked HistoryMatchState) bool {
					if terminal {
						return linked.RepairOutcome != nil && linked.RepairOutcome.Evidence == expected.Evidence && linked.RepairAttempt == nil
					}
					return linked.RepairAttempt != nil && linked.RepairAttempt.Evidence == expected.Evidence && linked.RepairOutcome == nil
				},
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			query.Context = ctx
			canceled, err := journal.QueryHistory(query)
			if err != nil || canceled.Coverage != HistoryCoveragePartial || len(canceled.Candidates) != 0 || canceled.Cursor == "" {
				t.Fatalf("canceled history exposed provisional results: %+v / %v", canceled, err)
			}
			query.Context, query.Cursor = context.Background(), canceled.Cursor
			for step := 0; step < 512; step++ {
				page, err := journal.QueryHistory(query)
				if err != nil {
					t.Fatal(err)
				}
				if page.Coverage == HistoryCoverageComplete {
					if len(page.Candidates) != 1 || page.MatchCount != 1 || page.HasMore || page.EvidenceMatches != 1 {
						t.Fatalf("repair-filtered page = %+v", page)
					}
					if terminal {
						if len(page.Links.RepairOutcomes) != 1 || page.Links.RepairOutcomes[0].Evidence != expected.Evidence || len(page.Links.RepairAttempts) != 0 {
							t.Fatalf("terminal links = %+v", page.Links)
						}
					} else if len(page.Links.RepairAttempts) != 1 || page.Links.RepairAttempts[0].Evidence != expected.Evidence || len(page.Links.RepairOutcomes) != 0 {
						t.Fatalf("pending links = %+v", page.Links)
					}
					return
				}
				if len(page.Candidates) != 0 || len(page.Links.RepairOutcomes)+len(page.Links.RepairAttempts) != 0 || page.Cursor == "" || page.Cursor == query.Cursor {
					t.Fatalf("provisional/stalled page: %+v", page)
				}
				state, err := decodeHistoryCursor(page.Cursor)
				if err != nil || state == nil || state.Version != historyCursorVersion || len(state.Candidates) > 2 || len(state.Links) > 2 || !exclusiveHistoryOutcome(state.PendingAcceptance, state.PendingRepairOutcome, state.PendingRepairAttempt) {
					t.Fatalf("unbounded or conflicting retained state: %+v / %v", state, err)
				}
				query.Cursor = page.Cursor
			}
			t.Fatal("repair continuation failed to complete")
		})
	}
}

func TestHistoryRepairMaterializationChargesAndRevalidatesSelectedLink(t *testing.T) {
	journal, screen, _ := historyRepairFixture(t, true)
	file, err := os.Open(journal.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	state := &historyCursor{Version: historyCursorVersion, Snapshot: historySnapshotID(journal.Path, info), QueryKey: "repair-bundle", Phase: "scan", Limit: 1}
	deadline := time.Now().Add(time.Second)
	done, _, _, err := scanHistoryChunk(context.Background(), deadline, file, state, defaultHistoryByteBudget, 100, HistoryScopeSystemOne, screen.Evidence, nil, nil, nil)
	if err != nil || !done || len(state.Candidates) != 1 {
		t.Fatalf("prepare source: %+v %v", state, err)
	}
	state.Offset, state.Links = 0, makeHistoryLinkRefs(state.Candidates)
	done, _, _, err = scanHistoryLinks(context.Background(), deadline, file, state, defaultHistoryByteBudget, 100)
	if err != nil || !done || state.Links[0].RepairOutcome == nil || state.Links[0].RepairAttempt != nil {
		t.Fatalf("prepare exact terminal link: %+v %v", state.Links, err)
	}
	query := HistoryQuery{Scope: HistoryScopeSystemOne, Evidence: screen.Evidence, Limit: 1, ByteBudget: 1, RowBudget: 1}
	page := HistoryPage{}
	limited, consumed, rows, err := materializeHistoryPage(context.Background(), deadline, file, journal.Path, state.Snapshot, state, query, &page, 1, 1)
	wantBytes := state.Candidates[0].Order.Size + state.Links[0].RepairOutcome.Size
	if err != nil || limited || rows != 2 || consumed != wantBytes || len(page.Candidates) != 1 || len(page.Links.RepairOutcomes) != 1 {
		t.Fatalf("atomic repair bundle uncharged: rows=%d bytes=%d page=%+v err=%v", rows, consumed, page, err)
	}
	bad := state.Candidates[0].Order
	state.Links[0].RepairOutcome = &bad
	if _, _, _, err := materializeHistoryPage(context.Background(), deadline, file, journal.Path, state.Snapshot, state, query, &HistoryPage{}, 1, 1); !errors.Is(err, ErrHistorySnapshotChanged) {
		t.Fatalf("substituted linked record accepted: %v", err)
	}
}

func TestHistoryRepairCursorVersionAndReadOnlyIntegrity(t *testing.T) {
	journal, screen, _ := historyRepairFixture(t, false)
	file, err := os.OpenFile(journal.Path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("{broken linked row}\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(journal.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(journal.Path, 0o400); err != nil {
		t.Fatal(err)
	}
	query := HistoryQuery{Scope: HistoryScopeSystemOne, Evidence: screen.Evidence, QueryKey: "repair-read-only", Limit: 1}
	page, err := journal.QueryHistory(query)
	if err != nil || page.Coverage != HistoryCoverageComplete || page.Integrity.InvalidCount != 1 || len(page.Links.RepairAttempts) != 1 {
		t.Fatalf("read-only malformed link handling = %+v / %v", page, err)
	}
	after, err := os.ReadFile(journal.Path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("read-only history changed retained journal bytes")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	query.Context = ctx
	partial, err := journal.QueryHistory(query)
	if err != nil {
		t.Fatal(err)
	}
	state, err := decodeHistoryCursor(partial.Cursor)
	if err != nil || state == nil {
		t.Fatalf("missing canceled continuation: %v", err)
	}
	state.Version = 2
	query.Cursor, err = encodeHistoryCursor(state)
	if err != nil {
		t.Fatal(err)
	}
	query.Context = context.Background()
	if _, err := journal.QueryHistory(query); !errors.Is(err, ErrHistorySnapshotChanged) {
		t.Fatalf("old cursor semantics accepted: %v", err)
	}
	invalid := historyOrder{Time: "fixture", Offset: -1, Size: 1}
	if validHistoryRepairRef(&invalid, int64(len(before))) || exclusiveHistoryOutcome(&invalid, &invalid, nil) || exclusiveHistoryOutcome(nil, &invalid, &invalid) {
		t.Fatal("invalid/conflicting repair reference passed structural validation")
	}
}

func TestHistoryRepairAppendInvalidatesContinuation(t *testing.T) {
	journal, screen, attempt := historyRepairFixture(t, false)
	query := HistoryQuery{Scope: HistoryScopeSystemOne, Evidence: screen.Evidence, QueryKey: "repair-snapshot", Limit: 1, RowBudget: 1, ByteBudget: 1}
	partial, err := journal.QueryHistory(query)
	if err != nil || partial.Coverage != HistoryCoveragePartial || partial.Cursor == "" || len(partial.Candidates) != 0 {
		t.Fatalf("missing initial continuation: %+v / %v", partial, err)
	}
	attempt.Kind, attempt.Determination, attempt.Evidence, attempt.Time = "diagnostic verified repair", "resolved", "receipt:new-terminal", "2026-09-01T00:03:00Z"
	appendHistoryFixtures(t, journal, attempt)
	query.Cursor = partial.Cursor
	if _, err := journal.QueryHistory(query); !errors.Is(err, ErrHistorySnapshotChanged) {
		t.Fatalf("terminal receipt append reused stale continuation: %v", err)
	}
}
