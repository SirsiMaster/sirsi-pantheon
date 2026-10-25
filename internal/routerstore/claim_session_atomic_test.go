package routerstore

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Exercise the real authenticated HTTP methods, not a direct claim mock. The
// injected SQL failure must roll back the lease and its claim counter together.
func TestAuthenticatedClaimSessionBindingFailureRollsBackDC01(t *testing.T) {
	for _, exact := range []bool{false, true} {
		name := "ClaimNext"
		if exact {
			name = "ClaimItem"
		}
		t.Run(name, func(t *testing.T) {
			var logs strings.Builder
			store, clients := ruleHarness(t, "enforce", &logs)
			backend := store.(*SQLiteStore)
			client := clients("lane", "dc01-owner")
			register(t, backend, "dc01-owner", "lane", client.host, "active", client.now())
			id, err := backend.Send("sender", "lane", "atomic bind", "review", "inert")
			if err != nil {
				t.Fatal(err)
			}
			before, err := backend.Get(id)
			if err != nil {
				t.Fatal(err)
			}
			counters, err := backend.Counters()
			if err != nil {
				t.Fatal(err)
			}
			trigger := `CREATE TRIGGER dc01_reject_bind BEFORE INSERT ON lease_sessions WHEN NEW.kind='item' BEGIN SELECT RAISE(ABORT,'DC01 injected bind failure'); END`
			if !backend.dialect().hasPragmas {
				trigger = `CREATE FUNCTION dc01_reject_bind_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='item' THEN RAISE EXCEPTION 'DC01 injected bind failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER dc01_reject_bind BEFORE INSERT ON lease_sessions FOR EACH ROW EXECUTE FUNCTION dc01_reject_bind_fn()`
			}
			if _, err = backend.db.Exec(trigger); err != nil {
				t.Fatal(err)
			}
			var lease *Lease
			if exact {
				lease, err = client.ClaimItem("lane", id, time.Minute)
			} else {
				lease, err = client.ClaimNext("lane", time.Minute)
			}
			if err == nil || lease != nil {
				t.Fatalf("binding failure exposed usable claim: lease=%+v err=%v", lease, err)
			}
			after, err := backend.Get(id)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("failed binding changed canonical item: before=%+v after=%+v err=%v", before, after, err)
			}
			afterCounters, err := backend.Counters()
			if err != nil || !reflect.DeepEqual(counters, afterCounters) {
				t.Fatalf("failed binding changed counters: %v", err)
			}
			owner, err := backend.ItemSession(id)
			if err != nil || owner != "" {
				t.Fatalf("failed binding left owner: %q %v", owner, err)
			}
			if err = backend.Complete(id, "unacknowledged-token", "foreign result"); !errors.Is(err, ErrLeaseInvalid) {
				t.Fatalf("faulted claim remained executable: %v", err)
			}
			if backend.dialect().hasPragmas {
				_, err = backend.db.Exec(`DROP TRIGGER dc01_reject_bind`)
			} else {
				_, err = backend.db.Exec(`DROP TRIGGER dc01_reject_bind ON lease_sessions; DROP FUNCTION dc01_reject_bind_fn()`)
			}
			if err != nil {
				t.Fatal(err)
			}
			// Recovery is an explicit subsequent claim after definitive rollback.
			lease, err = client.ClaimItem("lane", id, time.Minute)
			if err != nil || lease == nil {
				t.Fatalf("claim after rollback: %+v %v", lease, err)
			}
			owner, err = backend.ItemSession(id)
			if err != nil || owner == "" {
				t.Fatalf("successful claim not bound: %q %v", owner, err)
			}
			boundItem, err := backend.Get(id)
			if err != nil {
				t.Fatal(err)
			}
			foreign := clients("lane", "dc01-foreign")
			register(t, backend, "dc01-foreign", "lane", foreign.host, "active", foreign.now())
			if err = foreign.Complete(id, lease.Token, "copied-token result"); !errors.Is(err, ErrNotOwner) {
				t.Fatalf("foreign authenticated thread used copied token: %v", err)
			}
			unchanged, err := backend.Get(id)
			if err != nil || !reflect.DeepEqual(boundItem, unchanged) {
				t.Fatalf("foreign refusal changed item: %v", err)
			}
			client.dropSession()
			if err = client.Complete(id, lease.Token, "actual owner"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLegacyLocalItemClaimCompatibilityDC01(t *testing.T) {
	s := newTestStore(t)
	id, err := s.Send("sender", "lane", "legacy", "review", "inert")
	if err != nil {
		t.Fatal(err)
	}
	lease, err := s.ClaimItem("lane", id, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := s.ItemSession(id)
	if err != nil || owner != "" {
		t.Fatalf("legacy claim authority changed: %q %v", owner, err)
	}
	if err = s.Complete(id, lease.Token, "legacy fenced completion"); err != nil {
		t.Fatal(err)
	}
}
