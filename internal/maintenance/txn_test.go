package maintenance

import (
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func thread(id, agent, host string, status router.ThreadStatus) *router.Thread {
	return &router.Thread{ThreadID: id, AgentID: agent, Host: host, Status: status}
}

func TestEnumerateRegistered(t *testing.T) {
	reg := &router.ThreadRegistry{Threads: map[string]*router.Thread{
		"t-active":     thread("t-active", "ra", "m1", router.ThreadStatusActive),
		"t-idle":       thread("t-idle", "ra", "m1", router.ThreadStatusIdle),
		"t-blocked":    thread("t-blocked", "ra", "m1", router.ThreadStatusBlocked),
		"t-stale":      thread("t-stale", "ra", "m1", router.ThreadStatusStale),
		"t-closed":     thread("t-closed", "ra", "m1", router.ThreadStatusClosed),
		"t-reaped":     thread("t-reaped", "ra", "m1", router.ThreadStatusReaped),
		"t-suspended":  thread("t-suspended", "ra", "m1", router.ThreadStatusSuspended),
		"t-otherhost":  thread("t-otherhost", "ra", "m5", router.ThreadStatusActive),
		"t-otheragent": thread("t-otheragent", "codex", "m1", router.ThreadStatusActive),
	}}

	tests := []struct {
		name    string
		reg     *router.ThreadRegistry
		scope   Scope
		want    []string // expected ThreadIDs, in order
		wantErr bool
	}{
		{
			name:    "nil registry fails closed",
			reg:     nil,
			scope:   Scope{Host: "m1"},
			wantErr: true,
		},
		{
			name:    "missing host fails closed",
			reg:     reg,
			scope:   Scope{},
			wantErr: true,
		},
		{
			name:  "live participants on host, excluding terminal and suspended",
			reg:   reg,
			scope: Scope{Host: "m1"},
			want:  []string{"t-active", "t-blocked", "t-idle", "t-otheragent", "t-stale"},
		},
		{
			name:  "agent prefix narrows scope",
			reg:   reg,
			scope: Scope{Host: "m1", AgentPrefix: "ra"},
			want:  []string{"t-active", "t-blocked", "t-idle", "t-stale"},
		},
		{
			name:  "different host sees only its own threads",
			reg:   reg,
			scope: Scope{Host: "m5"},
			want:  []string{"t-otherhost"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EnumerateRegistered(tc.reg, tc.scope)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d participants, want %d: %+v", len(got), len(tc.want), got)
			}
			for i, id := range tc.want {
				if got[i].ThreadID != id {
					t.Errorf("index %d: got %q, want %q (order must be deterministic)", i, got[i].ThreadID, id)
				}
			}
		})
	}
}

func TestMint(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	participants := []Participant{
		{ThreadID: "t-b", AgentID: "ra"},
		{ThreadID: "t-a", AgentID: "ra"},
	}

	t.Run("requires host", func(t *testing.T) {
		if _, err := Mint(Scope{}, participants, time.Minute, now); err == nil {
			t.Fatal("expected error for missing host")
		}
	})

	t.Run("requires positive ttl", func(t *testing.T) {
		if _, err := Mint(Scope{Host: "m1"}, participants, 0, now); err == nil {
			t.Fatal("expected error for non-positive ttl")
		}
	})

	t.Run("mints a provisional txn with sorted participants and expiry", func(t *testing.T) {
		txn, err := Mint(Scope{Host: "m1"}, participants, 5*time.Minute, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !txn.Provisional {
			t.Error("P1 mint must always be Provisional")
		}
		if txn.ID == "" || txn.Digest == "" {
			t.Error("expected a non-empty id and digest")
		}
		if !txn.ExpiresAt.Equal(now.Add(5 * time.Minute)) {
			t.Errorf("ExpiresAt = %v, want %v", txn.ExpiresAt, now.Add(5*time.Minute))
		}
		if txn.Participants[0].ThreadID != "t-a" || txn.Participants[1].ThreadID != "t-b" {
			t.Errorf("participants not sorted: %+v", txn.Participants)
		}
	})

	t.Run("digest is stable regardless of input participant order", func(t *testing.T) {
		reversed := []Participant{participants[1], participants[0]}
		a, err := Mint(Scope{Host: "m1"}, participants, time.Minute, now)
		if err != nil {
			t.Fatal(err)
		}
		// Force identical ids to isolate ordering from the random id.
		b, err := Mint(Scope{Host: "m1"}, reversed, time.Minute, now)
		if err != nil {
			t.Fatal(err)
		}
		b.ID = a.ID
		bd, err := digestOf(b.ID, b.Scope, b.Participants)
		if err != nil {
			t.Fatal(err)
		}
		ad, err := digestOf(a.ID, a.Scope, a.Participants)
		if err != nil {
			t.Fatal(err)
		}
		if ad != bd {
			t.Errorf("digest depends on input order: %s vs %s", ad, bd)
		}
	})

	t.Run("two mints of the same input produce different ids and digests", func(t *testing.T) {
		a, err := Mint(Scope{Host: "m1"}, participants, time.Minute, now)
		if err != nil {
			t.Fatal(err)
		}
		b, err := Mint(Scope{Host: "m1"}, participants, time.Minute, now)
		if err != nil {
			t.Fatal(err)
		}
		if a.ID == b.ID {
			t.Error("expected unique txn ids across mints")
		}
		if a.Digest == b.Digest {
			t.Error("expected the id-bound digest to differ when ids differ")
		}
	})
}
