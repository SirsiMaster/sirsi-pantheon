package routerstore

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// ruleHarness serves backend with the Rule of Ra in the given mode and returns
// a client factory: each client is a fresh session for agent, with or without
// a thread id in its environment.
func ruleHarness(t *testing.T, mode string, logs *strings.Builder) (Store, func(agent, threadID string) *RemoteStore) {
	t.Helper()
	backend := newDst(t)
	now := time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC)
	h, err := Handler(backend, ServerOptions{Token: "t0k", MaxWait: time.Second, RuleOfRa: mode, RuleOfRaStale: 10 * time.Minute,
		now: func() time.Time { return now }, Log: func(f string, a ...any) { logs.WriteString(fmtSprintf(f, a...) + "\n") }})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return backend, func(agent, threadID string) *RemoteStore {
		t.Setenv("SIRSI_AGENT_ID", agent)
		t.Setenv("SIRSI_THREAD_ID", threadID)
		rs := NewRemoteStore(srv.URL, "t0k")
		rs.sessionDir = ""
		rs.now = func() time.Time { return now }
		return rs
	}
}

func register(t *testing.T, backend Store, threadID, agent, host, status string, seen time.Time) {
	t.Helper()
	if err := backend.UpsertThreads([]ThreadRecord{{ThreadID: threadID, Agent: agent, Status: status, LastSeenAt: seen.UTC().Format(time.RFC3339Nano), Payload: []byte(`{}`), Host: host}}); err != nil {
		t.Fatal(err)
	}
}

func TestRuleOfRaEnforceRefusesUnregisteredAndAllowsRegistered(t *testing.T) {
	var logs strings.Builder
	backend, client := ruleHarness(t, "enforce", &logs)
	host, _ := os.Hostname()
	now := time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC)

	// No thread id at all: refused, and reads still work.
	anon := client("lane-a", "")
	if _, _, err := anon.SendGuarded(SendReq{From: "lane-a", To: "b", Title: "x", Type: "proposal", Instructions: "x"}); !errors.Is(err, ErrUnregistered) {
		t.Fatalf("unregistered mutation must be refused with ErrUnregistered, got %v", err)
	}
	if _, err := anon.Inbox("lane-a"); err != nil {
		t.Fatalf("reads must stay open to an unregistered session: %v", err)
	}

	// Registered, active, fresh, same agent + host: allowed.
	register(t, backend, "thr-a1", "lane-a", host, "active", now.Add(-time.Minute))
	reg := client("lane-a", "thr-a1")
	id, _, err := reg.SendGuarded(SendReq{From: "lane-a", To: "b", Title: "ok", Type: "proposal", Instructions: "x"})
	if err != nil {
		t.Fatalf("registered session must be allowed: %v", err)
	}
	if it, err := backend.Get(id); err != nil || it.Title != "ok" {
		t.Fatalf("send did not land: %v", err)
	}

	// Two sessions under one agent id: the one without its own thread is refused.
	other := client("lane-a", "")
	if _, _, err := other.SendGuarded(SendReq{From: "lane-a", To: "b", Title: "y", Type: "proposal", Instructions: "x"}); !errors.Is(err, ErrUnregistered) {
		t.Fatalf("a second session of the same agent without its own thread must be refused, got %v", err)
	}

	// Stale heartbeat: refused.
	register(t, backend, "thr-a2", "lane-a", host, "active", now.Add(-time.Hour))
	if _, _, err := client("lane-a", "thr-a2").SendGuarded(SendReq{From: "lane-a", To: "b", Title: "z", Type: "proposal", Instructions: "x"}); !errors.Is(err, ErrUnregistered) {
		t.Fatalf("stale thread must be refused, got %v", err)
	}
	// Thread of another agent / another host: refused.
	register(t, backend, "thr-b1", "lane-b", host, "active", now)
	if _, _, err := client("lane-a", "thr-b1").SendGuarded(SendReq{From: "lane-a", To: "b", Title: "w", Type: "proposal", Instructions: "x"}); !errors.Is(err, ErrUnregistered) {
		t.Fatalf("another agent's thread must be refused, got %v", err)
	}
	register(t, backend, "thr-a3", "lane-a", "other-host", "active", now)
	if _, _, err := client("lane-a", "thr-a3").SendGuarded(SendReq{From: "lane-a", To: "b", Title: "v", Type: "proposal", Instructions: "x"}); !errors.Is(err, ErrUnregistered) {
		t.Fatalf("a thread registered on another host must be refused, got %v", err)
	}
	// Bootstrap verbs stay open: heartbeat/upsert from an unregistered session.
	if err := anon.UpsertThreads([]ThreadRecord{{ThreadID: "thr-new", Agent: "lane-a", Status: "active", LastSeenAt: now.Format(time.RFC3339Nano), Payload: []byte(`{}`), Host: host}}); err != nil {
		t.Fatalf("registration itself must not require registration: %v", err)
	}
}

func TestRuleOfRaLogModeAllowsAndRecords(t *testing.T) {
	var logs strings.Builder
	_, client := ruleHarness(t, "log", &logs)
	anon := client("lane-l", "")
	if _, _, err := anon.SendGuarded(SendReq{From: "lane-l", To: "b", Title: "x", Type: "proposal", Instructions: "x"}); err != nil {
		t.Fatalf("log mode must allow: %v", err)
	}
	if !strings.Contains(logs.String(), "WOULD REFUSE SendGuarded from lane-l@") || !strings.Contains(logs.String(), "carries no thread id") {
		t.Fatalf("log mode must record the would-be refusal:\n%s", logs.String())
	}
}

func TestSessionCarriesThreadAcrossTheWireAndSurvivesMigration(t *testing.T) {
	backend := newDst(t)
	s, err := backend.MintSessionForThread("h", "a", "rt", "thr-x")
	if err != nil {
		t.Fatal(err)
	}
	got, err := backend.GetSession(s.ID)
	if err != nil || got.ThreadID != "thr-x" {
		t.Fatalf("thread id must persist on the session: %+v %v", got, err)
	}
	if v, _ := backend.db.Query(`PRAGMA user_version`); v != nil {
		var ver int
		if v.Next() {
			_ = v.Scan(&ver)
		}
		_ = v.Close()
		if ver != 19 {
			t.Fatalf("schema version %d, want 19", ver)
		}
	}
}

func fmtSprintf(f string, a ...any) string { return strings.TrimSpace(fmt.Sprintf(f, a...)) }
