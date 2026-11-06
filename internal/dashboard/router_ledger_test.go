package dashboard

import (
	"encoding/json"
	"errors"
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

func TestRouterLedgerShapePreservesPhaseAndBlockedBy(t *testing.T) {
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
	if t0["phase"] != "Phase A" || t0["blocked_by"] != "waiting on T0" {
		t.Fatalf("phase/blocked_by not preserved: %v", t0)
	}
	if tasks[1].(map[string]any)["phase"] != "Phase B" {
		t.Fatal("task order or phase lost")
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
