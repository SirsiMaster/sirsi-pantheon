package routerstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func barrierCtx() context.Context {
	return context.WithValue(context.Background(), laneClaimBarrierActorKey{}, laneClaimBarrierActor{"sirsi-software-admin", "host", "thread"})
}
func TestLaneClaimBarrierOwnershipSettlement(t *testing.T) {
	s := newTestStore(t)
	ctx := barrierCtx()
	id, err := s.Send("ra", "gemma-pantheon", "real", "review", "x")
	if err != nil {
		t.Fatal(err)
	}
	l, err := s.ClaimItem("gemma-pantheon", id, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AddTask(Task{Agent: "gemma-pantheon", TaskID: "task", Subject: "work", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	tl, err := s.ClaimTask("gemma-pantheon", "task", "worker", "thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.LaneClaimBarrier(ctx, LaneClaimBarrierRequest{Action: "acquire", Lane: "gemma-pantheon", Token: "0123456789abcdef0123456789abcdef"}) // gitleaks:allow -- fixed synthetic capability in isolated store fixture only
	if err != nil {
		t.Fatal(err)
	}
	if b.ActiveItems != 1 || b.ActiveTasks != 1 || b.Token == "" {
		t.Fatalf("bad settlement counts %d %d", b.ActiveItems, b.ActiveTasks)
	}
	for _, f := range []func() error{func() error { _, e := s.ClaimNext("gemma-pantheon", time.Minute); return e }, func() error { _, e := s.ClaimItem("gemma-pantheon", id, time.Minute); return e }, func() error { _, e := s.ClaimTask("gemma-pantheon", "task", "w", "t", time.Minute); return e }, func() error { _, e := s.ClaimNextTask("gemma-pantheon", "w", "t", time.Minute); return e }} {
		if e := f(); !errors.Is(e, ErrLanePaused) {
			t.Fatalf("claim passed pause: %v", e)
		}
	}
	if err = s.Complete(id, l.Token, "done"); err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteTaskLease("gemma-pantheon", "task", tl.Token, "evidence"); err != nil {
		t.Fatal(err)
	}
	st, err := s.LaneClaimBarrier(ctx, LaneClaimBarrierRequest{"status", "gemma-pantheon", b.Token, ""})
	if err != nil || st.ActiveItems != 0 || st.ActiveTasks != 0 {
		t.Fatal("not settled", err)
	}
	if _, e := s.LaneClaimBarrier(ctx, LaneClaimBarrierRequest{"release", "gemma-pantheon", "wrong", ""}); e == nil {
		t.Fatal("wrong capability accepted")
	}
	if e := s.SetState(laneClaimBarrierKey, "{}"); e == nil {
		t.Fatal("generic overwrite")
	}
	if _, e := s.CompareAndSwapState(laneClaimBarrierKey, "", false, "{}"); e == nil {
		t.Fatal("generic CAS")
	}
	if _, e := s.LaneClaimBarrier(context.Background(), LaneClaimBarrierRequest{"acquire", "gemma-pantheon", "0123456789abcdef0123456789abcdef", ""}); e == nil {
		t.Fatal("unbound actor")
	}
	if _, e := s.ClaimNext("ra", time.Minute); !errors.Is(e, ErrNoWork) {
		t.Fatal("sibling affected", e)
	}
	if _, e := s.LaneClaimBarrier(ctx, LaneClaimBarrierRequest{"release", "gemma-pantheon", b.Token, ""}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.LaneClaimBarrier(ctx, LaneClaimBarrierRequest{"release", "gemma-pantheon", b.Token, ""}); e == nil {
		t.Fatal("replay accepted")
	}
}
func TestLaneClaimBarrierPauseVsClaim(t *testing.T) {
	for n := 0; n < 8; n++ {
		for _, exact := range []bool{false, true} {
			s := newTestStore(t)
			id, e := s.Send("ra", "gemma-pantheon", "race", "review", "x")
			if e != nil {
				t.Fatal(e)
			}
			start := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(2)
			var claimErr, pauseErr error
			var b LaneClaimBarrierResult
			go func() {
				defer wg.Done()
				<-start
				if exact {
					_, claimErr = s.ClaimItem("gemma-pantheon", id, time.Minute)
				} else {
					_, claimErr = s.ClaimNext("gemma-pantheon", time.Minute)
				}
			}()
			go func() {
				defer wg.Done()
				<-start
				b, pauseErr = s.LaneClaimBarrier(barrierCtx(), LaneClaimBarrierRequest{"acquire", "gemma-pantheon", "0123456789abcdef0123456789abcdef", ""})
			}()
			close(start)
			wg.Wait()
			if pauseErr != nil {
				t.Fatal(pauseErr)
			}
			if claimErr == nil {
				if b.ActiveItems != 1 {
					t.Fatal("claim invisible to barrier")
				}
			} else if !errors.Is(claimErr, ErrLanePaused) || b.ActiveItems != 0 {
				t.Fatal("nonserial result", claimErr)
			}
			if _, e = s.ClaimItem("gemma-pantheon", id, time.Minute); !errors.Is(e, ErrLanePaused) {
				t.Fatal("postbarrier claim", e)
			}
			// Both callers are joined. Do not retain sixteen idle PG pools until
			// parent cleanup; its later Close remains an idempotent fallback.
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestLaneClaimBarrierWireIdentity(t *testing.T) {
	var logs strings.Builder
	backend, client := ruleHarness(t, "off", &logs)
	host, _ := os.Hostname()
	now := time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC)
	register(t, backend, "controller", "sirsi-software-admin", host, "active", now)
	c := client("sirsi-software-admin", "controller")
	b, e := c.LaneClaimBarrier(context.Background(), LaneClaimBarrierRequest{"acquire", "gemma-pantheon", "0123456789abcdef0123456789abcdef", ""})
	if e != nil {
		t.Fatal(e)
	}
	register(t, backend, "gemma-worker", "gemma-pantheon", host, "active", now)
	worker := client("gemma-pantheon", "gemma-worker")
	id, e := backend.Send("ra", "gemma-pantheon", "paused", "review", "fixture")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = worker.ClaimNext("gemma-pantheon", time.Minute); !errors.Is(e, ErrLanePaused) {
		t.Fatal("pause sentinel lost across HTTP", e)
	}
	if _, e = worker.ClaimItem("gemma-pantheon", id, time.Minute); !errors.Is(e, ErrLanePaused) {
		t.Fatal("exact pause sentinel lost across HTTP", e)
	}
	if _, e = c.LaneClaimBarrier(context.Background(), LaneClaimBarrierRequest{"status", "ra", b.Token, ""}); e == nil {
		t.Fatal("foreign lane accepted")
	}
	register(t, backend, "other", "ra", host, "active", now)
	other := client("ra", "other")
	if _, e = other.LaneClaimBarrier(context.Background(), LaneClaimBarrierRequest{"release", "gemma-pantheon", b.Token, ""}); e == nil {
		t.Fatal("foreign actor accepted")
	}
	c = client("sirsi-software-admin", "controller")
	if _, e = c.LaneClaimBarrier(context.Background(), LaneClaimBarrierRequest{"release", "gemma-pantheon", b.Token, ""}); e != nil {
		t.Fatal(e)
	}
}

// Independent connections exercise the database fence, not a process mutex.
func TestLaneClaimBarrierSeparateConnections(t *testing.T) {
	for _, kind := range []string{"next", "item", "task", "next-task"} {
		path := filepath.Join(t.TempDir(), "shared.db")
		s := openBackendStore(t, path)
		defer s.Close()
		var other *SQLiteStore
		var err error
		if dsn := pgTestDSN(); dsn != "" {
			other, err = OpenPostgres(dsn)
		} else {
			other, err = OpenPath(path)
		}
		if err != nil {
			t.Fatal(err)
		}
		defer other.Close()
		id, e := s.Send("ra", "gemma-pantheon", kind, "review", "x")
		if e != nil {
			t.Fatal(e)
		}
		if e = s.AddTask(Task{Agent: "gemma-pantheon", TaskID: kind, Subject: kind, Status: "pending"}); e != nil {
			t.Fatal(e)
		}
		ready := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var ce, pe error
		var b LaneClaimBarrierResult
		go func() {
			defer wg.Done()
			<-ready
			switch kind {
			case "next":
				_, ce = other.ClaimNext("gemma-pantheon", time.Minute)
			case "item":
				_, ce = other.ClaimItem("gemma-pantheon", id, time.Minute)
			case "task":
				_, ce = other.ClaimTask("gemma-pantheon", kind, "w", "t", time.Minute)
			case "next-task":
				_, ce = other.ClaimNextTask("gemma-pantheon", "w", "t", time.Minute)
			}
		}()
		go func() {
			defer wg.Done()
			<-ready
			b, pe = s.LaneClaimBarrier(barrierCtx(), LaneClaimBarrierRequest{"acquire", "gemma-pantheon", "0123456789abcdef0123456789abcdef", ""})
		}()
		close(ready)
		wg.Wait()
		if pe != nil {
			t.Fatal(pe)
		}
		if ce != nil && !errors.Is(ce, ErrLanePaused) {
			t.Fatal(ce)
		}
		if ce == nil && b.ActiveItems+b.ActiveTasks != 1 || ce != nil && b.ActiveItems+b.ActiveTasks != 0 {
			t.Fatal("cross-connection claim escaped snapshot")
		}
		// Each race has an isolated database so an earlier winner cannot mask a missed claim.
		if _, e = s.LaneClaimBarrier(barrierCtx(), LaneClaimBarrierRequest{"release", "gemma-pantheon", b.Token, ""}); e != nil {
			t.Fatal(e)
		}
	}
}

func TestLaneClaimBarrierRecoveryRequiresClosedExactOwner(t *testing.T) {
	s := newTestStore(t)
	ctx := barrierCtx()
	old := "0123456789abcdef0123456789abcdef"
	next := "abcdef0123456789abcdef0123456789"
	if _, err := s.LaneClaimBarrier(ctx, LaneClaimBarrierRequest{Action: "acquire", Lane: "gemma-pantheon", Token: old}); err != nil {
		t.Fatal(err)
	}
	newer := context.WithValue(context.Background(), laneClaimBarrierActorKey{}, laneClaimBarrierActor{"sirsi-software-admin", "host", "new-thread"})
	req := LaneClaimBarrierRequest{Action: "recover", Lane: "gemma-pantheon", Token: next, PreviousThread: "thread"}
	if _, err := s.LaneClaimBarrier(newer, req); err == nil {
		t.Fatal("unknown previous thread recovered")
	}
	for i, status := range []string{"active", "idle", "blocked"} {
		if err := s.UpsertThreads([]ThreadRecord{{ThreadID: "thread", Agent: "sirsi-software-admin", Host: "host", Status: status, LastSeenAt: s.clock().Add(time.Duration(i) * time.Second).Format(time.RFC3339), Payload: []byte(`{}`)}}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.LaneClaimBarrier(newer, req); err == nil {
			t.Fatalf("recovered nonterminal %s", status)
		}
	}
	if err := s.UpsertThreads([]ThreadRecord{{ThreadID: "thread", Agent: "sirsi-software-admin", Host: "host", Status: "closed", LastSeenAt: s.clock().Add(5 * time.Second).Format(time.RFC3339), Payload: []byte(`{}`)}}); err != nil {
		t.Fatal(err)
	}
	foreign := context.WithValue(context.Background(), laneClaimBarrierActorKey{}, laneClaimBarrierActor{"sirsi-software-admin", "other-host", "new-thread"})
	if _, err := s.LaneClaimBarrier(foreign, req); err == nil {
		t.Fatal("foreign host recovered")
	}
	for i := 0; i < 2; i++ {
		if _, err := s.LaneClaimBarrier(newer, req); err != nil {
			t.Fatal("recover/retry", err)
		}
	}
	if _, err := s.LaneClaimBarrier(ctx, LaneClaimBarrierRequest{Action: "release", Lane: "gemma-pantheon", Token: old}); err == nil {
		t.Fatal("old owner released recovered barrier")
	}
	if _, err := s.ClaimNext("gemma-pantheon", time.Minute); !errors.Is(err, ErrLanePaused) {
		t.Fatal("recovery unpaused lane", err)
	}
	if _, err := s.LaneClaimBarrier(newer, LaneClaimBarrierRequest{Action: "release", Lane: "gemma-pantheon", Token: next}); err != nil {
		t.Fatal(err)
	}
}

func TestLaneClaimBarrierSuspendedCannotRecover(t *testing.T) {
	for _, status := range []string{"suspended", "reaped"} {
		s := newTestStore(t)
		if err := s.UpsertThreads([]ThreadRecord{{ThreadID: "thread", Agent: "sirsi-software-admin", Host: "host", Status: status, LastSeenAt: s.clock().Format(time.RFC3339), Payload: []byte(`{}`)}}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.LaneClaimBarrier(barrierCtx(), LaneClaimBarrierRequest{Action: "acquire", Lane: "gemma-pantheon", Token: "0123456789abcdef0123456789abcdef"}); err != nil { // gitleaks:allow -- fixed synthetic capability in isolated store fixture only
			t.Fatal(err)
		}
		newer := context.WithValue(context.Background(), laneClaimBarrierActorKey{}, laneClaimBarrierActor{"sirsi-software-admin", "host", "new-thread"})
		if _, err := s.LaneClaimBarrier(newer, LaneClaimBarrierRequest{Action: "recover", Lane: "gemma-pantheon", Token: "abcdef0123456789abcdef0123456789", PreviousThread: "thread"}); err == nil { // gitleaks:allow -- fixed synthetic capability in isolated store fixture only
			t.Fatalf("recovered %s", status)
		}
	}
}

func TestLaneClaimBarrierPostgresControllerDeadlinePreservesPause(t *testing.T) {
	if pgTestDSN() == "" {
		t.Skip("requires an explicitly owned isolated PostgreSQL fixture; resetPostgres drops its router schema")
	}
	s := newTestStore(t)
	s.db.SetMaxOpenConns(1)                          // Identify the same retained connection used by the controller.
	const token = "0123456789abcdef0123456789abcdef" // gitleaks:allow -- fixed synthetic capability in isolated store fixture only
	if _, err := s.LaneClaimBarrier(barrierCtx(), LaneClaimBarrierRequest{Action: "acquire", Lane: "gemma-pantheon", Token: token}); err != nil {
		t.Fatal(err)
	}
	probeCtx, stopProbe := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopProbe()
	var before string
	if err := s.db.QueryRowContext(probeCtx, `SELECT value FROM state WHERE key=?`, laneClaimBarrierKey).Scan(&before); err != nil {
		t.Fatal(err)
	}
	var controllerPID int
	if err := s.db.QueryRowContext(probeCtx, `SELECT pg_backend_pid()`).Scan(&controllerPID); err != nil {
		t.Fatal(err)
	}
	// Open another pool without resetting the fixture schema. Its transaction
	// holds the actual durable row until this test explicitly rolls it back.
	other, err := OpenPostgres(pgTestDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	blocker, err := other.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	var blockerPID int
	if err := blocker.QueryRowContext(probeCtx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		t.Fatal(err)
	}
	if _, err := blocker.ExecContext(probeCtx, `UPDATE state SET value=value WHERE key=?`, laneClaimBarrierKey); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(barrierCtx(), 2*time.Second)
	defer cancel()
	done, exited := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(exited)
		_, err := s.LaneClaimBarrier(ctx, LaneClaimBarrierRequest{Action: "release", Lane: "gemma-pantheon", Token: token})
		done <- err
	}()
	// ALL exits cancel, unblock, then join. A negative control must not leave a
	// context-free SQL statement or its caller running beyond this fixture.
	defer func() {
		cancel()
		_ = blocker.Rollback()
		select {
		case <-exited:
		case <-time.After(3 * time.Second):
			t.Error("controller did not join after fixture lock release")
		}
	}()
	blocked := false
	for ctx.Err() == nil {
		if err := other.db.QueryRowContext(probeCtx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE pid=? AND wait_event_type='Lock' AND ?=ANY(pg_blocking_pids(pid)))`, controllerPID, blockerPID).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !blocked {
		t.Fatal("controller never demonstrably waited on the independent fixture row lock")
	}
	// The blocker is deliberately still held here: returning only after its
	// rollback is not deadline cancellation and must fail this assertion.
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("want caller deadline while blocker remains held, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("controller remained blocked after its caller deadline")
	}
	if err := blocker.Rollback(); err != nil {
		t.Fatal(err)
	}
	<-exited
	var after string
	if err := s.db.QueryRowContext(probeCtx, `SELECT value FROM state WHERE key=?`, laneClaimBarrierKey).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("canceled release changed durable owner/capability")
	}
	status, err := s.LaneClaimBarrier(barrierCtx(), LaneClaimBarrierRequest{Action: "status", Lane: "gemma-pantheon", Token: token})
	if err != nil || !status.Paused {
		t.Fatalf("original owner/capability no longer holds the pause: %v", err)
	}
	if _, err := s.ClaimNext("gemma-pantheon", time.Minute); !errors.Is(err, ErrLanePaused) {
		t.Fatalf("deadline silently resumed claims: %v", err)
	}
}
