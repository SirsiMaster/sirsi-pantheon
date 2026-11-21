package dashboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
)

func TestHostedRouterSummaryCrossLane(t *testing.T) {
	const private = "distinct-private-fixture-marker"
	const stamp = "2026-11-14T19:00:00Z"
	fabric := ledger.FabricBoard{
		GeneratedAt: stamp, Work: ledger.FabricWork{Open: 7, Done: 3, Total: 10},
		Messages: ledger.FabricMessages{Open: 2},
		Lanes:    []ledger.FabricLane{{Agent: "lane-a", StateLabel: private, Open: 5}, {Agent: "lane-b", Open: 2}},
		Health:   ledger.FabricHealth{Issues: []string{private}},
		Activity: []ledger.FabricActivity{{TaskID: private}},
		Tasks:    ledger.FabricTaskPage{Rows: []ledger.FabricTask{{ID: private, Subject: private}}},
	}
	snap := RouterSnapshot{
		GeneratedAt: stamp, Fabric: &fabric,
		Lanes: RouterLanes{
			Counts: map[string]int{"LIVE": 1, private: 1},
			List:   []RouterLaneVerdict{{Agent: "lane-a", Verdict: "LIVE", Detail: private}, {Agent: "lane-b", Verdict: private}},
		},
		Queue: []RouterQueueRow{{Agent: "lane-a", Open: 1}, {Agent: "lane-b", Open: 1}},
		Items: []RouterQueueItem{
			{ID: private, From: "lane-a", To: "lane-b", Title: private, Instructions: private, BlockedBy: private},
			{From: "lane-b", To: "lane-a", Instructions: private},
		},
		Attention: []RouterAttention{{Title: private}},
	}
	for _, path := range []string{"/api/router", "/api/router/v1/snapshot", "/api/fabric"} {
		t.Run(path, func(t *testing.T) {
			cfg := Config{CloudBoard: true, RouterFn: func() (RouterSnapshot, error) { return snap, nil },
				FabricFn: func() (ledger.FabricBoard, error) { return fabric, nil }}
			for _, hosted := range []bool{true, false} {
				cfg.CloudBoard = hosted
				rec := httptest.NewRecorder()
				New(cfg).handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
				if rec.Code != http.StatusOK {
					t.Fatalf("hosted=%t: HTTP %d", hosted, rec.Code)
				}
				if strings.Contains(rec.Body.String(), private) == hosted {
					t.Fatalf("hosted=%t: wrong private marker disposition", hosted)
				}
				if !hosted {
					continue
				}
				if rec.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("hosted observation must not be cached")
				}
				if path == "/api/fabric" {
					var body hostedFabricSummary
					if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || !body.PrivateWithheld || body.Work.Open != 7 || body.Messages.Open != 2 || len(body.Lanes) != 2 || body.Lanes[0].Open != 5 || body.GeneratedAt != stamp {
						t.Fatal("hosted Fabric must preserve live aggregate counts, lane counts and freshness")
					}
				} else {
					var body hostedRouterSummary
					if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || !body.PrivateWithheld || body.GeneratedAt != stamp || len(body.Queue) != 2 || len(body.Lanes.List) != 2 || body.Lanes.List[1].Verdict != "UNKNOWN" || body.Lanes.Counts["LIVE"] != 1 || body.Fabric == nil || body.Fabric.Work.Done != 3 {
						t.Fatal("hosted router must preserve useful summaries and explicitly label withholding")
					}
				}
				for _, key := range []string{`"items"`, `"title"`, `"instructions"`, `"blocked_by"`, `"tasks"`, `"task_id"`} {
					if strings.Contains(rec.Body.String(), key) {
						t.Fatalf("private field %s present on hosted response", key)
					}
				}
			}
		})
	}
}

func TestHostedRouterSummaryRefusesDetailsBeforeAcquisition(t *testing.T) {
	const private = "private-diagnostic-fixture"
	cfg := Config{
		CloudBoard: true,
		RouterFn:   func() (RouterSnapshot, error) { return RouterSnapshot{}, errors.New(private) },
		FabricFn: func() (ledger.FabricBoard, error) {
			t.Fatal("hosted detail request acquired Fabric")
			return ledger.FabricBoard{}, nil
		},
		LedgerAgentFn: func(string) (ledger.Snapshot, error) {
			t.Fatal("hosted detail request acquired a private lane ledger")
			return ledger.Snapshot{}, nil
		},
	}
	s := New(cfg)
	for path, status := range map[string]int{
		"/api/router": http.StatusBadGateway, "/api/router/v1/snapshot": http.StatusServiceUnavailable,
		"/api/fabric?tasks_filter=open": http.StatusForbidden, "/api/router/v1/ledger?agent=lane-b": http.StatusForbidden,
	} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			s.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != status || strings.Contains(rec.Body.String(), private) || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("unexpected hosted status, error disclosure or cache policy: %d", rec.Code)
			}
		})
	}
}
