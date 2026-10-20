package routerstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	dc "github.com/SirsiMaster/sirsi-pantheon/internal/desktopcustody"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

type custodyTestPower struct{ sleeps, cancels int }

func (*custodyTestPower) ScheduleWake(_ context.Context, _ dc.SleepAdmission, r dc.SleepRequest) (dc.WakeReceipt, error) {
	return dc.WakeReceipt{ID: "pantheon-sleep-" + r.ID, Host: r.Host, WakeAt: r.WakeAt, OwnerTag: "pantheon-sleep-" + r.ID, Readback: "owned wake"}, nil
}
func (*custodyTestPower) ReadWake(context.Context, dc.SleepAdmission, dc.SleepRequest, dc.WakeReceipt) error {
	return nil
}
func (p *custodyTestPower) Sleep(context.Context) error { p.sleeps++; return nil }
func (p *custodyTestPower) CancelWake(context.Context, dc.SleepAdmission, dc.SleepRequest, dc.WakeReceipt) error {
	p.cancels++
	return nil
}

type invocationFailJournal struct {
	dc.SleepJournal
	phase dc.SleepPhase
}

func (j invocationFailJournal) AppendSleep(e dc.SleepEvent) error {
	if e.Phase == j.phase {
		return errors.New("injected invocation journal write failure")
	}
	return j.SleepJournal.AppendSleep(e)
}
func custodyJoinedService(t *testing.T, f *custodyFixture, auth dc.OnlineAuthority) (dc.SleepService, *custodyTestPower) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "journal")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	j, closeJournal, err := dc.OpenNativeJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeJournal() })
	c := &dc.OnlineController{Authority: auth, ID: f.d.Request.ID, Identity: func(context.Context) (dc.HostIdentity, error) {
		return dc.HostIdentity{Host: "m5", PlatformUUID: f.d.Admission.PlatformUUID, BootID: f.d.Admission.BootID, ExecutableSHA256: f.d.ExecutableSHA256}, nil
	}, Inventory: func(context.Context) (string, error) { return f.d.InventorySHA256, nil }, Budget: func(context.Context) (dc.ExecutionBudget, error) { return f.budget, nil }}
	p := &custodyTestPower{}
	return dc.SleepService{Verifier: c, Custodian: c, Power: p, Journal: j, Now: func() time.Time { return f.now }}, p
}
func TestDesktopCustodyJoinedJournalFailureAbortsCanonicalConsumption(t *testing.T) {
	f := newCustodyFixture(t)
	f.ready(t)
	svc, p := custodyJoinedService(t, f, f.clients["custodian"])
	svc.Journal = invocationFailJournal{svc.Journal, dc.SleepInvoked}
	if err := svc.Dispatch(context.Background(), f.d.Request); !errors.Is(err, dc.ErrSleepAbortedBeforeNative) {
		t.Fatal(err)
	}
	state, err := f.call("custodian", "status")
	if err != nil {
		t.Fatal(err)
	}
	if state.ConsumedAt.IsZero() || state.AbortedAt.IsZero() || !state.NativeAuthorizedAt.IsZero() || state.Return != nil || p.sleeps != 0 || p.cancels != 1 {
		t.Fatalf("invalid pre-native compensation: %+v power=%+v", state, p)
	}
	if _, err := f.s.MaatReserve(custodyReservation(f.now, "m5"), false); err != nil {
		t.Fatal("canonical fence stranded ordinary work", err)
	}
	if _, err := f.call("custodian", "consume"); err == nil {
		t.Fatal("aborted nonce revived")
	}
	if err := f.s.CreateDesktopSleep(f.d); err != nil {
		t.Fatal(err)
	}
	if _, err := f.call("custodian", "invoke"); err == nil {
		t.Fatal("idempotent management reapply revived native authority")
	}
	if err := svc.AbortBeforeNative(context.Background(), f.d.Request); !errors.Is(err, dc.ErrSleepAbortedBeforeNative) {
		t.Fatal(err)
	}
	if p.cancels != 1 {
		t.Fatal("idempotent abort repeated cleanup")
	}
	events, err := svc.Journal.ReadSleep(f.d.Request.ID)
	if err != nil || events[len(events)-1].Phase != dc.SleepAborted {
		t.Fatalf("abort journal: %+v %v", events, err)
	}
}

type lostInvocationResponse struct {
	dc.OnlineAuthority
	afterCommit bool
	aborts      int
}

func (a *lostInvocationResponse) DesktopSleep(ctx context.Context, c dc.CustodyCall) (dc.CustodyState, error) {
	if c.Operation == "abort-before-native" {
		a.aborts++
	}
	if c.Operation == "invoke" {
		if a.afterCommit {
			if _, err := a.OnlineAuthority.DesktopSleep(ctx, c); err != nil {
				return dc.CustodyState{}, err
			}
		}
		return dc.CustodyState{}, errors.New("lost invocation response")
	}
	return a.OnlineAuthority.DesktopSleep(ctx, c)
}
func TestDesktopCustodyLostInvocationResponseNeverSleepsOrAutomaticallyAborts(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-commit", true: "after-commit"}[after], func(t *testing.T) {
			f := newCustodyFixture(t)
			f.ready(t)
			auth := &lostInvocationResponse{OnlineAuthority: f.clients["custodian"], afterCommit: after}
			svc, p := custodyJoinedService(t, f, auth)
			if err := svc.Dispatch(context.Background(), f.d.Request); !errors.Is(err, dc.ErrSleepUnknown) {
				t.Fatal(err)
			}
			if p.sleeps != 0 || p.cancels != 0 || auth.aborts != 0 {
				t.Fatalf("uncertain invoke retried or compensated: %+v aborts=%d", p, auth.aborts)
			}
			if _, err := f.s.MaatReserve(custodyReservation(f.now, "m5"), false); err == nil {
				t.Fatal("unknown invocation released canonical fence")
			}
			err := svc.AbortBeforeNative(context.Background(), f.d.Request)
			if after {
				if err == nil || errors.Is(err, dc.ErrSleepAbortedBeforeNative) || p.cancels != 0 {
					t.Fatal("server-authorized but uninvoked state force-cleared", err)
				}
			} else {
				if !errors.Is(err, dc.ErrSleepAbortedBeforeNative) || p.cancels != 1 {
					t.Fatal("explicit server-proved pre-native recovery failed", err)
				}
				if _, err := f.s.MaatReserve(custodyReservation(f.now, "m5"), false); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
func TestDesktopCustodyInvocationAbortRaceHasExactlyOneWinner(t *testing.T) {
	for i := 0; i < 8; i++ {
		f := newCustodyFixture(t)
		f.ready(t)
		if _, err := f.call("custodian", "consume"); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, op := range []string{"invoke", "abort-before-native"} {
			wg.Add(1)
			go func(op string) { defer wg.Done(); <-start; _, err := f.call("custodian", op); results <- err }(op)
		}
		close(start)
		wg.Wait()
		close(results)
		wins := 0
		for err := range results {
			if err == nil {
				wins++
			}
		}
		if wins != 1 {
			t.Fatalf("invoke/abort winners %d", wins)
		}
		s, err := f.call("custodian", "status")
		if err != nil {
			t.Fatal(err)
		}
		if s.NativeAuthorizedAt.IsZero() == s.AbortedAt.IsZero() {
			t.Fatalf("non-exclusive terminal phases: %+v", s)
		}
	}
}
func TestDesktopCustodyAbortScopeAndExpiredRevokedRecovery(t *testing.T) {
	f := newCustodyFixture(t)
	f.ready(t)
	if _, err := f.call("custodian", "consume"); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"coordinator", "observer"} {
		if _, err := f.call(role, "abort-before-native"); err == nil {
			t.Fatal("other participant aborted custodian", role)
		}
	}
	if err := f.s.RevokeDesktopSleep(f.d.Request.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(6 * time.Minute)
	p := f.d.Custodian
	if err := f.s.UpsertThreads([]ThreadRecord{{ThreadID: p.ThreadID, Agent: p.Agent, Host: p.Host, Status: "active", LastSeenAt: f.now.Format(time.RFC3339Nano), Payload: []byte(`{}`)}}); err != nil {
		t.Fatal(err)
	}
	s, err := f.call("custodian", "abort-before-native")
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.call("custodian", "abort-before-native")
	if err != nil || !again.AbortedAt.Equal(s.AbortedAt) || !again.ConsumedAt.Equal(s.ConsumedAt) {
		t.Fatal("abort replay changed immutable lifecycle", err)
	}
	if _, err := f.s.MaatReserve(custodyReservation(f.now, "m5"), false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.call("custodian", "invoke"); err == nil {
		t.Fatal("expired/revoked abort revived native authority")
	}
}

func TestDesktopCustodyNativeAuthorizedJournalFailureRetainsBothFences(t *testing.T) {
	f := newCustodyFixture(t)
	f.ready(t)
	svc, p := custodyJoinedService(t, f, f.clients["custodian"])
	svc.Journal = invocationFailJournal{svc.Journal, dc.SleepNativeReturned}
	if err := svc.Dispatch(context.Background(), f.d.Request); !errors.Is(err, dc.ErrSleepUnknown) {
		t.Fatal(err)
	}
	if p.sleeps != 1 || p.cancels != 0 {
		t.Fatalf("authorized native failure cleanup unsafe: %+v", p)
	}
	if err := svc.AbortBeforeNative(context.Background(), f.d.Request); err == nil || errors.Is(err, dc.ErrSleepAbortedBeforeNative) {
		t.Fatal("authorized request compensated", err)
	}
	if _, err := f.s.MaatReserve(custodyReservation(f.now, "m5"), false); err == nil {
		t.Fatal("authorized unknown fence removed")
	}
}
func TestDesktopCustodyLegacyConsumedRecordCannotBePreNativeAborted(t *testing.T) {
	f := newCustodyFixture(t)
	f.ready(t)
	if _, err := f.call("custodian", "consume"); err != nil {
		t.Fatal(err)
	}
	// Simulate an older persisted record whose consumption did not require the
	// server-owned invocation phase. Missing phase is never proof of no power.
	if err := f.s.withMaatTransaction(func(tx *txHandle, _ *schedule.Ledger) error {
		r, err := readSleepRegistry(tx)
		if err != nil {
			return err
		}
		v := r.Records[f.d.Request.ID]
		v.NativeGateVersion = 0
		r.Records[f.d.Request.ID] = v
		return writeSleepRegistry(tx, r)
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.CreateDesktopSleep(f.d); err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{"abort-before-native", "invoke"} {
		if _, err := f.call("custodian", op); err == nil {
			t.Fatal("legacy record promoted by restart", op)
		}
	}
	if _, err := f.s.MaatReserve(custodyReservation(f.now, "m5"), false); err == nil {
		t.Fatal("legacy unknown fence cleared")
	}
}
func TestDesktopCustodyNativeAuthorizationRechecksCurrentAuthority(t *testing.T) {
	for _, mode := range []string{"revoked", "expired", "boot", "peer", "budget"} {
		t.Run(mode, func(t *testing.T) {
			f := newCustodyFixture(t)
			f.ready(t)
			if _, err := f.call("custodian", "consume"); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "revoked":
				if err := f.s.RevokeDesktopSleep(f.d.Request.ID, "cancel"); err != nil {
					t.Fatal(err)
				}
			case "expired":
				f.now = f.d.Admission.NotAfter
			case "boot":
				f.d.Admission.BootID = "wrong-boot"
			case "peer":
				p := f.d.Observer
				if err := f.s.UpsertThreads([]ThreadRecord{{ThreadID: p.ThreadID, Agent: p.Agent, Host: p.Host, Status: "reaped", LastSeenAt: f.now.Format(time.RFC3339Nano), Payload: []byte(`{}`)}}); err != nil {
					t.Fatal(err)
				}
			case "budget":
				f.budget.SupervisorPID++
			}
			if _, err := f.call("custodian", "invoke"); err == nil {
				t.Fatal("changed authority authorized native", mode)
			}
			s, err := f.call("custodian", "status")
			if err != nil {
				t.Fatal(err)
			}
			if !s.NativeAuthorizedAt.IsZero() {
				t.Fatal("rejected native authorization changed phase")
			}
		})
	}
}
