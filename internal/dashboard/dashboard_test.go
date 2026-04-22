package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
)

// testServer creates a dashboard Server with the given config and returns
// an httptest.Server plus a cleanup function.
func testServer(t *testing.T, cfg Config) *httptest.Server {
	t.Helper()
	s := New(cfg)
	return httptest.NewServer(s.srv.Handler)
}

func openTestNotifyStore(t *testing.T) *notify.Store {
	t.Helper()
	store, err := notify.Open(filepath.Join(t.TempDir(), "test-notify.db"))
	if err != nil {
		t.Fatalf("open notify store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func writeTestStele(t *testing.T, entries []stele.Entry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stele.jsonl")
	var lines []string
	for _, e := range entries {
		b, _ := json.Marshal(e)
		lines = append(lines, string(b))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write stele: %v", err)
	}
	return path
}

// ── HTML Page Tests ──────────────────────────────────────────────────

func TestOverview_HTTP200(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET / = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", ct)
	}
}

func TestOverview_WithStats(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{
		StatsFn: func() ([]byte, error) {
			return json.Marshal(map[string]interface{}{
				"ram_percent": 42.5,
				"ram_pressure": "low",
			})
		},
	})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET / = %d, want 200", resp.StatusCode)
	}
}

func TestScanPage_HTTP200(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{StelePath: filepath.Join(t.TempDir(), "nonexistent.jsonl")})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/scan")
	if err != nil {
		t.Fatalf("GET /scan: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /scan = %d, want 200", resp.StatusCode)
	}
}

func TestGhostsPage_HTTP200(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{StelePath: filepath.Join(t.TempDir(), "nonexistent.jsonl")})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ghosts")
	if err != nil {
		t.Fatalf("GET /ghosts: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /ghosts = %d, want 200", resp.StatusCode)
	}
}

func TestGuardPage_HTTP200(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/guard")
	if err != nil {
		t.Fatalf("GET /guard: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /guard = %d, want 200", resp.StatusCode)
	}
}

func TestNotificationsPage_HTTP200(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/notifications")
	if err != nil {
		t.Fatalf("GET /notifications: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /notifications = %d, want 200", resp.StatusCode)
	}
}

func TestHorusPage_HTTP200(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{StelePath: filepath.Join(t.TempDir(), "nonexistent.jsonl")})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/horus")
	if err != nil {
		t.Fatalf("GET /horus: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /horus = %d, want 200", resp.StatusCode)
	}
}

func TestVaultPage_HTTP200(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{StelePath: filepath.Join(t.TempDir(), "nonexistent.jsonl")})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/vault")
	if err != nil {
		t.Fatalf("GET /vault: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /vault = %d, want 200", resp.StatusCode)
	}
}

func TestNotFound(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/nonexistent-page")
	if err != nil {
		t.Fatalf("GET /nonexistent: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("GET /nonexistent = %d, want 404", resp.StatusCode)
	}
}

// ── API Tests ────────────────────────────────────────────────────────

func TestAPIStats_NilStatsFn(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/stats")
	if err != nil {
		t.Fatalf("GET /api/stats: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("nil StatsFn = %d, want 503", resp.StatusCode)
	}
}

func TestAPIStats_ReturnsJSON(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{
		StatsFn: func() ([]byte, error) {
			return json.Marshal(map[string]interface{}{
				"ram_percent": 55.2,
				"deity_count": 3,
			})
		},
	})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/stats")
	if err != nil {
		t.Fatalf("GET /api/stats: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /api/stats = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if body["ram_percent"] != 55.2 {
		t.Fatalf("ram_percent = %v, want 55.2", body["ram_percent"])
	}
}

func TestAPINotifications_NilStore(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/notifications")
	if err != nil {
		t.Fatalf("GET /api/notifications: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("nil NotifyDB = %d, want 503", resp.StatusCode)
	}
}

func TestAPINotifications_ReturnsArray(t *testing.T) {
	t.Parallel()
	store := openTestNotifyStore(t)
	_ = store.Record(notify.Notification{
		Source: "anubis", Action: "scan", Severity: "success", Summary: "test",
	})

	ts := testServer(t, Config{NotifyDB: store})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/notifications?limit=10")
	if err != nil {
		t.Fatalf("GET /api/notifications: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /api/notifications = %d, want 200", resp.StatusCode)
	}

	var results []notify.Notification
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Source != "anubis" {
		t.Fatalf("source = %q, want anubis", results[0].Source)
	}
}

func TestAPINotifications_FilterBySource(t *testing.T) {
	t.Parallel()
	store := openTestNotifyStore(t)
	_ = store.Record(notify.Notification{Source: "anubis", Action: "scan", Severity: "success", Summary: "a"})
	_ = store.Record(notify.Notification{Source: "isis", Action: "guard", Severity: "info", Summary: "b"})

	ts := testServer(t, Config{NotifyDB: store})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/notifications?source=isis")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	var results []notify.Notification
	_ = json.NewDecoder(resp.Body).Decode(&results)
	if len(results) != 1 || results[0].Source != "isis" {
		t.Fatalf("filter by source: got %d results, first source=%q", len(results), results[0].Source)
	}
}

func TestAPIStele_MissingFile(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{StelePath: filepath.Join(t.TempDir(), "nonexistent.jsonl")})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/stele")
	if err != nil {
		t.Fatalf("GET /api/stele: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("missing stele = %d, want 200 (empty array)", resp.StatusCode)
	}

	var results []stele.Entry
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("len = %d, want 0", len(results))
	}
}

func TestAPIStele_WithEntries(t *testing.T) {
	t.Parallel()
	entries := []stele.Entry{
		{Seq: 0, Type: stele.TypeAnubisScan, Deity: "anubis", Data: map[string]string{"items": "12"}},
		{Seq: 1, Type: stele.TypeKaHunt, Deity: "ka", Data: map[string]string{"ghosts": "3"}},
		{Seq: 2, Type: stele.TypeAnubisScan, Deity: "anubis", Data: map[string]string{"items": "7"}},
	}
	stelePath := writeTestStele(t, entries)

	ts := testServer(t, Config{StelePath: stelePath})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/stele?limit=2")
	if err != nil {
		t.Fatalf("GET /api/stele: %v", err)
	}
	defer resp.Body.Close()

	var results []stele.Entry
	_ = json.NewDecoder(resp.Body).Decode(&results)
	if len(results) != 2 {
		t.Fatalf("limit=2: got %d", len(results))
	}
	// Newest first
	if results[0].Seq != 2 {
		t.Fatalf("first entry seq = %d, want 2 (newest)", results[0].Seq)
	}
}

func TestAPIStele_TypeFilter(t *testing.T) {
	t.Parallel()
	entries := []stele.Entry{
		{Seq: 0, Type: stele.TypeAnubisScan, Deity: "anubis"},
		{Seq: 1, Type: stele.TypeKaHunt, Deity: "ka"},
		{Seq: 2, Type: stele.TypeAnubisScan, Deity: "anubis"},
	}
	stelePath := writeTestStele(t, entries)

	ts := testServer(t, Config{StelePath: stelePath})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/stele?type=" + stele.TypeKaHunt)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	var results []stele.Entry
	_ = json.NewDecoder(resp.Body).Decode(&results)
	if len(results) != 1 {
		t.Fatalf("type filter: got %d, want 1", len(results))
	}
	if results[0].Type != stele.TypeKaHunt {
		t.Fatalf("type = %q, want %q", results[0].Type, stele.TypeKaHunt)
	}
}

// ── Graceful Nil Degradation Tests ───────────────────────────────────

func TestPages_NilDataSources(t *testing.T) {
	t.Parallel()
	// All data sources nil — every page should still return 200.
	ts := testServer(t, Config{StelePath: filepath.Join(t.TempDir(), "none.jsonl")})
	defer ts.Close()

	pages := []string{"/", "/scan", "/ghosts", "/guard", "/notifications", "/horus", "/vault"}
	for _, p := range pages {
		resp, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("nil sources: GET %s = %d, want 200", p, resp.StatusCode)
		}
	}
}

// ── SSE Event Stream Tests ───────────────────────────────────────────

func TestAPIEvents_SSE(t *testing.T) {
	t.Parallel()
	eb := NewEventBuffer(64)
	eb.Push(Event{Type: "output", Data: `{"line":"hello"}`})
	eb.Push(Event{Type: "complete", Data: `{"status":"ok"}`})

	ts := testServer(t, Config{Events: eb})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatalf("GET /api/events: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
}

func TestEventBuffer_PushRead(t *testing.T) {
	t.Parallel()
	eb := NewEventBuffer(4)
	eb.Push(Event{Type: "a", Data: "1"})
	eb.Push(Event{Type: "b", Data: "2"})
	eb.Push(Event{Type: "c", Data: "3"})

	events := eb.Since(0)
	if len(events) != 3 {
		t.Fatalf("Since(0) = %d events, want 3", len(events))
	}
	if events[0].Type != "a" {
		t.Fatalf("first event = %q, want a", events[0].Type)
	}

	events = eb.Since(2)
	if len(events) != 1 {
		t.Fatalf("Since(2) = %d events, want 1", len(events))
	}
	if events[0].Type != "c" {
		t.Fatalf("Since(2)[0] = %q, want c", events[0].Type)
	}
}

func TestEventBuffer_Overflow(t *testing.T) {
	t.Parallel()
	eb := NewEventBuffer(2)
	eb.Push(Event{Type: "a", Data: "1"})
	eb.Push(Event{Type: "b", Data: "2"})
	eb.Push(Event{Type: "c", Data: "3"}) // evicts "a"

	events := eb.Since(0)
	// Oldest available is "b" (seq 1)
	if len(events) != 2 {
		t.Fatalf("overflow Since(0) = %d, want 2", len(events))
	}
	if events[0].Type != "b" {
		t.Fatalf("overflow first = %q, want b", events[0].Type)
	}
}

// ── Browser Opener Test ──────────────────────────────────────────────

func TestSetOpenBrowserFn(t *testing.T) {
	t.Parallel()
	old := getOpenBrowserFn()
	defer SetOpenBrowserFn(old)

	var called string
	SetOpenBrowserFn(func(url string) error {
		called = url
		return nil
	})

	fn := getOpenBrowserFn()
	_ = fn("http://test")
	if called != "http://test" {
		t.Fatalf("injected fn not called, got %q", called)
	}
}
