package routerstore

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// A lane that passes its own --thread but whose process carries no matching session binding is refused by design
// (body --thread is never identity). The refusal must say which fact differed and what to do, over the real HTTP
// server and RemoteStore, and a correctly bound caller — even with a stale lease_sessions row left for the reused
// task key — must still claim.
func TestTaskClaimRefusalNamesTheActualReason(t *testing.T) {
	cases := []struct {
		name, sessAgent, sessThread, claimAgent, claimThread string
		want                                                 string
	}{
		{"threadless session, explicit own --thread", "lane", "", "lane", "thr-own", "bound to no thread"},
		{"session bound to another thread", "lane", "thr-session", "lane", "thr-other", `is not this session's bound thread "thr-session"`},
		{"claim names another agent", "lane", "thr-own", "other-lane", "thr-own", `this session is agent "lane"`},
	}
	for _, next := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				store, base := remoteHarness(t)
				backend := store.(*SQLiteStore)
				for _, agent := range []string{"lane", "other-lane"} {
					if err := backend.AddTask(Task{Agent: agent, TaskID: "fresh", Subject: "fresh pending task"}); err != nil {
						t.Fatal(err)
					}
				}
				client := NewRemoteStore(base.base, "t0k")
				client.agent, client.threadID, client.sessionDir = tc.sessAgent, tc.sessThread, ""
				var err error
				if next {
					_, err = client.ClaimNextTask(tc.claimAgent, "worker", tc.claimThread, time.Minute)
				} else {
					_, err = client.ClaimTask(tc.claimAgent, "fresh", "worker", tc.claimThread, time.Minute)
				}
				if !errors.Is(err, ErrNotOwner) {
					t.Fatalf("want ErrNotOwner, got %v", err)
				}
				if !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("refusal hides the reason: %q lacks %q", err.Error(), tc.want)
				}
			})
		}
	}
}

func TestTaskClaimSucceedsOverStaleLeaseSessionRowForReusedKey(t *testing.T) {
	var logs strings.Builder
	store, clients := ruleHarness(t, "enforce", &logs)
	backend := store.(*SQLiteStore)
	if err := backend.AddTask(Task{Agent: "lane", TaskID: "reused", Subject: "fresh row, stale owner row"}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.db.Exec(`INSERT INTO lease_sessions(kind,key,session) VALUES('task','lane/reused','ghost-session')`); err != nil {
		t.Fatal(err)
	}
	client := clients("lane", "owner-thread")
	register(t, backend, "owner-thread", "lane", client.host, "active", client.now())
	if _, err := client.ClaimTask("lane", "reused", "worker", "owner-thread", time.Minute); err != nil {
		t.Fatalf("bound caller refused over a stale lease_sessions row: %v", err)
	}
	if owner, err := backend.TaskSession("lane", "reused"); err != nil || owner != client.session.ID {
		t.Fatalf("stale owner row not replaced: %q %v", owner, err)
	}
}
