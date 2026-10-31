package routerstore

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAuthenticatedTaskClaimRefusesEmptyOrForeignThreadBeforeMutation(t *testing.T) {
	for _, next := range []bool{false, true} {
		for _, thread := range []string{"", "foreign"} {
			t.Run(thread+map[bool]string{false: "exact", true: "next"}[next], func(t *testing.T) {
				store, base := remoteHarness(t)
				backend := store.(*SQLiteStore)
				if err := backend.AddTask(Task{Agent: "lane", TaskID: "original", Subject: "no unusable claim"}); err != nil {
					t.Fatal(err)
				}
				client := NewRemoteStore(base.base, "t0k")
				client.agent, client.threadID, client.sessionDir = "lane", thread, ""
				var lease *TaskLease
				var err error
				if next {
					lease, err = client.ClaimNextTask("lane", "worker", "owner-thread", time.Minute)
				} else {
					lease, err = client.ClaimTask("lane", "original", "worker", "owner-thread", time.Minute)
				}
				if !errors.Is(err, ErrNotOwner) || lease != nil {
					t.Fatalf("incoherent authenticated claim returned lease: %v", err)
				}
				var status, token, ownerThread string
				var attempts int
				if err = backend.db.QueryRow(`SELECT status,lease_token,thread_id,attempts FROM tasks WHERE agent='lane' AND task_id='original'`).Scan(&status, &token, &ownerThread, &attempts); err != nil {
					t.Fatal(err)
				}
				if status != "pending" || token != "" || ownerThread != "" || attempts != 0 {
					t.Fatal("refused claim changed original task/attempt")
				}
				if owner, err := backend.TaskSession("lane", "original"); err != nil || owner != "" {
					t.Fatal("refused claim installed session owner", err)
				}
			})
		}
	}
}

func TestAuthenticatedTaskClaimBindsAtomicallyAndReleasesAfterRemint(t *testing.T) {
	var logs strings.Builder
	store, clients := ruleHarness(t, "enforce", &logs)
	backend := store.(*SQLiteStore)
	if err := backend.AddTask(Task{Agent: "lane", TaskID: "original", Subject: "same caller"}); err != nil {
		t.Fatal(err)
	}
	client := clients("lane", "owner-thread")
	register(t, backend, "owner-thread", "lane", client.host, "active", client.now())
	lease, err := client.ClaimTask("lane", "original", "worker", "owner-thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := backend.TaskSession("lane", "original")
	if err != nil || owner != client.session.ID {
		t.Fatal("returned claim not bound to authenticated caller", err)
	}
	foreign := clients("lane", "foreign")
	register(t, backend, "foreign", "lane", foreign.host, "active", foreign.now())
	if err = foreign.ReleaseTaskLease("lane", "original", lease.Token, "not mine"); !errors.Is(err, ErrNotOwner) {
		t.Fatal("foreign thread adopted original", err)
	}
	client.dropSession()
	if err = client.ReleaseTaskLease("lane", "original", lease.Token, "own reminted caller"); err != nil {
		t.Fatal("same-thread remint lost release", err)
	}
}

func TestAuthenticatedTaskOwnerBindFailureRollsBackClaimAndAttempt(t *testing.T) {
	store, base := remoteHarness(t)
	backend := store.(*SQLiteStore)
	if err := backend.AddTask(Task{Agent: "lane", TaskID: "original", Subject: "atomic bind"}); err != nil {
		t.Fatal(err)
	}
	wakes, err := backend.ListWakeEvents("lane")
	if err != nil {
		t.Fatal(err)
	}
	trigger := `CREATE TRIGGER fixture_task_bind_failure BEFORE INSERT ON lease_sessions WHEN NEW.kind='task' BEGIN SELECT RAISE(ABORT,'fixture task bind'); END`
	if !backend.dialect().hasPragmas {
		trigger = `CREATE FUNCTION fixture_task_bind_failure_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='task' THEN RAISE EXCEPTION 'fixture task bind'; END IF; RETURN NEW; END $$; CREATE TRIGGER fixture_task_bind_failure BEFORE INSERT ON lease_sessions FOR EACH ROW EXECUTE FUNCTION fixture_task_bind_failure_fn()`
	}
	if _, err := backend.db.Exec(trigger); err != nil {
		t.Fatal(err)
	}
	client := NewRemoteStore(base.base, "t0k")
	client.agent, client.threadID, client.sessionDir = "lane", "owner-thread", ""
	lease, err := client.ClaimTask("lane", "original", "worker", "owner-thread", time.Minute)
	if err == nil || lease != nil {
		t.Fatal("failed owner bind returned usable lease")
	}
	var status, token string
	var attempts int
	if err = backend.db.QueryRow(`SELECT status,lease_token,attempts FROM tasks WHERE agent='lane' AND task_id='original'`).Scan(&status, &token, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || token != "" || attempts != 0 {
		t.Fatal("failed bind committed lease or attempt")
	}
	if owner, err := backend.TaskSession("lane", "original"); err != nil || owner != "" {
		t.Fatal("failed bind installed task owner", err)
	}
	if after, err := backend.ListWakeEvents("lane"); err != nil || !reflect.DeepEqual(wakes, after) {
		t.Fatal("failed bind acknowledged or changed source wake", err)
	}
}
