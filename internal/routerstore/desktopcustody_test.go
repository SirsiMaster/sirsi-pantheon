package routerstore

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	dc "github.com/SirsiMaster/sirsi-pantheon/internal/desktopcustody"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

type custodyFixture struct {
	s       *SQLiteStore
	d       dc.Delegation
	now     time.Time
	clients map[string]*RemoteStore
	tokens  map[string]string
	budget  dc.ExecutionBudget
}

func newCustodyFixture(t *testing.T) *custodyFixture {
	t.Helper()
	s := openBackendStore(t, filepath.Join(t.TempDir(), "custody.db"))
	t.Cleanup(func() { s.Close() })
	f := &custodyFixture{s: s, now: time.Date(2026, 10, 20, 6, 0, 0, 0, time.UTC), clients: map[string]*RemoteStore{}, tokens: map[string]string{}}
	s.now = func() time.Time { return f.now }
	h, err := Handler(s, ServerOptions{Token: "bootstrap", RuleOfRa: "enforce", now: func() time.Time { return f.now }})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	sha := strings.Repeat("a", 64)
	makeParticipant := func(role, host, mid string) dc.Participant {
		tok, rec, err := s.MintScopedHostToken(host, role, []string{role})
		if err != nil {
			t.Fatal(err)
		}
		if mid != "" {
			if err = s.AdoptTokenMachineID(host, mid); err != nil {
				t.Fatal(err)
			}
		}
		thread := role + "-thread"
		if err = s.UpsertThreads([]ThreadRecord{{ThreadID: thread, Agent: role, Host: host, Status: "active", LastSeenAt: f.now.Format(time.RFC3339Nano), Payload: []byte(`{}`)}}); err != nil {
			t.Fatal(err)
		}
		rs := NewRemoteStore(srv.URL, tok)
		rs.host = host
		rs.agent = role
		rs.runtime = sha
		rs.threadID = thread
		rs.sessionDir = ""
		rs.now = func() time.Time { return f.now }
		if _, err = rs.ensureSession(context.Background()); err != nil {
			t.Fatal(err)
		}
		f.clients[role] = rs
		f.tokens[role] = tok
		return dc.Participant{TokenID: rec.ID, SessionID: rs.session.ID, MachineID: mid, Host: host, Agent: role, ThreadID: thread, RuntimeSHA256: sha}
	}
	f.d = dc.Delegation{Request: dc.SleepRequest{ID: "sleep-one", Host: "m5", WakeAt: f.now.Add(30 * time.Second)}, Admission: dc.SleepAdmission{ID: "sleep-one", Issuer: "sirsi-router", Host: "m5", Owner: "owner", PlatformUUID: "platform-m5", BootID: "boot-one", Operation: dc.SleepOperation, RequestID: "sleep-one", Nonce: "nonce-one", ReviewDigest: sha, NotBefore: f.now.Add(-time.Second), NotAfter: f.now.Add(60 * time.Second), CustodyGeneration: "generation-one"}, OwnerRequestRef: "owner-thread/request", OwnerRequestSHA256: sha, ExecutableSHA256: sha, CandidateSHA256: sha, InventorySHA256: sha, ObserverMachineLabel: "m1"}
	f.d.Coordinator = makeParticipant("coordinator", "cloud-ssa", "")
	f.d.Custodian = makeParticipant("custodian", "host-m5", "platform-m5")
	f.d.Observer = makeParticipant("observer", "host-m1", "platform-m1")
	f.budget = dc.ExecutionBudget{AdmissionID: "controller-one", Agent: f.d.Custodian.Agent, ArgvSHA256: sha, BudgetSeconds: 600, ExecutionDeadline: f.now.Add(10 * time.Minute).Format(time.RFC3339), ExecutionStarted: f.now.Format(time.RFC3339), Resource: "m5", SupervisorPID: 444, SupervisorStart: "Tue Oct 20 06:00:00 2026", ThreadID: f.d.Custodian.ThreadID, Work: "controller-job"}
	f.d.CustodianReservationID = f.budget.AdmissionID
	f.d.CustodianWork = f.budget.Work
	f.d.ControllerBudget = f.budget
	f.d.CustodianBudgetSHA256 = sleepDigest(f.budget)
	controller := custodyReservation(f.now, "m5")
	controller.ID = f.budget.AdmissionID
	controller.Holder = f.budget.Agent
	controller.Work = f.budget.Work
	controller.ExemptPID = f.budget.SupervisorPID
	controller.LeaseTTLSec = 240
	controller.EstEnd = ""
	if _, err := f.s.MaatReserve(controller, false); err != nil {
		t.Fatal(err)
	}
	if err = s.CreateDesktopSleep(f.d); err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *custodyFixture) call(role, op string) (dc.CustodyState, error) {
	c := dc.CustodyCall{Operation: op, ID: f.d.Request.ID, Role: role, QuietDigest: strings.Repeat("b", 64)}
	if role == "custodian" {
		c.Identity = dc.HostIdentity{Host: "m5", PlatformUUID: f.d.Admission.PlatformUUID, BootID: f.d.Admission.BootID, ExecutableSHA256: f.d.ExecutableSHA256}
		c.InventorySHA256 = f.d.InventorySHA256
		c.Budget = &f.budget
	}
	return f.clients[role].DesktopSleep(context.Background(), c)
}
func (f *custodyFixture) ready(t *testing.T) {
	t.Helper()
	for _, role := range []string{"coordinator", "custodian", "observer"} {
		if _, err := f.call(role, "ready"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDesktopCustodyManagementOnlyAndServerActorBinding(t *testing.T) {
	f := newCustodyFixture(t)
	if err := f.clients["custodian"].call("CreateDesktopSleep", []any{f.d}); err == nil {
		t.Fatal("management creation exposed on wire")
	}
	if err := f.s.SetState(desktopSleepKey, `{}`); err == nil {
		t.Fatal("generic state overwrote authority")
	}
	if _, err := f.s.CompareAndSwapState(desktopSleepKey, "", false, `{}`); err == nil {
		t.Fatal("generic CAS overwrote authority")
	}
	if _, err := f.s.DesktopSleep(context.Background(), dc.CustodyCall{Operation: "consume", ID: f.d.Request.ID, Role: "custodian"}); err == nil {
		t.Fatal("unauthenticated local call authorized")
	}
	c := dc.CustodyCall{Operation: "inspect", ID: f.d.Request.ID, Role: "custodian", Identity: dc.HostIdentity{Host: "m5", PlatformUUID: f.d.Admission.PlatformUUID, BootID: f.d.Admission.BootID, ExecutableSHA256: f.d.ExecutableSHA256}, InventorySHA256: f.d.InventorySHA256}
	if _, err := f.clients["observer"].DesktopSleep(context.Background(), c); err == nil {
		t.Fatal("body role/identity impersonated custodian")
	}
	if _, err := f.call("custodian", "inspect"); err != nil {
		t.Fatal(err)
	}
}

func TestDesktopCustodyConsumesOnceAndRetainsFenceAfterRestartAndRevocation(t *testing.T) {
	f := newCustodyFixture(t)
	f.ready(t)
	if _, err := f.call("custodian", "consume"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.CreateDesktopSleep(f.d); err != nil {
		t.Fatal("identical revision reapply", err)
	}
	if _, err := f.call("custodian", "consume"); err == nil {
		t.Fatal("consumed twice")
	}
	if err := f.s.RevokeDesktopSleep(f.d.Request.ID, "owner canceled"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.CreateDesktopSleep(f.d); err != nil {
		t.Fatal(err)
	}
	s, err := f.call("custodian", "status")
	if err != nil || s.ConsumedAt.IsZero() || s.RevokedAt.IsZero() {
		t.Fatalf("terminal state reset: %+v %v", s, err)
	}
	changed := f.d
	changed.Admission.Nonce = "replacement"
	if err = f.s.CreateDesktopSleep(changed); err == nil {
		t.Fatal("revision expanded original record")
	}
	next := f.d
	next.Request.ID = "another"
	next.Admission.ID = "another"
	next.Admission.RequestID = "another"
	if err = f.s.CreateDesktopSleep(next); err == nil {
		t.Fatal("nonce reused under another id")
	}
	for _, resource := range []string{"m5", "cpu@m5", "host-m5", "platform-m5"} {
		if _, err = f.s.MaatReserve(custodyReservation(f.now, resource), false); err == nil {
			t.Fatal("consumed host admitted workload", resource)
		}
	}
}
func custodyReservation(now time.Time, host string) schedule.Reservation {
	return schedule.Reservation{ID: "new-work", Resource: host, Holder: "foreign", Work: "native-test", Regime: schedule.RegimeBuild, Start: now.Format(time.RFC3339), EstEnd: now.Add(time.Minute).Format(time.RFC3339)}
}

func TestDesktopCustodyRejectsStaleBootGenerationRevocationAndMissingPeers(t *testing.T) {
	for _, mode := range []string{"boot", "inventory", "stale", "revoked-session", "revoked-token", "missing-peer", "expired"} {
		t.Run(mode, func(t *testing.T) {
			f := newCustodyFixture(t)
			f.ready(t)
			switch mode {
			case "boot":
				f.d.Admission.BootID = "other"
			case "inventory":
				f.d.InventorySHA256 = strings.Repeat("c", 64)
			case "stale":
				f.now = f.now.Add(16 * time.Second)
			case "expired":
				f.now = f.now.Add(6 * time.Minute)
			case "revoked-session":
				if err := f.s.RevokeSession(f.d.Observer.SessionID); err != nil {
					t.Fatal(err)
				}
			case "revoked-token":
				if err := f.s.RevokeHostToken(f.d.Observer.TokenID); err != nil {
					t.Fatal(err)
				}
			case "missing-peer":
				if err := f.s.RevokeDesktopSleep(f.d.Request.ID, "cancel"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.call("custodian", "consume"); err == nil {
				t.Fatal("invalid custody accepted", mode)
			}
		})
	}
}

func TestDesktopCustodyRejectsNonActivePeerAfterReady(t *testing.T) {
	for _, role := range []string{"coordinator", "observer"} {
		for _, status := range []string{"reaped", "closed", "suspended", "idle", "unknown", ""} {
			t.Run(role+"/"+status, func(t *testing.T) {
				f := newCustodyFixture(t)
				f.ready(t)
				p := sleepRole(f.d, role)
				if err := f.s.UpsertThreads([]ThreadRecord{{ThreadID: p.ThreadID, Agent: p.Agent, Host: p.Host, Status: status, LastSeenAt: f.now.Add(time.Second).Format(time.RFC3339Nano), Payload: []byte(`{}`)}}); err != nil {
					t.Fatal(err)
				}
				binding, err := f.s.ThreadBinding(p.ThreadID)
				if err != nil || binding.Status != status {
					t.Fatalf("peer status mutation not applied: %+v %v", binding, err)
				}
				if _, err := f.call("custodian", "consume"); err == nil {
					t.Fatal("non-active peer authorized consumption", status)
				}
				state, err := f.call("custodian", "inspect")
				if err != nil {
					t.Fatal(err)
				}
				if !state.ConsumedAt.IsZero() {
					t.Fatal("rejected consumption burned authority")
				}
				if _, err := f.s.MaatReserve(custodyReservation(f.now, "m5"), false); err != nil {
					t.Fatal("rejected consumption fenced ordinary work", err)
				}
			})
		}
	}
}

func TestDesktopCustodyReservationConsumeRaceHasExactlyOneWinner(t *testing.T) {
	for attempt := 0; attempt < 8; attempt++ {
		f := newCustodyFixture(t)
		f.ready(t)
		var wg sync.WaitGroup
		start := make(chan struct{})
		results := make(chan error, 2)
		wg.Add(2)
		go func() { defer wg.Done(); <-start; _, err := f.call("custodian", "consume"); results <- err }()
		go func() {
			defer wg.Done()
			<-start
			_, err := f.s.MaatReserve(custodyReservation(f.now, "cpu@m5"), false)
			results <- err
		}()
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
			t.Fatalf("reserve and consume race has %d winners", wins)
		}
	}
}

func TestDesktopCustodyRefusesScheduledFuturePayload(t *testing.T) {
	f := newCustodyFixture(t)
	f.ready(t)
	r := custodyReservation(f.now.Add(10*time.Second), "cpu@m5")
	if _, err := f.s.MaatReserve(r, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.call("custodian", "consume"); err == nil {
		t.Fatal("future overlapping payload admitted sleep")
	}
}

func TestDesktopCustodyProductionAuthorityRefusesLocalHTTPOrUnenrolledOrigin(t *testing.T) {
	f := newCustodyFixture(t)
	for _, store := range []Store{f.s, f.clients["custodian"]} {
		for _, origin := range []string{"", "http://local", "https://different.invalid"} {
			if _, err := DesktopSleepAuthority(store, origin); err == nil {
				t.Fatal("unqualified production authority accepted")
			}
		}
	}
}

func TestDesktopCustodyExactControllerAdmissionDoesNotHideForeignFloor(t *testing.T) {
	f := newCustodyFixture(t)
	// Replace the fixture only before creation: management cannot modify the
	// existing record. A second fresh nonce/id has a precise controller grant.
	d := f.d
	d.Request.ID = "controller-two"
	d.Admission.ID = d.Request.ID
	d.Admission.RequestID = d.Request.ID
	d.Admission.Nonce = "nonce-two"
	d.CustodianReservationID = "controller"
	d.CustodianWork = "bounded-controller"
	if _, err := f.s.MaatMutate("release", f.budget.AdmissionID, ""); err != nil {
		t.Fatal(err)
	}
	f.budget.AdmissionID = d.CustodianReservationID
	f.budget.Work = d.CustodianWork
	d.ControllerBudget = f.budget
	d.CustodianBudgetSHA256 = sleepDigest(f.budget)
	f.d = d
	r := custodyReservation(f.now, "m5")
	r.ID = "controller"
	r.Holder = d.Custodian.Agent
	r.Work = d.CustodianWork
	r.ExemptPID = f.budget.SupervisorPID
	r.LeaseTTLSec = 240
	r.EstEnd = ""
	if _, err := f.s.MaatReserve(r, false); err != nil {
		t.Fatal(err)
	}
	if err := f.s.CreateDesktopSleep(d); err != nil {
		t.Fatal(err)
	}
	f.ready(t)
	foreign := custodyReservation(f.now, "cpu@m5")
	foreign.ID = "foreign-floor"
	foreign.Cores = 1
	foreign.Share = schedule.ShareFloor
	if _, err := f.s.MaatReserve(foreign, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.call("custodian", "consume"); err == nil {
		t.Fatal("foreign floor hidden by controller admission")
	}
	if _, err := f.s.MaatMutate("release", foreign.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.call("custodian", "consume"); err != nil {
		t.Fatal("exact controller admission blocked own callback", err)
	}
}

func TestDesktopCustodyIndependentReturnProofClearsOnlyVerifiedFence(t *testing.T) {
	f := newCustodyFixture(t)
	f.ready(t)
	s, err := f.call("custodian", "consume")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.call("custodian", "invoke"); err != nil {
		t.Fatal(err)
	}
	f.now = f.d.Request.WakeAt.Add(2 * time.Second)
	for _, p := range []dc.Participant{f.d.Coordinator, f.d.Custodian, f.d.Observer} {
		if err := f.s.UpsertThreads([]ThreadRecord{{ThreadID: p.ThreadID, Agent: p.Agent, Host: p.Host, Status: "active", LastSeenAt: f.now.Format(time.RFC3339Nano), Payload: []byte(`{}`)}}); err != nil {
			t.Fatal(err)
		}
	}
	e := dc.ReturnEvidence{RequestID: f.d.Request.ID, Host: "m5", PlatformUUID: f.d.Admission.PlatformUUID, BootID: f.d.Admission.BootID, SleepRecordID: "native-event", WakeRecordID: "pantheon-sleep-" + f.d.Request.ID, FullWake: true, ObservedAt: f.d.Request.WakeAt, Digest: strings.Repeat("f", 64), ObserverReceiptDigest: strings.Repeat("f", 64)}
	call := dc.CustodyCall{Operation: "returned", ID: f.d.Request.ID, Role: "custodian", Return: &e}
	if _, err := f.clients["custodian"].DesktopSleep(context.Background(), call); err == nil {
		t.Fatal("caller JSON alone cleared fence")
	}
	o := dc.ObserverEvidence{RequestID: f.d.Request.ID, Host: "m5", PlatformUUID: f.d.Admission.PlatformUUID, BootID: f.d.Admission.BootID, LostAt: s.ConsumedAt.Add(time.Second), ReturnedAt: f.now.Add(-time.Second), ContinuitySHA256: strings.Repeat("e", 64)}
	obs := dc.CustodyCall{Operation: "observed-return", ID: f.d.Request.ID, Role: "observer", Observation: &o}
	if _, err := f.clients["custodian"].DesktopSleep(context.Background(), obs); err == nil {
		t.Fatal("custodian impersonated independent observer")
	}
	if _, err := f.clients["observer"].DesktopSleep(context.Background(), obs); err != nil {
		t.Fatal(err)
	}
	e.ObserverReceiptDigest = sleepDigest(o)
	if _, err := f.clients["custodian"].DesktopSleep(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.MaatReserve(custodyReservation(f.now, "m5"), false); err != nil {
		t.Fatal("independently verified return retained scheduler fence", err)
	}
	if _, err := f.call("custodian", "consume"); err == nil {
		t.Fatal("return revived consumed permit")
	}
}

func TestDesktopCustodyPlannedSleepUsesRealBudgetAndGenuinePostWakeHeartbeat(t *testing.T) {
	f := newCustodyFixture(t)
	d := f.d
	d.Request.ID = "long-sleep"
	d.Admission.ID = d.Request.ID
	d.Admission.RequestID = d.Request.ID
	d.Admission.Nonce = "long-nonce"
	d.Request.WakeAt = f.now.Add(180 * time.Second)
	d.Admission.NotBefore = f.now
	d.Admission.NotAfter = f.now.Add(210 * time.Second)
	if err := f.s.CreateDesktopSleep(d); err != nil {
		t.Fatal(err)
	}
	f.d = d
	f.ready(t)
	s, err := f.call("custodian", "consume")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.call("custodian", "invoke"); err != nil {
		t.Fatal(err)
	}
	if !s.ControllerLeaseDeadline.Equal(f.now.Add(240 * time.Second)) {
		t.Fatal("real lease deadline not captured")
	}
	f.now = f.now.Add(181 * time.Second)
	if _, err := f.call("custodian", "status"); err == nil {
		t.Fatal("planned sleep manufactured a heartbeat freshness waiver")
	}
	p := d.Custodian
	if err := f.s.UpsertThreads([]ThreadRecord{{ThreadID: p.ThreadID, Agent: p.Agent, Host: p.Host, Status: "active", LastSeenAt: f.now.Format(time.RFC3339Nano), Payload: []byte(`{}`)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.call("custodian", "status"); err != nil {
		t.Fatal("genuine post-wake registration refresh rejected", err)
	}
	if _, err := f.call("custodian", "consume"); err == nil {
		t.Fatal("post-wake refresh revived consumed authority")
	}
}
