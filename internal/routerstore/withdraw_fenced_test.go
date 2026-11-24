package routerstore

import (
	"errors"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Exercise the actual least-privileged service role, not only the scratch
// cluster's schema-owning test user.
func TestWithdrawTaskLeasePostgresServiceRole(t *testing.T) {
	if pgTestDSN() == "" {
		t.Skip("PostgreSQL service-role check")
	}
	s := openTestStore(t)
	for _, id := range []string{"old", "next"} {
		if err := s.AddTask(Task{Agent: "lane", TaskID: id, Subject: id}); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := s.ClaimTask("lane", "old", "worker", "thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	dsn, err := url.Parse(pgTestDSN())
	if err != nil {
		t.Fatal(err)
	}
	dsn.User = url.User("router_service")
	service, err := OpenPostgres(dsn.String())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err = service.WithdrawTaskLease("lane", "old", lease.Token, "lane/next", "lane@host", "duplicate"); err != nil {
		t.Fatalf("service-role fenced withdrawal: %v", err)
	}
	got, err := s.GetTask("lane", "old")
	if err != nil || got.Status != "withdrawn" {
		t.Fatalf("row=%+v err=%v", got, err)
	}
}

func TestWithdrawTaskLeaseFenceAndSuccessor(t *testing.T) {
	s := openTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	s.now = func() time.Time { return now }
	for _, id := range []string{"old", "next", "canceled"} {
		if err := s.AddTask(Task{Agent: "lane", TaskID: id, Subject: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CancelTask("lane", "canceled", "board:a@example.com", "test"); err != nil {
		t.Fatal(err)
	}
	lease, err := s.ClaimTask("lane", "old", "worker", "thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token, target string
		want                error
	}{
		{"missing fence", "", "lane/next", ErrLeaseInvalid},
		{"wrong fence", "wrong", "lane/next", ErrLeaseInvalid},
		{"canceled successor", lease.Token, "lane/canceled", ErrInvalidInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := s.WithdrawTaskLease("lane", "old", tc.token, tc.target, "lane@host", "duplicate"); !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
	if err := s.WithdrawTask("lane", "old", "lane/next", "lane@host", "dup"); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("legacy erased live lease: %v", err)
	}
	now = now.Add(2 * time.Minute)
	if err := s.WithdrawTaskLease("lane", "old", lease.Token, "lane/next", "lane@host", "dup"); !errors.Is(err, ErrLeaseInvalid) {
		t.Fatalf("expired fence: %v", err)
	}
	now = now.Add(-2 * time.Minute)
	if err := s.WithdrawTaskLease("lane", "old", lease.Token, "lane/next", "lane@host", "duplicate"); err != nil {
		t.Fatal(err)
	}
	row, err := s.GetTask("lane", "old")
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "withdrawn" || row.ResultRef != "superseded:lane/next" || len(row.Timeline) == 0 || row.Timeline[len(row.Timeline)-1].Owner != "lane@host" {
		t.Fatalf("wrong result: %+v", row)
	}
	if _, ok, err := s.GetState(receiptKey("lane", "old")); err != nil || ok {
		t.Fatalf("completion receipt created: ok=%v err=%v", ok, err)
	}
}

// A successor update beginning after both rows have been locked must not
// complete before the fenced withdrawal's decision. The peer may cancel later.
func TestWithdrawLocksSuccessorBeforeDecision(t *testing.T) {
	s := openTestStore(t)
	peer := taskEvidencePeer(t, s)
	for _, id := range []string{"old", "next"} {
		if err := s.AddTask(Task{Agent: "lane", TaskID: id, Subject: id}); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan error, 1)
	started := make(chan struct{})
	blocked := false
	s.now = func() time.Time {
		go func() {
			close(started)
			_, err := peer.db.Exec(`UPDATE tasks SET status='canceled' WHERE agent=? AND task_id=?`, "lane", "next")
			done <- err
		}()
		<-started
		select {
		case err := <-done:
			t.Errorf("successor changed before decision: %v", err)
		case <-time.After(100 * time.Millisecond):
			blocked = true
		}
		return time.Now().UTC()
	}
	if err := s.WithdrawTask("lane", "old", "lane/next", "lane@host", "duplicate"); err != nil {
		t.Fatal(err)
	}
	if !blocked {
		t.Fatal("successor update did not wait on the decision")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("peer did not resume")
	}
}

// Expiry is evaluated after waiting for the actual writer/row lock.
func TestWithdrawTaskLeaseChecksExpiryAfterLockWait(t *testing.T) {
	s := openTestStore(t)
	peer := taskEvidencePeer(t, s)
	epoch := time.Now().UTC().Truncate(time.Second)
	var seconds atomic.Int64
	seconds.Store(epoch.Unix())
	s.now = func() time.Time { return time.Unix(seconds.Load(), 0).UTC() }
	for _, id := range []string{"old", "next"} {
		if err := s.AddTask(Task{Agent: "lane", TaskID: id, Subject: id}); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := s.ClaimTask("lane", "old", "worker", "thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := peer.beginImmediate()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`UPDATE tasks SET phase=phase WHERE agent=? AND task_id=?`, "lane", "old"); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		done <- s.WithdrawTaskLease("lane", "old", lease.Token, "lane/next", "lane@host", "dup")
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("withdraw did not wait for source lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	seconds.Store(epoch.Add(2 * time.Minute).Unix())
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrLeaseInvalid) {
			t.Fatalf("expired fence accepted: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("withdraw did not finish")
	}
}

func TestWithdrawTaskLeaseOwnLaneOverWire(t *testing.T) {
	f := newScopedFixture(t, []string{"cloud-a", "cloud-b"})
	for _, tk := range []Task{{Agent: "cloud-a", TaskID: "old", Subject: "old"}, {Agent: "cloud-a", TaskID: "next", Subject: "next"}, {Agent: "cloud-b", TaskID: "theirs", Subject: "theirs"}} {
		if err := f.backend.AddTask(tk); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := f.backend.ClaimTask("cloud-a", "old", "worker", "thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	a := f.client("cloud-a")
	wantLaneScope(t, "another lane", a.WithdrawTaskLease("cloud-b", "theirs", lease.Token, "cloud-a/next", "forged", "dup"))
	if err = a.WithdrawTaskLease("cloud-a", "old", "wrong", "cloud-a/next", "forged", "dup"); !errors.Is(err, ErrLeaseInvalid) {
		t.Fatalf("wrong fence: %v", err)
	}
	if err = a.WithdrawTaskLease("cloud-a", "old", lease.Token, "cloud-a/next", "board:forged@example.com", "dup"); err != nil {
		t.Fatal(err)
	}
	row, err := f.backend.GetTask("cloud-a", "old")
	if err != nil {
		t.Fatal(err)
	}
	if got := row.Timeline[len(row.Timeline)-1].Owner; !strings.HasPrefix(got, "cloud-a@") {
		t.Fatalf("untrusted actor accepted: %q", got)
	}
}

func TestWithdrawTaskLeaseRejectsCopiedTokenFromAnotherSession(t *testing.T) {
	h := newIdentityHarness(t)
	owner := h.client("codex-fence")
	owner.threadID = "own-thread"
	if err := owner.AddTask(Task{Agent: "codex-fence", TaskID: "old", Subject: "old"}); err != nil {
		t.Fatal(err)
	}
	if err := owner.AddTask(Task{Agent: "codex-fence", TaskID: "next", Subject: "next"}); err != nil {
		t.Fatal(err)
	}
	lease, err := owner.ClaimTask("codex-fence", "old", "worker", owner.threadID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	foreign := h.client("codex-fence")
	foreign.threadID = "other-thread"
	if err = foreign.WithdrawTaskLease("codex-fence", "old", lease.Token, "codex-fence/next", "forged", "dup"); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("foreign copied token: %v", err)
	}
	if err = owner.WithdrawTaskLease("codex-fence", "old", lease.Token, "codex-fence/next", "forged", "dup"); err != nil {
		t.Fatalf("owner refused: %v", err)
	}
}

// A suspended worker cannot spend a lease which expires after its host clock
// sample but before the mutation: the database must check its own wall clock.
func TestWithdrawTaskLeaseExpiresAfterClockSample(t *testing.T) {
	s := openTestStore(t)
	for _, id := range []string{"old", "next"} {
		if err := s.AddTask(Task{Agent: "lane", TaskID: id, Subject: id}); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := s.ClaimTask("lane", "old", "worker", "thread", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time {
		sample := time.Now().UTC()
		time.Sleep(time.Until(lease.Expires.Add(time.Second)))
		return sample
	}
	if err = s.WithdrawTaskLease("lane", "old", lease.Token, "lane/next", "lane@host", "dup"); !errors.Is(err, ErrLeaseInvalid) {
		t.Fatalf("expired lease spent after clock sample: %v", err)
	}
}

func TestWithdrawTaskLeaseOppositeDirectionsSerialize(t *testing.T) {
	s := openTestStore(t)
	peer := taskEvidencePeer(t, s)
	for _, id := range []string{"a", "b"} {
		if err := s.AddTask(Task{Agent: "lane", TaskID: id, Subject: id}); err != nil {
			t.Fatal(err)
		}
	}
	a, err := s.ClaimTask("lane", "a", "worker-a", "thread-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.ClaimTask("lane", "b", "worker-b", "thread-b", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	done := make(chan error, 2)
	go func() { <-start; done <- s.WithdrawTaskLease("lane", "a", a.Token, "lane/b", "lane@host", "dup") }()
	go func() { <-start; done <- peer.WithdrawTaskLease("lane", "b", b.Token, "lane/a", "lane@host", "dup") }()
	close(start)
	var success, refused int
	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			if err == nil {
				success++
			} else if errors.Is(err, ErrInvalidInput) {
				refused++
			} else {
				t.Fatalf("unexpected result: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("opposite-direction withdrawals deadlocked")
		}
	}
	if success != 1 || refused != 1 {
		t.Fatalf("success=%d refused=%d", success, refused)
	}
}

func TestWithdrawTaskLeaseEnforcesRegisteredClaimSession(t *testing.T) {
	var logs strings.Builder
	store, clients := ruleHarness(t, "enforce", &logs)
	backend := store.(*SQLiteStore)
	for _, id := range []string{"old", "next"} {
		if err := backend.AddTask(Task{Agent: "lane", TaskID: id, Subject: id}); err != nil {
			t.Fatal(err)
		}
	}
	owner := clients("lane", "owner-thread")
	register(t, backend, "owner-thread", "lane", owner.host, "active", owner.now())
	lease, err := owner.ClaimTask("lane", "old", "worker", "owner-thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	other := clients("lane", "other-thread")
	register(t, backend, "other-thread", "lane", other.host, "active", other.now())
	if err = other.WithdrawTaskLease("lane", "old", lease.Token, "lane/next", "forged", "dup"); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("other registered thread spent token: %v", err)
	}
	if err = owner.WithdrawTaskLease("lane", "old", lease.Token, "lane/next", "forged", "dup"); err != nil {
		t.Fatalf("owner denied: %v", err)
	}
	row, err := backend.GetTask("lane", "old")
	if err != nil || row.Status != "withdrawn" {
		t.Fatalf("row=%+v err=%v", row, err)
	}
}
