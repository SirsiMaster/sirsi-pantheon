package maat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiagnosticDiscovery129CompletedRecordsAndLostReplyBeyondFirstPage(t *testing.T) {
	s, m, _, j, _ := diagnosticRecordFixture(t)
	for i := 0; i < 130; i++ {
		m.Observation.SourceSHA256 = fmt.Sprintf("%064x", i+100)
		m.Observation.ObservedAtUTC = time.Now().UTC()
		requestID := fmt.Sprintf("%08x-0000-4000-8000-%012x", i+1, i+1)
		if i == 129 {
			requestID = diagnosticRequestTestID
			j.lostAck = true
		}
		if _, err := s.PrepareDiagnosticRequest(requestID); err != nil {
			t.Fatal(err)
		}
		r, err := s.BeginDiagnosticRequest(requestID)
		if err != nil {
			t.Fatal(err)
		}
		_, recordErr := r.Record(m.Observation, j, func() error { return nil })
		closeErr := r.Close()
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		if i == 129 {
			if !errors.Is(recordErr, ErrDiagnosticDeliveryUnknown) {
				t.Fatal(recordErr)
			}
		} else if recordErr != nil {
			t.Fatal(recordErr)
		}
	}
	if j.count() != 130 {
		t.Fatal("fixture did not record129 completed rows plus the lost-ack row", j.count())
	}
	reopened, err := OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	query := DiagnosticDiscoveryQuery{Limit: 128}
	first, err := reopened.QueryDiagnosticDiscovery(query)
	if err != nil {
		t.Fatal(err)
	}
	if first.History.Coverage != HistoryCoveragePartial || first.History.NextCursor == "" || first.History.AfterCursor || first.History.Returned != 128 || len(first.Requests) != 0 {
		t.Fatal("first page falsely complete/truncated", first.History)
	}
	records := map[string]bool{}
	requests := map[string]bool{}
	page := first
	pages := 0
	pendingFound := false
	completed := 0
	pending := 0
	completedRequests := 0
	for {
		pages++
		if pages > 8 {
			t.Fatal("discovery failed to make bounded progress")
		}
		if page.History.QueryID != first.History.QueryID || page.History.Snapshot != first.History.Snapshot || page.History.CountScope != "page" || page.History.Returned > 128 {
			t.Fatal("query/snapshot/count scope drift", page.History)
		}
		for _, record := range page.Records {
			if records[record.Intent.ID] {
				t.Fatal("duplicate record across pages")
			}
			records[record.Intent.ID] = true
			if record.State == "recorded_verified" && record.Result != nil {
				completed++
			}
			if record.State == "pending" && record.Result == nil {
				pending++
			}
		}
		for _, request := range page.Requests {
			if requests[request.Request.ID] {
				t.Fatal("duplicate request across pages")
			}
			requests[request.Request.ID] = true
			if request.State == "recorded_verified" && request.Result != nil {
				completedRequests++
			}
			if request.Request.ID == diagnosticRequestTestID {
				pendingFound = pages > 1 && request.State == "pending" && request.Intent != nil && request.Result == nil
			}
		}
		if page.History.Coverage == HistoryCoverageComplete {
			if page.History.NextCursor != "" {
				t.Fatal("complete page retained cursor")
			}
			break
		}
		if page.History.Coverage != HistoryCoveragePartial || page.History.NextCursor == "" {
			t.Fatal("partial page has no continuation")
		}
		query.Cursor = page.History.NextCursor
		page, err = reopened.QueryDiagnosticDiscovery(query)
		if err != nil {
			t.Fatal(err)
		}
		if !page.History.AfterCursor {
			t.Fatal("continuation presented as initial query")
		}
	}
	if len(records) != 130 || completed != 129 || pending != 1 || len(requests) != 130 || completedRequests != 129 || !pendingFound || j.count() != 130 {
		t.Fatal("restart discovery omitted or replayed old lost-reply record", len(records), completed, pending, len(requests), completedRequests, pendingFound, j.count())
	}
}

func TestDiagnosticDiscoveryChangedSnapshotRefusesThenFreshQueryRestarts(t *testing.T) {
	s, _, _, j, _ := diagnosticRecordFixture(t)
	for _, id := range []string{diagnosticRequestTestID, "790c358a-d905-4546-871d-de40a76cc0ac"} {
		if _, err := s.PrepareDiagnosticRequest(id); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.QueryDiagnosticDiscovery(DiagnosticDiscoveryQuery{Limit: 1})
	if err != nil || first.History.NextCursor == "" {
		t.Fatal(first.History, err)
	}
	if _, err := s.PrepareDiagnosticRequest("890c358a-d905-4546-871d-de40a76cc0ac"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.QueryDiagnosticDiscovery(DiagnosticDiscoveryQuery{Limit: 1, Cursor: first.History.NextCursor}); !errors.Is(err, ErrHistorySnapshotChanged) {
		t.Fatal("changed query continued", err)
	}
	fresh, err := s.QueryDiagnosticDiscovery(DiagnosticDiscoveryQuery{Limit: 1})
	if err != nil || fresh.History.AfterCursor || fresh.History.Snapshot == first.History.Snapshot || fresh.History.QueryID != first.History.QueryID {
		t.Fatal("fresh restart not explicit", fresh.History, err)
	}
	if j.count() != 0 {
		t.Fatal("discovery mutated journal")
	}
}

func TestDiagnosticDiscoverySameBytesSameMtimeRewriteStillInvalidatesCursor(t *testing.T) {
	s, _, _, _, root := diagnosticRecordFixture(t)
	for _, id := range []string{diagnosticRequestTestID, "790c358a-d905-4546-871d-de40a76cc0ac"} {
		if _, err := s.PrepareDiagnosticRequest(id); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.QueryDiagnosticDiscovery(DiagnosticDiscoveryQuery{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "attempts", diagnosticRequestTestID+diagnosticRequestSuffix)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.QueryDiagnosticDiscovery(DiagnosticDiscoveryQuery{Limit: 1, Cursor: first.History.NextCursor}); !errors.Is(err, ErrHistorySnapshotChanged) {
		t.Fatal("ctime rewrite was not detected", err)
	}
}

func TestDiagnosticDiscoveryCursorClosedQueryAndCanceledReadRefuse(t *testing.T) {
	s, _, _, _, root := diagnosticRecordFixture(t)
	for _, id := range []string{diagnosticRequestTestID, "790c358a-d905-4546-871d-de40a76cc0ac"} {
		if _, err := s.PrepareDiagnosticRequest(id); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.QueryDiagnosticDiscovery(DiagnosticDiscoveryQuery{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, cursor := range []string{"not-base64", first.History.NextCursor + "=", base64.RawURLEncoding.EncodeToString([]byte(`{"Schema":"alias"}`))} {
		if _, err := s.QueryDiagnosticDiscovery(DiagnosticDiscoveryQuery{Limit: 1, Cursor: cursor}); err == nil {
			t.Fatal("invalid cursor accepted")
		}
	}
	if _, err := s.QueryDiagnosticDiscovery(DiagnosticDiscoveryQuery{Limit: 2, Cursor: first.History.NextCursor}); err == nil {
		t.Fatal("changed page limit accepted")
	}
	raw, err := base64.RawURLEncoding.DecodeString(first.History.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	var cursor diagnosticDiscoveryCursor
	if err = json.Unmarshal(raw, &cursor); err != nil {
		t.Fatal(err)
	}
	cursor.After = "request:nonexistent"
	raw, err = json.Marshal(cursor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.QueryDiagnosticDiscovery(DiagnosticDiscoveryQuery{Limit: 1, Cursor: base64.RawURLEncoding.EncodeToString(raw)}); err == nil {
		t.Fatal("unrelated cursor boundary accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.QueryDiagnosticDiscovery(DiagnosticDiscoveryQuery{Context: ctx}); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled read accepted", err)
	}
	// A page budget cannot hide invalid custody sorted after the first row.
	if err := os.WriteFile(filepath.Join(root, "attempts", "790c358a-d905-4546-871d-de40a76cc0ac"+diagnosticRequestSuffix), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.QueryDiagnosticDiscovery(DiagnosticDiscoveryQuery{Limit: 1}); err == nil {
		t.Fatal("paging bypassed later-row namespace validation")
	}
}
