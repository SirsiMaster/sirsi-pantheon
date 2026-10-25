package routermcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/mcp"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// Exercise the registered tool through real MCP framing; use no surface thread
// lifecycle, service, canonical store or model. This isolates the translator and
// its per-instance map, not the separately owned DC01 authenticated binding fix.
func callClaimTool(t *testing.T, s *Server, name string, args map[string]interface{}) *mcp.ToolResult {
	t.Helper()
	params := map[string]interface{}{"name": name}
	if args != nil {
		params["arguments"] = args
	}
	request, err := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": params})
	if err != nil {
		t.Fatal(err)
	}
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" + string(request) + "\n"
	var output bytes.Buffer
	if err := s.protocol.RunWithIOContext(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&output)
	var initialized struct{ Error json.RawMessage }
	if err := decoder.Decode(&initialized); err != nil {
		t.Fatal(err)
	}
	if len(initialized.Error) > 0 && string(initialized.Error) != "null" {
		t.Fatal("initialize failed")
	}
	var response struct {
		Result mcp.ToolResult
		Error  json.RawMessage
	}
	if err := decoder.Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.Error) > 0 && string(response.Error) != "null" {
		t.Fatal("tool transport failed")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("unexpected trailing protocol response: %v", err)
	}
	return &response.Result
}
func claimText(result *mcp.ToolResult) string {
	var text strings.Builder
	for _, c := range result.Content {
		text.WriteString(c.Text)
	}
	return text.String()
}
func exactClaimFixture(t *testing.T) (*Server, *routerstore.SQLiteStore) {
	t.Helper()
	home := filepath.Join(t.TempDir(), "private-home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(key, "SIRSI_") {
			t.Setenv(key, "")
		}
	}
	t.Setenv("SIRSI_ALLOW_SCHEMA_MIGRATE", "1")
	t.Setenv(routercfg.StoreWakeEnv, "1")
	path := filepath.Join(home, "fixture-router.db")
	t.Setenv("SIRSI_ROUTER_DB", path)
	store, err := routerstore.OpenPath(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, row := range []routerstore.Item{
		{ID: "older-own", From: "fixture-sender", To: "owned-claim", Title: "Older eligible own work", Status: "open", Opened: "2026-01-01T00:00:00Z"},
		{ID: "exact-own", From: "fixture-sender", To: "owned-claim", Title: "Exact own canary", Status: "open", Opened: "2026-01-02T00:00:00Z"},
		{ID: "other-recipient", From: "fixture-sender", To: "other-claim", Title: "Other recipient work", Status: "open", Opened: "2026-01-01T00:00:00Z"},
	} {
		if err := store.Put(row); err != nil {
			t.Fatal(err)
		}
	}
	s := newServer("owned-claim", home)
	s.reg.set("owned-inert-thread", "owned-claim")
	return s, store
}
func fixtureItems(t *testing.T, store *routerstore.SQLiteStore) []routerstore.Item {
	t.Helper()
	items := []routerstore.Item{}
	for _, id := range []string{"older-own", "exact-own", "other-recipient"} {
		row, err := store.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, row)
	}
	return items
}
func localClaims(s *Server) map[string]claimRecord {
	s.reg.mu.RLock()
	defer s.reg.mu.RUnlock()
	result := map[string]claimRecord{}
	for id, record := range s.reg.claimed {
		result[id] = record
	}
	return result
}
func TestRouterClaimExactIDSelectsTargetAndKeepsInstanceOwnership(t *testing.T) {
	s, store := exactClaimFixture(t)
	before := fixtureItems(t, store)
	started := time.Now()
	result := callClaimTool(t, s, "router_claim", map[string]interface{}{"id": "exact-own"})
	if result.IsError || !strings.Contains(claimText(result), "Claimed exact-own ") {
		t.Fatalf("exact tool claim failed: %s", claimText(result))
	}
	after := fixtureItems(t, store)
	if after[1].Status != "claimed" || !reflect.DeepEqual(before[0], after[0]) || !reflect.DeepEqual(before[2], after[2]) {
		t.Fatal("exact selection changed older/foreign work or missed target")
	}
	claims := localClaims(s)
	record, ok := claims["exact-own"]
	if !ok || len(claims) != 1 || record.token == "" {
		t.Fatal("wrong per-instance claim map")
	}
	if valid, err := store.VerifyLease("exact-own", record.token); err != nil || !valid {
		t.Fatalf("local claim is not the durable lease: %v", err)
	}
	if record.expires.Before(started.Add(15*time.Minute-time.Second)) || record.expires.After(time.Now().Add(15*time.Minute+time.Second)) {
		t.Fatal("existing15-minute lease was changed")
	}
	if !strings.Contains(claimText(result), record.expires.Format(time.RFC3339)) {
		t.Fatal("reply does not use the returned lease expiry")
	}
	other := newServer("owned-claim", s.repo)
	other.reg.set("second-inert-thread", "owned-claim")
	refused := callClaimTool(t, other, "router_close", map[string]interface{}{"id": "exact-own", "ack": true, "result": "owned coordination fixture"})
	if !refused.IsError || !strings.Contains(claimText(refused), "not claimed by this session") {
		t.Fatal("fresh instance borrowed the first map")
	}
	if !reflect.DeepEqual(after, fixtureItems(t, store)) {
		t.Fatal("nonclaiming instance changed durable items")
	}
	if !s.reg.ownsClaim("exact-own") || other.reg.ownsClaim("exact-own") {
		t.Fatal("claim map ownership moved between instances")
	}
}
func TestRouterClaimOmittedIDPreservesClaimNext(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]interface{}
	}{{"omitted_arguments", nil}, {"empty_object", map[string]interface{}{}}} {
		t.Run(tc.name, func(t *testing.T) {
			s, store := exactClaimFixture(t)
			before := fixtureItems(t, store)
			result := callClaimTool(t, s, "router_claim", tc.args)
			if result.IsError || !strings.Contains(claimText(result), "Claimed older-own ") {
				t.Fatalf("legacy next claim changed: %s", claimText(result))
			}
			after := fixtureItems(t, store)
			if after[0].Status != "claimed" || !reflect.DeepEqual(before[1:], after[1:]) {
				t.Fatal("legacy next did not select only older own work")
			}
			if claims := localClaims(s); len(claims) != 1 || !s.reg.ownsClaim("older-own") {
				t.Fatal("legacy next lost local map")
			}
		})
	}
}
func TestRouterClaimExactRefusalNeverFallsBackOrMutatesOtherWork(t *testing.T) {
	for _, name := range []string{"wrong_recipient", "other_agent_cannot_take_own", "missing_id", "already_claimed"} {
		t.Run(name, func(t *testing.T) {
			s, store := exactClaimFixture(t)
			id := "exact-own"
			if name == "wrong_recipient" {
				id = "other-recipient"
			}
			if name == "other_agent_cannot_take_own" {
				s.reg.set("other-inert-thread", "other-claim")
			}
			if name == "missing_id" {
				id = "missing-exact"
			}
			var existing *routerstore.Lease
			if name == "already_claimed" {
				var err error
				existing, err = store.ClaimItem("owned-claim", id, claimTTL)
				if err != nil {
					t.Fatal(err)
				}
			}
			before := fixtureItems(t, store)
			counts, err := store.Counters()
			if err != nil {
				t.Fatal(err)
			}
			result := callClaimTool(t, s, "router_claim", map[string]interface{}{"id": id})
			if !result.IsError {
				t.Fatal("unavailable exact target was accepted")
			}
			if !reflect.DeepEqual(before, fixtureItems(t, store)) || len(localClaims(s)) != 0 {
				t.Fatal("exact refusal claimed unrelated work or changed local ownership")
			}
			now, err := store.Counters()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(counts, now) {
				t.Fatal("exact refusal changed claim accounting")
			}
			if existing != nil {
				if valid, err := store.VerifyLease(id, existing.Token); err != nil || !valid {
					t.Fatal("refusal consumed another existing lease")
				}
			}
		})
	}
}
func TestRouterClaimBadInputsAndUnregisteredRefuseBeforeRPC(t *testing.T) {
	var rpc atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { rpc.Add(1); http.Error(w, "unexpected fixture RPC", 500) }))
	t.Cleanup(server.Close)
	home := filepath.Join(t.TempDir(), "private-home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("SIRSI_ROUTER_URL", server.URL)
	t.Setenv("SIRSI_ROUTER_TOKEN", "inert-fixture-only")
	s := newServer("owned-claim", home)
	s.reg.set("inert-thread", "owned-claim")
	cases := []struct {
		name string
		args map[string]interface{}
	}{
		{"empty", map[string]interface{}{"id": ""}}, {"whitespace", map[string]interface{}{"id": "  "}},
		{"leading_space", map[string]interface{}{"id": " exact-own"}}, {"NUL", map[string]interface{}{"id": "bad\x00id"}},
		{"LF", map[string]interface{}{"id": "bad\nid"}}, {"control", map[string]interface{}{"id": "bad\tid"}},
		{"number", map[string]interface{}{"id": float64(1)}}, {"boolean", map[string]interface{}{"id": true}},
		{"null", map[string]interface{}{"id": nil}}, {"array", map[string]interface{}{"id": []any{"exact-own"}}},
		{"object", map[string]interface{}{"id": map[string]any{"value": "exact-own"}}},
		{"unknown_key", map[string]interface{}{"agent": "other-claim"}}, {"unknown_with_id", map[string]interface{}{"id": "exact-own", "ttl": 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := callClaimTool(t, s, "router_claim", tc.args)
			if !result.IsError || rpc.Load() != 0 || len(localClaims(s)) != 0 {
				t.Fatalf("bad input reached RPC/map: %s rpc=%d", tc.name, rpc.Load())
			}
		})
	}
	unregistered := newServer("owned-claim", home)
	result := callClaimTool(t, unregistered, "router_claim", map[string]interface{}{"id": "exact-own"})
	if !result.IsError || !strings.Contains(claimText(result), "no registered thread") || rpc.Load() != 0 {
		t.Fatal("unregistered exact claim reached RPC")
	}
}

func TestRouterClaimEmptyInboxPreservesNoWorkAndMap(t *testing.T) {
	s, store := exactClaimFixture(t)
	s.reg.set("empty-inert-thread", "empty-agent")
	before := fixtureItems(t, store)
	result := callClaimTool(t, s, "router_claim", map[string]interface{}{})
	if result.IsError || claimText(result) != "No open item to claim. Inbox is clear." {
		t.Fatalf("legacy empty inbox changed: %s", claimText(result))
	}
	if !reflect.DeepEqual(before, fixtureItems(t, store)) || len(localClaims(s)) != 0 {
		t.Fatal("empty inbox changed unrelated work or local claims")
	}
}
