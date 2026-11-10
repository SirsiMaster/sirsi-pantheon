package routerstore

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestServiceKeepsAttendedSessionAgainstOldLeaseReap(t *testing.T) {
	s := openBackendStore(t, filepath.Join(t.TempDir(), "keep.db"))
	defer s.Close()
	h, err := Handler(s, ServerOptions{Token: "bootstrap-fixture", RuleOfRa: "off"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	client := NewRemoteStore(srv.URL, "bootstrap-fixture")
	client.host, client.agent, client.sessionDir = "m1", "ra", ""
	seen := s.clock().Add(-3 * time.Hour).Format(time.RFC3339Nano)
	later := s.clock().Format(time.RFC3339Nano)
	for _, c := range []struct {
		name, surface, lastError, want string
	}{
		{"attended lease reap is refused", "claude", "reaped: session s1 lease expired (no renewal in > 2h0m0s) at x", "active"},
		{"codex too", "codex", "reaped: session s1 lease expired (no renewal in > 2h0m0s) at x", "active"},
		{"worker still expires", "worker", "reaped: session s1 lease expired (no renewal in > 2h0m0s) at x", "reaped"},
		{"dead PID reap unchanged", "claude", "reaped: PID 42 dead per OS truth at x", "reaped"},
	} {
		t.Run(c.name, func(t *testing.T) {
			id := "thr-" + c.surface + "-" + c.want + "-" + c.lastError[:12]
			live, _ := json.Marshal(map[string]string{"thread_id": id, "surface": c.surface, "session_id": "s1", "status": "active"})
			if err := s.UpsertThreads([]ThreadRecord{{ThreadID: id, Agent: "ra", Host: "m1", Status: "active", LastSeenAt: seen, Payload: live}}); err != nil {
				t.Fatal(err)
			}
			dead, _ := json.Marshal(map[string]string{"thread_id": id, "surface": c.surface, "session_id": "s1", "status": "reaped", "last_error": c.lastError})
			applied, err := client.UpsertThreadCAS(ThreadRecord{ThreadID: id, Agent: "ra", Host: "m1", Status: "reaped", LastSeenAt: later, Payload: dead})
			if err != nil || !applied {
				t.Fatalf("old client must see its write applied: %v %v", applied, err)
			}
			rows, err := s.ListThreads()
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rows {
				if r.ThreadID == id {
					if r.Status != c.want {
						t.Fatalf("status %q, want %q", r.Status, c.want)
					}
					return
				}
			}
			t.Fatal("row missing")
		})
	}
}
