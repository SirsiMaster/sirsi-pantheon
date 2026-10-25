package maat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryQualificationSharesLinkPassBeforeWindow(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "decisions.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	const records = 384
	encoder := json.NewEncoder(f)
	for i := 0; i < records; i++ {
		d := Decision{Time: time.Date(2026, 9, 1, 0, i, 0, 0, time.UTC).Format(time.RFC3339Nano), Host: "fixture", Kind: "allocation", Requester: "inert", Assessed: "local", Determination: "deny", Why: "unrelated body", Evidence: fmt.Sprintf("allocation:%d", i)}
		if i == 0 {
			d.Kind = "diagnostic owner review"
			d.Evidence = "review:old"
		}
		if i == records-1 {
			d.Kind = "diagnostic owner acceptance"
			d.Evidence = "acceptance:new"
			d.ResolutionFor = "review:old"
			d.Why = "late linked conclusion"
			d.Determination = "accepted"
		}
		if err = encoder.Encode(d); err != nil {
			t.Fatal(err)
		}
	}
	if err = f.Sync(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	j := &FileDecisionJournal{Path: path}
	q := HistoryQuery{Scope: HistoryScopeAll, QueryKey: "batch-linked-text", Limit: 1, RowBudget: 512,
		MatchState: func(d Decision, links HistoryMatchState) bool {
			return !links.Hidden && links.Resolution != nil && links.Resolution.Why == "late linked conclusion"
		},
	}
	work := 0
	progress := map[string]bool{}
	for call := 0; call < 80; call++ {
		page, err := j.QueryHistory(q)
		if err != nil {
			t.Fatal(err)
		}
		work += page.ScannedRecords
		if page.ScannedRecords > q.RowBudget {
			t.Fatalf("row budget expanded: %d", page.ScannedRecords)
		}
		// Fixed structural bound, not an empirical speed assertion. Independent
		// full-journal work per candidate exceeds it long before completion.
		if work > records*20 {
			t.Fatalf("candidate-by-candidate relationship rescans: %d records", work)
		}
		if page.Coverage == HistoryCoverageComplete {
			if len(page.Candidates) != 1 || page.Candidates[0].Decision.Evidence != "review:old" || len(page.Links.Acceptances) != 1 || page.Links.Acceptances[0].Why != "late linked conclusion" || page.MatchCount != 1 {
				t.Fatalf("linked qualification/window changed: %+v", page)
			}
			return
		}
		if len(page.Candidates) != 0 || page.Cursor == "" || progress[page.Cursor] {
			t.Fatal("provisional or nonprogressing continuation")
		}
		progress[page.Cursor] = true
		state, err := decodeHistoryCursor(page.Cursor)
		if err != nil {
			t.Fatal(err)
		}
		if len(state.Batch) > maxHistoryQualificationBatch || len(state.BatchLinks) > maxHistoryQualificationBatch || len(state.Candidates) > 2 || len(page.Cursor) > maxHistoryCursorBytes {
			t.Fatal("batch/reference state exceeded bound")
		}
		q.Cursor = page.Cursor
	}
	t.Fatal("bounded linked query did not finish")
}

func TestHistoryQualificationBatchCursorCancelAndMalformedState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	j := &FileDecisionJournal{Path: filepath.Join(root, "decisions.jsonl")}
	appendHistoryFixtures(t, j, historyFixtureScreen(t, "2026-10-01T00:00:00Z", "screen:batch"))
	query := HistoryQuery{Scope: HistoryScopeAll, QueryKey: "cancel-batch", Limit: 1, RowBudget: 1, MatchState: func(Decision, HistoryMatchState) bool { return true }}
	first, err := j.QueryHistory(query)
	if err != nil || first.Cursor == "" || len(first.Candidates) != 0 {
		t.Fatalf("partial=%+v %v", first, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	query.Context = ctx
	query.Cursor = first.Cursor
	paused, err := j.QueryHistory(query)
	if err != nil || paused.Coverage != HistoryCoveragePartial || len(paused.Candidates) != 0 || paused.ScannedRecords != 0 {
		t.Fatalf("cancelled read materialized=%+v %v", paused, err)
	}
	query.Context = context.Background()
	state, err := decodeHistoryCursor(first.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	state.Phase = "batch-finalize"
	state.BatchIndex = 2
	malformed, err := encodeHistoryCursor(state)
	if err != nil {
		t.Fatal(err)
	}
	query.Cursor = malformed
	if _, err = j.QueryHistory(query); err == nil {
		t.Fatal("malformed batch accepted")
	}
	state.Version = 3
	state.BatchIndex = 0
	legacy, err := encodeHistoryCursor(state)
	if err != nil {
		t.Fatal(err)
	}
	query.Cursor = legacy
	if _, err = j.QueryHistory(query); err != ErrHistorySnapshotChanged {
		t.Fatalf("old cursor did not require restart: %v", err)
	}
}
