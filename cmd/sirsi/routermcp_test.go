package main

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
	"sync"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routermcp"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestRouterMCPCommandReleasedManifest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SIRSI_AGENT_ID", "")
	cmd := newRouterMCPCmd(routermcp.Run)
	cmd.SetArgs([]string{})
	cmd.SetIn(io.NopCloser(strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n" +
			`{"jsonrpc":"2.0","id":3,"method":"resources/list"}` + "\n")))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&out)
	got := map[string]interface{}{}
	for i := 0; i < 3; i++ {
		var response struct {
			Result map[string]interface{} `json:"result"`
			Error  interface{}            `json:"error"`
		}
		if err := decoder.Decode(&response); err != nil {
			t.Fatal(err)
		}
		if response.Error != nil {
			t.Fatal(response.Error)
		}
		for k, v := range response.Result {
			got[k] = v
		}
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout not JSON-only: %v", err)
	}
	b, err := os.ReadFile("../../internal/routermcp/testdata/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]interface{}
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical CLI manifest differs from released bare server: %v", got)
	}
}

// Exercise the actual CLI-installed session marker hook, without replacing it
// or temporarily publishing a different caller into global identity state.
func TestRouterMCPExplicitAudienceOverridesActualSessionMarker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, key := range []string{"SIRSI_AGENT_ID", "SIRSI_THREAD_ID", routerstore.EnvCloudHost, routerstore.EnvUseMachineIDHost, "SIRSI_ROUTER_DB"} {
		t.Setenv(key, "")
	}
	t.Setenv("SIRSI_ALLOW_SCHEMA_MIGRATE", "1")
	t.Setenv(router.SessionIDEnv, "owned-provider-marker-fixture")
	if err := router.WriteSessionAgentMarker(router.CurrentSessionID(), "parent-lane"); err != nil {
		t.Fatal(err)
	}
	if err := router.WriteSessionThreadMarker(router.CurrentSessionID(), "parent-thread"); err != nil {
		t.Fatal(err)
	}
	if routerstore.IdentityHook == nil {
		t.Fatal("actual CLI identity hook is absent")
	}
	if agent, thread := routerstore.IdentityHook(); agent != "parent-lane" || thread != "parent-thread" {
		t.Fatal("fixture did not activate actual foreign parent markers")
	}
	backend, err := routerstore.OpenPath(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.UpsertThreadCAS(routerstore.ThreadRecord{ThreadID: "owned-mcp-thread", Agent: "owned-mcp", Host: host, Status: "active", LastSeenAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	h, err := routerstore.Handler(backend, routerstore.ServerOptions{Token: "owned-fixture-token", RuleOfRa: "enforce"})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var sid string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/call/Inbox" {
			mu.Lock()
			sid = r.Header.Get("X-Sirsi-Session")
			mu.Unlock()
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SIRSI_ROUTER_URL", srv.URL)
	t.Setenv("SIRSI_ROUTER_TOKEN", "owned-fixture-token")
	client, err := routerstore.ResolveForCaller("owned-mcp", "owned-mcp-thread")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	if _, err := client.Inbox("owned-mcp"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	actual := sid
	mu.Unlock()
	session, err := backend.GetSession(actual)
	if err != nil {
		t.Fatal(err)
	}
	if session.Agent != "owned-mcp" || session.ThreadID != "owned-mcp-thread" {
		t.Fatal("actual CLI marker replaced explicit RPC audience")
	}
	if agent, thread := routerstore.IdentityHook(); agent != "parent-lane" || thread != "parent-thread" {
		t.Fatal("explicit caller changed parent markers")
	}
}

func TestRouterMCPCommandRefusesPositionalsBeforeRun(t *testing.T) {
	calls := 0
	cmd := newRouterMCPCmd(func(context.Context, io.ReadCloser, io.Writer) error { calls++; return nil })
	cmd.SetArgs([]string{"ignored"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err == nil || calls != 0 {
		t.Fatalf("args accepted/run called: %v %d", err, calls)
	}
}
