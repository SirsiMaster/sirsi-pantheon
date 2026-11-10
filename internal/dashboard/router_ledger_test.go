package dashboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func ledgerFixture(agent string) (ledger.Snapshot, error) {
	if agent != "claude-m5-compasspoint" {
		return ledger.Snapshot{GeneratedAt: "2026-11-06T12:00:00Z"}, nil
	}
	return ledger.Snapshot{GeneratedAt: "2026-11-06T12:00:00Z", Agents: []ledger.Agent{{
		AgentID: agent,
		Items:   []ledger.Item{{ID: "i1", Title: "Review", From: "ra", Type: "request", Opened: "2026-11-06T11:00:00Z", AgeSeconds: 3600, Blocked: true, BlockedBy: "i0"}},
		Tasks: []routerstore.Task{
			{TaskID: "T1", Subject: "Build", Status: "blocked", Phase: "Phase A", BlockedBy: "waiting on T0", Updated: "2026-11-06T11:30:00Z", ResponsibleParty: "self"},
			{TaskID: "T2", Subject: "Ship", Status: "pending", Phase: "Phase B"},
		},
	}}}, nil
}

func getJSON(t *testing.T, s *Server, path string) (int, map[string]any, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	return rec.Code, m, rec
}

func TestRouterLedgerShapeUsesSafeTaskProjection(t *testing.T) {
	s := New(Config{LedgerAgentFn: ledgerFixture})
	code, m, rec := getJSON(t, s, "/api/router/v1/ledger?agent=claude-m5-compasspoint")
	if code != 200 {
		t.Fatalf("status %d: %s", code, rec.Body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("ledger must be no-store")
	}
	if m["generated_at"] != "2026-11-06T12:00:00Z" {
		t.Fatalf("generated_at = %v", m["generated_at"])
	}
	items := m["items"].([]any)
	tasks := m["tasks"].([]any)
	if len(items) != 1 || len(tasks) != 2 {
		t.Fatalf("items=%d tasks=%d", len(items), len(tasks))
	}
	it := items[0].(map[string]any)
	for _, k := range []string{"id", "title", "from", "type", "opened", "age_seconds", "blocked"} {
		if _, ok := it[k]; !ok {
			t.Errorf("item missing %q", k)
		}
	}
	t0 := tasks[0].(map[string]any)
	for _, k := range []string{"task_id", "subject", "status", "phase", "blocked_by", "updated", "responsible_party"} {
		if _, ok := t0[k]; !ok {
			t.Errorf("task missing %q", k)
		}
	}
	if t0["blocked_by"] != "" {
		t.Fatalf("blocker prose escaped: %v", t0)
	}
	// Owner decision 2026-11-08: phase is a display label, shown like the subject,
	// so the board can draw x/y per phase.
	if t0["phase"] != "Phase A" || tasks[1].(map[string]any)["phase"] != "Phase B" {
		t.Fatalf("phase labels lost: %v / %v", t0, tasks[1])
	}
}

func TestRouterLedgerPhaseIsCleanedLikeTheSubject(t *testing.T) {
	long := strings.Repeat("p", 200)
	s := New(Config{LedgerAgentFn: func(agent string) (ledger.Snapshot, error) {
		return ledger.Snapshot{Agents: []ledger.Agent{{AgentID: agent, Tasks: []routerstore.Task{
			{TaskID: "a", Status: "pending", Phase: "Gap\nburndown:\x1b intake"},
			{TaskID: "b", Status: "pending", Phase: long},
		}}}}, nil
	}})
	code, m, _ := getJSON(t, s, "/api/router/v1/ledger?agent=lane")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	tasks := m["tasks"].([]any)
	if got := tasks[0].(map[string]any)["phase"]; got != "Gap burndown:  intake" {
		t.Fatalf("control characters must become spaces, got %q", got)
	}
	if got := tasks[1].(map[string]any)["phase"].(string); len(got) != 128 {
		t.Fatalf("phase must be capped at 128 characters, got %d", len(got))
	}
}

func TestRouterLedgerExcludesPrivateTaskPayloadAcrossPages(t *testing.T) {
	const secret = "PRIVATE-LEASE-JOURNAL-CREDENTIAL-CANARY"
	s := New(Config{CloudBoard: true, LedgerAgentFn: func(agent string) (ledger.Snapshot, error) {
		a := ledger.Agent{AgentID: agent}
		for i := 0; i < 130; i++ {
			a.Tasks = append(a.Tasks, routerstore.Task{TaskID: fmt.Sprintf("task-%03d", i), Subject: "safe", Status: "pending", Phase: "safe phase", BlockedBy: secret})
		}
		a.Tasks[0].BlockedBy = "done-001"
		a.Tasks = append(a.Tasks, routerstore.Task{TaskID: "done-001", Subject: "safe done", Status: "done", Phase: "safe phase", BlockedBy: secret})
		a.Items = append(a.Items, ledger.Item{ID: "item-1", Title: "safe item", From: "ra", Type: "request", Blocked: true, BlockedBy: secret})
		return ledger.Snapshot{Agents: []ledger.Agent{a}}, nil
	}})
	code, m, rec := getJSON(t, s, "/api/router/v1/ledger?agent=lane")
	if code != http.StatusOK || strings.Contains(rec.Body.String(), secret) {
		t.Fatalf("private task payload escaped or request failed: %d %s", code, rec.Body)
	}
	items := m["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["blocked"] != true {
		t.Fatalf("item or its blocked flag lost: %v", items)
	}
	if got := len(m["tasks"].([]any)); got != 131 {
		t.Fatalf("got %d tasks; want all 130 open and 1 completed", got)
	}
	found := false
	for _, raw := range m["tasks"].([]any) {
		task := raw.(map[string]any)
		if task["task_id"] == "task-000" {
			found = true
			if task["blocked_by"] != "done-001" {
				t.Fatal("same-lane blocker across status groups was lost")
			}
		}
	}
	if !found {
		t.Fatal("expected task missing")
	}
}

func TestRouterLedgerRejectsOversizedLaneWithoutPartialRows(t *testing.T) {
	s := New(Config{LedgerAgentFn: func(agent string) (ledger.Snapshot, error) {
		a := ledger.Agent{AgentID: agent}
		for i := 0; i <= ledger.FabricAllTaskLimit; i++ {
			a.Tasks = append(a.Tasks, routerstore.Task{TaskID: fmt.Sprintf("task-%05d", i), Status: "pending"})
		}
		return ledger.Snapshot{Agents: []ledger.Agent{a}}, nil
	}})
	code, _, rec := getJSON(t, s, "/api/router/v1/ledger?agent=lane")
	if code != http.StatusServiceUnavailable || strings.Contains(rec.Body.String(), "task-00000") {
		t.Fatalf("oversized lane must fail closed, not return partial rows: %d", code)
	}
}

func TestRouterLedgerUnknownAgentIs404AndKnownEmptyLaneIs200(t *testing.T) {
	s := New(Config{LedgerAgentFn: ledgerFixture, RouterFn: func() (RouterSnapshot, error) {
		return RouterSnapshot{Lanes: RouterLanes{List: []RouterLaneVerdict{{Agent: "quiet-lane"}}}}, nil
	}})
	if code, _, _ := getJSON(t, s, "/api/router/v1/ledger?agent=nobody"); code != 404 {
		t.Fatalf("unknown agent = %d, want 404", code)
	}
	code, m, _ := getJSON(t, s, "/api/router/v1/ledger?agent=quiet-lane")
	if code != 200 {
		t.Fatalf("registered empty lane = %d, want 200", code)
	}
	if items, ok := m["items"].([]any); !ok || len(items) != 0 {
		t.Fatalf("empty ledger must carry empty arrays, got %v", m["items"])
	}
	if code, _, _ := getJSON(t, s, "/api/router/v1/ledger"); code != 400 {
		t.Fatalf("missing agent = %d, want 400", code)
	}
	if code, _, _ := getJSON(t, s, "/api/router/v1/ledger?agent=../etc"); code != 400 {
		t.Fatalf("bad agent = %d, want 400", code)
	}
}

func TestRouterLedgerEmptyDistinguishesUnavailableUnknownAndKnownLane(t *testing.T) {
	const privateError = "MODEL-PRIVATE-ROUTER-PRODUCER-ERROR"
	const generatedAt = "2026-11-09T06:00:00Z"
	laneSnapshot := func(agent string) RouterSnapshot {
		return RouterSnapshot{Lanes: RouterLanes{List: []RouterLaneVerdict{{Agent: agent}}}}
	}
	for _, tc := range []struct {
		name      string
		routerFn  func() (RouterSnapshot, error)
		want      int
		wantError string
	}{
		{"unconfigured inventory", nil, http.StatusServiceUnavailable, "lane registry unavailable"},
		{"failed inventory", func() (RouterSnapshot, error) {
			return RouterSnapshot{}, errors.New(privateError)
		}, http.StatusServiceUnavailable, "lane registry unavailable"},
		{"failed partial inventory includes lane", func() (RouterSnapshot, error) {
			return laneSnapshot("lane"), errors.New(privateError)
		}, http.StatusServiceUnavailable, "lane registry unavailable"},
		{"known empty lane", func() (RouterSnapshot, error) {
			return laneSnapshot("lane"), nil
		}, http.StatusOK, ""},
		{"unknown lane in available inventory", func() (RouterSnapshot, error) {
			return laneSnapshot("other"), nil
		}, http.StatusNotFound, "unknown agent"},
		{"unknown lane in successfully empty inventory", func() (RouterSnapshot, error) {
			return RouterSnapshot{}, nil
		}, http.StatusNotFound, "unknown agent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ledgerCalls, routerCalls := 0, 0
			cfg := Config{LedgerAgentFn: func(agent string) (ledger.Snapshot, error) {
				ledgerCalls++
				if agent != "lane" {
					t.Fatalf("wrong ledger agent %q", agent)
				}
				return ledger.Snapshot{GeneratedAt: generatedAt}, nil
			}}
			if tc.routerFn != nil {
				cfg.RouterFn = func() (RouterSnapshot, error) {
					routerCalls++
					return tc.routerFn()
				}
			}
			code, body, rec := getJSON(t, New(cfg), "/api/router/v1/ledger?agent=lane")
			if code != tc.want || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status/cache = %d/%q, want %d/no-store: %s", code, rec.Header().Get("Cache-Control"), tc.want, rec.Body)
			}
			wantRouterCalls := 0
			if tc.routerFn != nil {
				wantRouterCalls = 1
			}
			if ledgerCalls != 1 || routerCalls != wantRouterCalls {
				t.Fatalf("producer calls ledger=%d/router=%d", ledgerCalls, routerCalls)
			}
			if strings.Contains(rec.Body.String(), privateError) {
				t.Fatal("raw producer error leaked")
			}
			if tc.want != http.StatusOK {
				if body["error"] != tc.wantError {
					t.Fatalf("error = %v, want %q", body["error"], tc.wantError)
				}
				if _, ok := body["items"]; ok {
					t.Fatal("failure returned a healthy-looking ledger")
				}
				return
			}
			if body["agent"] != "lane" || body["generated_at"] != generatedAt {
				t.Fatalf("known-empty identity/freshness lost: %v", body)
			}
			for _, key := range []string{"items", "tasks"} {
				rows, ok := body[key].([]any)
				if !ok || len(rows) != 0 {
					t.Fatalf("known-empty %s must be [], got %v", key, body[key])
				}
			}
		})
	}
}

func TestRouterLedgerPresentAgentDoesNotRequireLaneInventory(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(fmt.Sprintf("router-configured-%t", configured), func(t *testing.T) {
			routerCalls := 0
			cfg := Config{LedgerAgentFn: func(agent string) (ledger.Snapshot, error) {
				return ledger.Snapshot{Agents: []ledger.Agent{{AgentID: agent}}}, nil
			}}
			if configured {
				cfg.RouterFn = func() (RouterSnapshot, error) {
					routerCalls++
					return RouterSnapshot{}, errors.New("MODEL-unused producer error")
				}
			}
			code, body, _ := getJSON(t, New(cfg), "/api/router/v1/ledger?agent=lane")
			if code != http.StatusOK || body["agent"] != "lane" || routerCalls != 0 {
				t.Fatalf("ledger's own lane evidence must not depend on fallback: %d %v calls=%d", code, body, routerCalls)
			}
		})
	}
}

func TestRouterLedgerProducerFailureAndMissingAre503(t *testing.T) {
	bad := New(Config{LedgerAgentFn: func(string) (ledger.Snapshot, error) { return ledger.Snapshot{}, errors.New("db locked") }})
	code, m, rec := getJSON(t, bad, "/api/router/v1/ledger?agent=a")
	if code != 503 || !strings.Contains(rec.Body.String(), "unavailable") {
		t.Fatalf("failing producer = %d %v", code, m)
	}
	if strings.Contains(rec.Body.String(), "db locked") {
		t.Fatal("raw producer error leaked")
	}
	if code, _, _ := getJSON(t, New(Config{}), "/api/router/v1/ledger?agent=a"); code != 503 {
		t.Fatalf("unwired = %d, want 503", code)
	}
}

func TestRouterLedgerAndJobsAreGETOnlyAndCloudBoardAllowed(t *testing.T) {
	s := New(Config{CloudBoard: true, LedgerAgentFn: ledgerFixture})
	if code, _, _ := getJSON(t, s, "/api/router/v1/ledger?agent=claude-m5-compasspoint"); code != 200 {
		t.Fatalf("cloud board ledger = %d, want 200", code)
	}
	for _, p := range []string{"/api/router/v1/ledger?agent=a", "/api/router/v1/jobs?agent=a"} {
		rec := httptest.NewRecorder()
		s.handler.ServeHTTP(rec, httptest.NewRequest("POST", p, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s = %d, want 405", p, rec.Code)
		}
	}
	if !cloudBoardAllows("/api/router/v1/ledger") || !cloudBoardAllows("/api/router/v1/jobs") {
		t.Fatal("cloud board allowlist must cover the ledger and jobs reads")
	}
}

func TestRouterLedgerUsesTheSurfaceCORSAllowlist(t *testing.T) {
	s := New(Config{LedgerAgentFn: ledgerFixture})
	for origin, want := range map[string]string{"https://sirsi.ai": "https://sirsi.ai", "https://untrusted.invalid": ""} {
		req := httptest.NewRequest("GET", "/api/router/v1/ledger?agent=claude-m5-compasspoint", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		s.handler.ServeHTTP(rec, req)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != want {
			t.Errorf("origin %s: ACAO %q, want %q", origin, got, want)
		}
	}
}

func TestRouterJobsHasNoSourceYet(t *testing.T) {
	code, m, _ := getJSON(t, New(Config{}), "/api/router/v1/jobs?agent=claude-m5-compasspoint")
	if code != 404 || m["error"] != "no source connected" {
		t.Fatalf("jobs = %d %v, want 404 no source connected", code, m)
	}
}

// Every asset ui.go serves from the embedded filesystem must actually be embedded:
// board-actions.js was served but missing from go:embed, so it answered 500.
func TestEveryServedUIAssetIsEmbedded(t *testing.T) {
	src, err := os.ReadFile("ui.go")
	if err != nil {
		t.Fatal(err)
	}
	files := regexp.MustCompile(`serveEmbedded\(w, "(ui/[^"]+)"`).FindAllStringSubmatch(string(src), -1)
	paths := regexp.MustCompile(`case "(/assets/[^"]+)":`).FindAllStringSubmatch(string(src), -1)
	s := New(Config{})
	served := 0
	for _, f := range files {
		if _, err := uiFS.ReadFile(f[1]); err != nil {
			t.Errorf("%s is served but not embedded: %v", f[1], err)
		}
		served++
	}
	for _, p := range paths {
		rec := httptest.NewRecorder()
		s.handler.ServeHTTP(rec, httptest.NewRequest("GET", p[1], nil))
		if rec.Code != 200 {
			t.Errorf("%s = %d, want 200", p[1], rec.Code)
		}
	}
	if served < 4 || len(paths) < 6 {
		t.Fatalf("scan found too little (%d files, %d paths): the test lost its anchor", served, len(paths))
	}
}
