package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// runnerCfg returns a Config with a live runner backed by a harmless binary so
// the "started" path executes without side effects (/usr/bin/true ignores args).
func runnerCfg() Config {
	return Config{Events: NewEventBuffer(16), SirsiBin: "/usr/bin/true"}
}

func postJSON(t *testing.T, url string, body ActionRequest) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func TestApiActions_ListsRegistry(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/actions")
	if err != nil {
		t.Fatalf("GET /api/actions: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var specs []ActionSpec
	if err := json.NewDecoder(resp.Body).Decode(&specs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byKey := map[string]ActionSpec{}
	for _, sp := range specs {
		byKey[sp.Key] = sp
	}
	if sp, ok := byKey["ra/kill"]; !ok || !sp.Destructive {
		t.Error("ra/kill must be present and Destructive")
	}
	if sp, ok := byKey["scan"]; !ok || sp.Destructive {
		t.Error("scan must be present and non-destructive")
	}
	// E1: every gap-list action reachable.
	for _, k := range []string{"audit", "maat", "risk", "network/fix", "thoth/sync", "seshat/ingest", "net/align", "ra/deploy", "ra/collect"} {
		if _, ok := byKey[k]; !ok {
			t.Errorf("action %q missing from registry", k)
		}
	}
}

func TestApiRun_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	ts := testServer(t, runnerCfg())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/run")
	if err != nil {
		t.Fatalf("GET /api/run: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestApiRun_UnknownAction(t *testing.T) {
	t.Parallel()
	ts := testServer(t, runnerCfg())
	defer ts.Close()
	resp := postJSON(t, ts.URL+"/api/run", ActionRequest{Action: "does-not-exist"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestApiRun_LegacyDestructiveBlocked(t *testing.T) {
	t.Parallel()
	ts := testServer(t, runnerCfg())
	defer ts.Close()
	// Legacy form-encoded ?cmd= can never trigger a destructive action.
	resp, err := http.Post(ts.URL+"/api/run?cmd=ra/kill", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (destructive via legacy form must be blocked)", resp.StatusCode)
	}
}

func TestApiRun_DestructivePrepareThenCommit(t *testing.T) {
	t.Parallel()
	ts := testServer(t, runnerCfg())
	defer ts.Close()

	// Phase 1 — no token => prepare (dry-run), returns a token. Nothing executes.
	resp := postJSON(t, ts.URL+"/api/run", ActionRequest{Action: "ra/kill", Target: "scope-x"})
	if resp.StatusCode != 200 {
		t.Fatalf("prepare status = %d, want 200", resp.StatusCode)
	}
	var prep PreparedAction
	if err := json.NewDecoder(resp.Body).Decode(&prep); err != nil {
		t.Fatalf("decode prepare: %v", err)
	}
	resp.Body.Close()
	if !prep.DryRun || prep.ConfirmToken == "" {
		t.Fatal("prepare must be dry-run and return a confirm token")
	}

	// A bogus token must be rejected (403), not executed.
	bad := postJSON(t, ts.URL+"/api/run", ActionRequest{Action: "ra/kill", Target: "scope-x", ConfirmToken: "bogus"})
	if bad.StatusCode != http.StatusForbidden {
		t.Fatalf("bad-token status = %d, want 403", bad.StatusCode)
	}
	bad.Body.Close()

	// Phase 2 — real token commits (executes the harmless binary).
	ok := postJSON(t, ts.URL+"/api/run", ActionRequest{
		Action: "ra/kill", Target: "scope-x", ConfirmToken: prep.ConfirmToken, ActionHash: prep.ActionHash,
	})
	defer ok.Body.Close()
	if ok.StatusCode != 200 {
		t.Fatalf("commit status = %d, want 200", ok.StatusCode)
	}
	var res ActionResult
	if err := json.NewDecoder(ok.Body).Decode(&res); err != nil {
		t.Fatalf("decode commit: %v", err)
	}
	if res.Status != "started" {
		t.Fatalf("commit Status = %q, want started", res.Status)
	}
}

func TestApiVaultPrune_RequiresConfirm(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{})
	defer ts.Close()

	// No token => prepare (dry-run), returns a token. Nothing is pruned, and
	// the vault is never even opened.
	resp, err := http.Post(ts.URL+"/api/vault/prune?older_than=720h", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	var prep PreparedAction
	if decErr := json.NewDecoder(resp.Body).Decode(&prep); decErr != nil {
		t.Fatalf("decode prepare: %v", decErr)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || !prep.DryRun || prep.ConfirmToken == "" {
		t.Fatalf("prune without token must return a dry-run PreparedAction with a token (status=%d)", resp.StatusCode)
	}

	// Bogus token must be rejected, not pruned.
	bad, err := http.Post(ts.URL+"/api/vault/prune?older_than=720h&confirm_token=bogus", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer bad.Body.Close()
	if bad.StatusCode != http.StatusForbidden {
		t.Fatalf("bad-token prune status = %d, want 403", bad.StatusCode)
	}
}

func TestApiSlay_RealKillRequiresConfirm(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{})
	defer ts.Close()

	// dry_run=false with no token => prepare, never kills.
	resp, err := http.Post(ts.URL+"/api/slay?target=lsp&dry_run=false", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	var prep PreparedAction
	if decErr := json.NewDecoder(resp.Body).Decode(&prep); decErr != nil {
		t.Fatalf("decode prepare: %v", decErr)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || !prep.DryRun || prep.ConfirmToken == "" {
		t.Fatalf("real kill without token must return a PreparedAction (status=%d)", resp.StatusCode)
	}

	bad, err := http.Post(ts.URL+"/api/slay?target=lsp&dry_run=false&confirm_token=bogus", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer bad.Body.Close()
	if bad.StatusCode != http.StatusForbidden {
		t.Fatalf("bad-token kill status = %d, want 403", bad.StatusCode)
	}
}

// codex P1 regression: a confirm token prepared for one positional arg set must
// NOT validate a commit with different args (args are executable input).
func TestApiRun_ConfirmTokenBindsArgs(t *testing.T) {
	t.Parallel()
	ts := testServer(t, runnerCfg())
	defer ts.Close()

	// Prepare ra/kill for scope "alpha".
	resp := postJSON(t, ts.URL+"/api/run", ActionRequest{Action: "ra/kill", Target: "node", Args: []string{"alpha"}})
	var prep PreparedAction
	if err := json.NewDecoder(resp.Body).Decode(&prep); err != nil {
		t.Fatalf("decode prepare: %v", err)
	}
	resp.Body.Close()
	if prep.ConfirmToken == "" {
		t.Fatal("expected a confirm token")
	}

	// Commit with the SAME token but DIFFERENT args ("beta") — must be rejected.
	swapped := postJSON(t, ts.URL+"/api/run", ActionRequest{
		Action: "ra/kill", Target: "node", Args: []string{"beta"},
		ConfirmToken: prep.ConfirmToken, ActionHash: prep.ActionHash,
	})
	defer swapped.Body.Close()
	if swapped.StatusCode != http.StatusForbidden {
		t.Fatalf("commit with swapped args status = %d, want 403 (token must bind args)", swapped.StatusCode)
	}

	// Commit with the SAME args must still succeed (binding doesn't break legit use).
	resp2 := postJSON(t, ts.URL+"/api/run", ActionRequest{Action: "ra/kill", Target: "node", Args: []string{"alpha"}})
	var prep2 PreparedAction
	_ = json.NewDecoder(resp2.Body).Decode(&prep2)
	resp2.Body.Close()
	ok := postJSON(t, ts.URL+"/api/run", ActionRequest{
		Action: "ra/kill", Target: "node", Args: []string{"alpha"},
		ConfirmToken: prep2.ConfirmToken, ActionHash: prep2.ActionHash,
	})
	defer ok.Body.Close()
	if ok.StatusCode != 200 {
		t.Fatalf("commit with matching args status = %d, want 200", ok.StatusCode)
	}
}

func TestApiStats_Typed(t *testing.T) {
	t.Parallel()
	want := StatsResponse{
		TotalRAM:    32,
		UsedRAM:     16,
		RAMPressure: "low",
		GitBranch:   "main",
		DeityCount:  3,
		Timestamp:   time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
	}
	cfg := Config{StatsFn: func() ([]byte, error) { return json.Marshal(want) }}
	ts := testServer(t, cfg)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/stats")
	if err != nil {
		t.Fatalf("GET /api/stats: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode typed stats: %v", err)
	}
	if got.TotalRAM != want.TotalRAM || got.GitBranch != want.GitBranch || got.DeityCount != want.DeityCount {
		t.Errorf("typed stats mismatch: got %+v", got)
	}
}
