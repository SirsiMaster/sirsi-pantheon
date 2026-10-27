package mcp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

// A real Go-encoded canonical private journal, not a fake Recent transport.
// Its oldest record is outside both former consumer windows (50 and 1000).
const surfaceJournalRows = 2012

func surfaceJournal(t *testing.T, large bool) *maat.FileDecisionJournal {
	t.Helper()
	root := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "decisions.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	encoder := json.NewEncoder(file)
	for i := 0; i < surfaceJournalRows; i++ {
		row := maat.Decision{Time: time.Date(2026, 9, 1, 0, i, 0, 0, time.UTC).Format(time.RFC3339Nano), Host: "inert", Kind: "allocation", Requester: "fixture", Assessed: "local capacity", Determination: "deny", Why: "unrelated", Evidence: fmt.Sprintf("allocation:%04d", i)}
		if i == 0 {
			row.Why = "retained-older-match"
			row.Evidence = "old:target"
		}
		if i == 1 {
			row.Kind = "diagnostic owner review"
			row.Why = "owner reviewed"
			row.OriginEvidence = "old:target"
			row.Evidence = "owner:review"
		}
		if i == 2 {
			row.Kind = "diagnostic owner acceptance"
			row.Determination = "accepted"
			row.ResolutionFor = "owner:review"
			row.Evidence = "owner:accepted"
		}
		if large && i > 2 {
			row.Why = strings.Repeat("x", 3000)
		}
		if err = encoder.Encode(row); err != nil {
			t.Fatal(err)
		}
	}
	if err = file.Sync(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(root)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("private journal parent: %v %v", info, err)
	}
	info, err = os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private journal: %v %v", info, err)
	}
	return &maat.FileDecisionJournal{Path: path}
}

func mcpHistory(t *testing.T, args map[string]interface{}) maatHistoryProjection {
	t.Helper()
	result, err := handleMaatCasebook(args)
	if err != nil || result.IsError || len(result.Content) != 1 {
		t.Fatalf("history result=%+v err=%v", result, err)
	}
	var view maatHistoryProjection
	if err = json.Unmarshal([]byte(result.Content[0].Text), &view); err != nil {
		t.Fatal(err)
	}
	return view
}
func TestMaatHistoryMCPFindsOlderMatchBeforeWindow(t *testing.T) {
	journal := surfaceJournal(t, false)
	old := openMaatCasebookJournal
	t.Cleanup(func() { openMaatCasebookJournal = old })
	openMaatCasebookJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	args := map[string]interface{}{"query": "retained-older-match", "limit": float64(1)}
	var view maatHistoryProjection
	for n := 0; n < 80; n++ {
		view = mcpHistory(t, args)
		if view.History.Coverage == "complete" {
			break
		}
		if len(view.Cases) != 0 || view.History.NextCursor == "" {
			t.Fatalf("partial projection=%+v", view)
		}
		args["cursor"] = view.History.NextCursor
	}
	if view.History.Coverage != "complete" || len(view.Cases) != 1 || view.Cases[0].Evidence != "old:target" {
		t.Fatalf("older match omitted: %+v", view)
	}
	if view.History.CountScope != "query" || !view.History.TotalComplete || view.History.Total == nil || *view.History.Total != 1 {
		t.Fatalf("query counts=%+v", view.History)
	}
}
func TestMaatHistoryMCPPartialContinuationAndPlatformSnapshotScope(t *testing.T) {
	journal := surfaceJournal(t, true)
	old := openMaatCasebookJournal
	t.Cleanup(func() { openMaatCasebookJournal = old })
	openMaatCasebookJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	args := map[string]interface{}{"query": "retained-older-match", "limit": float64(1)}
	first := mcpHistory(t, args)
	if first.History.Coverage != "partial" || len(first.Cases) != 0 || first.History.NextCursor == "" {
		t.Fatalf("bounded scan must not claim absence: %+v", first)
	}
	args["cursor"] = first.History.NextCursor
	// Keep the full large journal and production 300ms/4MiB request budgets.
	// Eighty requests was an accidental throughput assertion under race plus
	// coverage on hosted Macs. Bound actual work instead: 64 journal passes
	// covers the 48-reference qualification batches plus scan/link/selected-row
	// rereads, but rejects a whole-journal rescan for each of the 2012 records.
	const maxScannedRecords = surfaceJournalRows * 64
	scanned := first.History.ScannedRecords
	seen := map[string]bool{first.History.NextCursor: true}
	complete := false
	for n := 0; n < maxScannedRecords; n++ {
		page := mcpHistory(t, args)
		scanned += page.History.ScannedRecords
		if scanned > maxScannedRecords {
			t.Fatalf("continuation exceeded structural work bound: %d records", scanned)
		}
		if page.History.Snapshot != first.History.Snapshot || page.History.QueryID != first.History.QueryID {
			t.Fatal("mixed snapshot/query")
		}
		if page.History.Coverage == "complete" {
			if len(page.Cases) != 1 || page.Cases[0].Evidence != "old:target" {
				t.Fatalf("lost old match=%+v", page)
			}
			complete = true
			t.Logf("completed after %d continuation calls and %d scanned records (bound %d)", n+1, scanned, maxScannedRecords)
			break
		}
		if len(page.Cases) != 0 || page.History.NextCursor == "" || page.History.ScannedRecords <= 0 || seen[page.History.NextCursor] {
			t.Fatalf("provisional or nonprogressing scan: %+v", page.History)
		}
		seen[page.History.NextCursor] = true
		args["cursor"] = page.History.NextCursor
	}
	if !complete {
		t.Fatal("bounded continuation did not finish within its structural work bound")
	}
	if err := journal.Append(maat.Decision{Time: "2026-10-25T00:00:00Z", Host: "inert", Kind: "allocation", Requester: "fixture", Assessed: "new", Determination: "deny", Why: "retained-older-match", Evidence: "appended:target"}); err != nil {
		t.Fatal(err)
	}
	continuedArgs := map[string]interface{}{"query": "retained-older-match", "limit": float64(1), "cursor": first.History.NextCursor}
	if runtime.GOOS == "windows" {
		result, err := handleMaatCasebook(continuedArgs)
		if err != nil || !result.IsError {
			t.Fatalf("legacy Windows append did not invalidate continuation: %+v %v", result, err)
		}
		return
	}
	readComplete := func(args map[string]interface{}) maatHistoryProjection {
		var result maatHistoryProjection
		scanned := 0
		for n := 0; n < maxScannedRecords; n++ {
			result = mcpHistory(t, args)
			scanned += result.History.ScannedRecords
			if scanned > maxScannedRecords {
				t.Fatalf("continuation exceeded structural work bound: %d records", scanned)
			}
			if result.History.Coverage == "complete" {
				return result
			}
			if len(result.Cases) != 0 || result.History.NextCursor == "" {
				t.Fatalf("frozen/fresh continuation stalled or exposed provisional results: %+v", result.History)
			}
			args["cursor"] = result.History.NextCursor
		}
		t.Fatal("bounded history continuation did not finish within its structural work bound")
		return maatHistoryProjection{}
	}
	frozen := readComplete(continuedArgs)
	if frozen.History.Snapshot != first.History.Snapshot || len(frozen.Cases) != 1 || frozen.Cases[0].Evidence != "old:target" {
		t.Fatalf("append changed the frozen MCP result: %+v", frozen)
	}
	fresh := readComplete(map[string]interface{}{"query": "retained-older-match", "limit": float64(1)})
	if fresh.History.Snapshot == first.History.Snapshot || len(fresh.Cases) != 1 || fresh.Cases[0].Evidence != "appended:target" {
		t.Fatalf("fresh MCP query did not see appended match: %+v", fresh)
	}
}
func TestMaatHistoryMCPRejectsMalformedBeforeRead(t *testing.T) {
	old := openMaatCasebookJournal
	t.Cleanup(func() { openMaatCasebookJournal = old })
	reads := 0
	openMaatCasebookJournal = func() (maat.DecisionJournal, error) { reads++; return nil, fmt.Errorf("must not read") }
	badCursor := base64.RawURLEncoding.EncodeToString([]byte(`{"v":null,"snapshot":"s","query":"q","phase":"scan","offset":0,"limit":1}`))
	for _, args := range []map[string]interface{}{{"limit": float64(0)}, {"scope": "foreign"}, {"cursor": badCursor}, {"cursor": "not a cursor"}, {"query": strings.Repeat("q", 513)}, {"extra": "ignored"}} {
		result, err := handleMaatCasebook(args)
		if err != nil || !result.IsError {
			t.Fatalf("malformed query accepted: %+v", args)
		}
	}
	if reads != 0 {
		t.Fatalf("malformed requests performed %d reads", reads)
	}
}
func TestMaatHistoryMCPAfterCursorCountsAndQueryBinding(t *testing.T) {
	journal := surfaceJournal(t, false)
	old := openMaatCasebookJournal
	t.Cleanup(func() { openMaatCasebookJournal = old })
	openMaatCasebookJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	args := map[string]interface{}{"kind": "allocation", "limit": float64(1)}
	var first maatHistoryProjection
	for n := 0; n < 80; n++ {
		first = mcpHistory(t, args)
		if first.History.Coverage == "complete" {
			break
		}
		args["cursor"] = first.History.NextCursor
	}
	if first.History.Coverage != "complete" || first.History.NextCursor == "" {
		t.Fatal("no next page")
	}
	args["cursor"] = first.History.NextCursor
	var second maatHistoryProjection
	for n := 0; n < 80; n++ {
		second = mcpHistory(t, args)
		if second.History.Coverage == "complete" {
			break
		}
		args["cursor"] = second.History.NextCursor
	}
	if second.History.CountScope != "after-cursor" || second.History.TotalComplete || second.History.Total != nil || len(second.Cases) != 1 || second.Cases[0].ID == first.Cases[0].ID {
		t.Fatalf("mislabelled or replayed page: %+v", second)
	}
	args["query"] = "changed filters"
	result, err := handleMaatCasebook(args)
	if err != nil || !result.IsError {
		t.Fatal("cursor escaped its query")
	}
}

func TestMaatHistoryMCPSameSnapshotRepairLinks(t *testing.T) {
	journal := surfaceJournal(t, false)
	for i, row := range []maat.Decision{
		{Kind: "diagnostic repair attempt", Determination: "attempted", Evidence: "repair:attempt"},
		{Kind: "diagnostic verified repair", Determination: "resolved", Evidence: "repair:verified"},
	} {
		row.Time = fmt.Sprintf("2026-10-25T00:00:0%dZ", i)
		row.Host = "inert"
		row.Requester = "fixture"
		row.Assessed = "retained old screen"
		row.Why = "independent post-observation"
		row.Resource = "binary-drift"
		row.OriginEvidence = "old:target"
		if err := journal.Append(row); err != nil {
			t.Fatal(err)
		}
	}
	old := openMaatCasebookJournal
	t.Cleanup(func() { openMaatCasebookJournal = old })
	openMaatCasebookJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	args := map[string]interface{}{"evidence": "old:target", "limit": float64(1)}
	for n := 0; n < 80; n++ {
		view := mcpHistory(t, args)
		if view.History.Coverage == "complete" {
			if len(view.Cases) != 1 || view.Cases[0].VerifiedRepairEvidence != "repair:verified" || view.Cases[0].RepairOutcomeEvidence != "repair:verified" {
				t.Fatalf("lost same-snapshot repair links=%+v", view)
			}
			return
		}
		if len(view.Cases) != 0 || view.History.NextCursor == "" {
			t.Fatal("provisional or stalled relationship scan")
		}
		args["cursor"] = view.History.NextCursor
	}
	t.Fatal("bounded repair-link fixture did not finish")
}

func TestMaatHistoryMCPCompleteEmptyIsExplicitArray(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "empty.jsonl")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	old := openMaatCasebookJournal
	t.Cleanup(func() { openMaatCasebookJournal = old })
	openMaatCasebookJournal = func() (maat.DecisionJournal, error) { return &maat.FileDecisionJournal{Path: path}, nil }
	result, err := handleMaatCasebook(nil)
	if err != nil || result.IsError {
		t.Fatalf("empty read=%+v %v", result, err)
	}
	raw := result.Content[0].Text
	if !strings.Contains(raw, `"cases":[]`) || !strings.Contains(raw, `"coverage":"complete"`) {
		t.Fatalf("empty history unavailable/ambiguous: %s", raw)
	}
}
