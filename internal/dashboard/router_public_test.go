package dashboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// In-memory producers and Recorders only: no listener, store, command or host
// consumer. Canary strings are invented fixture data, never real credentials.
func privateRouterFixture() RouterSnapshot {
	stamp := "2026-11-10T19:04:42Z"
	private := "PRIVATE-CANARY: instruction /private/fixture credential lease payload"
	board := ledger.FabricFrom(ledger.Snapshot{GeneratedAt: stamp, Agents: []ledger.Agent{{
		AgentID: "worker", Tasks: []routerstore.Task{{
			Agent: "worker", TaskID: "task-one", Subject: "Public task", Status: "blocked", Updated: stamp,
			BlockedBy: private, Charter: &private, Outline: &private, ResultRef: private,
			Links: []routerstore.TaskLink{{Kind: "evidence", URL: "file:///private/PRIVATE-CANARY"}},
		}},
	}}}, nil, "fixture")
	return RouterSnapshot{
		Fabric: &board, FabricGeneration: "public-generation", GeneratedAt: stamp, BuiltMs: 9, Version: "fixture",
		Timings: map[string]int64{private: 4},
		Lanes: RouterLanes{Counts: map[string]int{"LIVE": 1, private: 2}, List: []RouterLaneVerdict{{
			Agent: "worker", Verdict: "LIVE", Detail: private, Open: 1, ObservedAt: stamp,
			WorkerThreadID: private, LastReportAt: stamp, LastReportSummary: private,
		}}},
		Queue:         []RouterQueueRow{{Agent: "worker", Open: 1}},
		Items:         []RouterQueueItem{{ID: "inbox-one", From: "sender", To: "worker", Title: "Public request", Type: "review", Opened: stamp, AckedAt: stamp, Instructions: private, BlockedBy: private}},
		Consumers:     RouterConsumers{Running: 1, Max: 3},
		Registry:      RouterRegistryPin{Pinned: true, Source: private, Commit: "public-commit", FetchedAt: stamp},
		KnownFailures: []RouterKnownFail{{ID: "failure-one", Title: "Public failure", Status: "open", FixedIn: "next", Guard: private}},
		Swap:          &RouterSwap{At: stamp, Verdict: "pressure", UsedMiB: 3, TotalMiB: 4, FreePct: 25, DeltaPages: 1, Correctness: true},
		Releases:      []RouterRelease{{Version: "v1", Date: "2026-11-10", Items: []string{private}}},
		Attention: []RouterAttention{{ID: "unworked:worker", Severity: "warn", Agent: "worker", Title: "Public attention", Detail: private, Action: private,
			Next: &RouterNext{Kind: "command-copy", Label: private, Command: private, Target: private}}},
	}
}

func TestHostedRouterMetadataBoundary(t *testing.T) {
	for _, path := range []string{"/api/router", "/api/router/v1/snapshot"} {
		for _, hosted := range []bool{true, false} {
			name := "local"
			if hosted {
				name = "hosted"
			}
			t.Run(name+path, func(t *testing.T) {
				snapshot := privateRouterFixture()
				before, err := json.Marshal(snapshot)
				if err != nil {
					t.Fatal(err)
				}
				calls := 0
				s := New(Config{CloudBoard: hosted, RouterFn: func() (RouterSnapshot, error) {
					calls++
					return snapshot, nil
				}})
				rec := httptest.NewRecorder()
				s.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
				if rec.Code != http.StatusOK || calls != 1 {
					t.Fatalf("status=%d producer calls=%d", rec.Code, calls)
				}
				after, err := json.Marshal(snapshot)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("response mutated nested producer input")
				}
				if !hosted {
					var got RouterSnapshot
					if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || !reflect.DeepEqual(got, snapshot) {
						t.Fatalf("local raw response changed: %v", err)
					}
					return
				}
				if strings.Contains(rec.Body.String(), "PRIVATE-CANARY") {
					t.Fatal("hosted response leaked private canary")
				}
				var got hostedRouterSummary
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if !got.PrivateWithheld || got.Fabric == nil || !got.Fabric.PrivateWithheld ||
					got.FabricGeneration != snapshot.FabricGeneration ||
					!reflect.DeepEqual(got.Lanes.Counts, map[string]int{"LIVE": 1}) ||
					len(got.Lanes.List) != 1 || got.Lanes.List[0].Verdict != "LIVE" ||
					len(got.Queue) != 1 || got.Queue[0].Open != 1 || got.BuiltMs != 9 || got.Version != "fixture" {
					t.Fatal("hosted aggregate or withholding marker changed")
				}
				var wire map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &wire); err != nil {
					t.Fatal(err)
				}
				assertNoPrivateRouterKeys(t, wire)
			})
		}
	}
}

func assertNoPrivateRouterKeys(t *testing.T, value any) {
	t.Helper()
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			switch key {
			case "instructions", "blocked_by", "result", "result_ref", "lease", "lease_token", "credentials", "last_report_summary", "worker_thread_id", "detail", "action", "next", "command", "target", "source", "guard", "timings_ms":
				t.Fatalf("private field %q present, even if empty", key)
			}
			assertNoPrivateRouterKeys(t, child)
		}
	case []any:
		for _, child := range v {
			assertNoPrivateRouterKeys(t, child)
		}
	}
}

func TestHostedRouterProducerErrorIsFixedAndLocalErrorUnchanged(t *testing.T) {
	for _, path := range []string{"/api/router", "/api/router/v1/snapshot"} {
		for _, hosted := range []bool{true, false} {
			for _, missing := range []bool{true, false} {
				cfg := Config{CloudBoard: hosted}
				calls := 0
				if !missing {
					cfg.RouterFn = func() (RouterSnapshot, error) {
						calls++
						return privateRouterFixture(), errors.New("PRIVATE-CANARY: /private/fixture failure credential lease payload")
					}
				}
				s := New(cfg)
				rec := httptest.NewRecorder()
				s.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
				status, message, wantCalls := http.StatusServiceUnavailable, "router producer not configured", 0
				if !missing {
					message, wantCalls = "router producer unavailable", 1
					if path == "/api/router" {
						status = http.StatusBadGateway
						if !hosted {
							message = "PRIVATE-CANARY: /private/fixture failure credential lease payload"
						}
					}
				}
				var body map[string]string
				err := json.Unmarshal(rec.Body.Bytes(), &body)
				if err != nil || rec.Code != status || len(body) != 1 || body["error"] != message || calls != wantCalls {
					t.Fatalf("path=%s hosted=%v missing=%v status=%d calls=%d body=%q err=%v", path, hosted, missing, rec.Code, calls, rec.Body.String(), err)
				}
			}
		}
	}
}

func TestHostedRouterSummaryDoesNotAliasLocalMetadata(t *testing.T) {
	snapshot := privateRouterFixture()
	before, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	s := New(Config{CloudBoard: true})
	public, ok := s.routerSnapshotResponse(snapshot).(hostedRouterSummary)
	if !ok {
		t.Fatal("hosted summary type changed")
	}
	public.Lanes.Counts["LIVE"] = 99
	public.Lanes.List[0].Open = 99
	public.Queue[0].Open = 99
	public.Fabric.Lanes[0].Open = 99
	after, err := json.Marshal(snapshot)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("hosted aggregate aliases nested local input")
	}
}

func TestHostedRouterKnownVerdictsExcludeUnknownKeys(t *testing.T) {
	want := map[string]int{"LIVE": 1, "WAKEABLE": 2, "HELD": 3, "AUTH_REQUIRED": 4, "WATCH_ONLY": 5, "UNSTAFFED": 6, "UNREACHABLE": 0, "PROVIDER_QUOTA": 7}
	snapshot := privateRouterFixture()
	snapshot.Lanes.Counts = map[string]int{}
	for key, n := range want {
		snapshot.Lanes.Counts[key] = n
	}
	snapshot.Lanes.Counts["PRIVATE-CANARY-unknown-key"] = 99
	s := New(Config{CloudBoard: true})
	got, ok := s.routerSnapshotResponse(snapshot).(hostedRouterSummary)
	if !ok || !reflect.DeepEqual(got.Lanes.Counts, want) || snapshot.Lanes.Counts["PRIVATE-CANARY-unknown-key"] != 99 {
		t.Fatal("recognized counts lost, unknown key exposed, or input mutated")
	}
}
