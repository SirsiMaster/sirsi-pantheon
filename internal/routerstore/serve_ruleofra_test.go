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
	return ruleHarnessOn(t, mode, logs, newDst(t))
}

func ruleHarnessOn(t *testing.T, mode string, logs *strings.Builder, backend Store) (Store, func(agent, threadID string) *RemoteStore) {
	t.Helper()
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
		if ver != 20 {
			t.Fatalf("schema version %d, want 20", ver)
		}
	}
}

func fmtSprintf(f string, a ...any) string { return strings.TrimSpace(fmt.Sprintf(f, a...)) }

// SSA 2026-09-10 (PR #724 P1): an unregistered caller must not be able to
// replace the binding the gate would authorize it with. Thread lifecycle verbs
// are scoped to the caller's host: a record for another host is refused, an
// existing thread on another host cannot be rewritten, resumed or deleted, and
// the record the store keeps carries the session's host — while the same
// machine's registry sync (every agent on that host) still passes.
func TestThreadAuthorityIsHostScoped(t *testing.T) {
	var logs strings.Builder
	backend, client := ruleHarness(t, "enforce", &logs)
	host, _ := os.Hostname()
	now := time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC)
	seen := now.Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	register(t, backend, "thr-victim", "victim", "other-host", "active", now.Add(-time.Minute))

	attacker := client("attacker", "thr-victim")
	// 1. the gate refuses: the thread belongs to victim@other-host.
	if _, _, err := attacker.SendGuarded(SendReq{From: "attacker", To: "b", Title: "x", Type: "proposal", Instructions: "x"}); !errors.Is(err, ErrUnregistered) {
		t.Fatalf("want ErrUnregistered, got %v", err)
	}
	// 2. rewriting the binding to attacker@<own host> with a newer timestamp is refused, not applied.
	later := now.UTC().Format(time.RFC3339Nano)
	if _, err := attacker.UpsertThreadCAS(ThreadRecord{ThreadID: "thr-victim", Agent: "attacker", Status: "active", LastSeenAt: later, Payload: []byte(`{}`), Host: host}); !errors.Is(err, ErrThreadAuthority) {
		t.Fatalf("cross-host rewrite must be ErrThreadAuthority, got %v", err)
	}
	if err := attacker.UpsertThreads([]ThreadRecord{{ThreadID: "thr-victim", Agent: "attacker", Status: "active", LastSeenAt: later, Payload: []byte(`{}`)}}); !errors.Is(err, ErrThreadAuthority) {
		t.Fatalf("cross-host snapshot rewrite must be ErrThreadAuthority, got %v", err)
	}
	if _, err := attacker.DeleteThreadCAS("thr-victim", "active", seen, ""); !errors.Is(err, ErrThreadAuthority) {
		t.Fatalf("cross-host delete must be ErrThreadAuthority, got %v", err)
	}
	if err := attacker.ResumeThreadCAS(ThreadRecord{ThreadID: "thr-victim", Agent: "attacker", Status: "active", LastSeenAt: later, Payload: []byte(`{}`)}, seen); !errors.Is(err, ErrThreadAuthority) {
		t.Fatalf("cross-host resume must be ErrThreadAuthority, got %v", err)
	}
	// A record that names another host outright is refused too.
	if _, err := attacker.UpsertThreadCAS(ThreadRecord{ThreadID: "thr-new", Agent: "attacker", Status: "active", LastSeenAt: later, Payload: []byte(`{}`), Host: "other-host"}); !errors.Is(err, ErrThreadAuthority) {
		t.Fatalf("claiming another host must be ErrThreadAuthority, got %v", err)
	}
	// 3. still no audience.
	if _, _, err := attacker.SendGuarded(SendReq{From: "attacker", To: "b", Title: "x", Type: "proposal", Instructions: "x"}); !errors.Is(err, ErrUnregistered) {
		t.Fatalf("binding must be intact: want ErrUnregistered, got %v", err)
	}
	if b, berr := backend.ThreadBinding("thr-victim"); berr != nil || b.Agent != "victim" || b.Host != "other-host" {
		t.Fatalf("victim binding was altered: %+v %v", b, berr)
	}

	// Own host: the machine's registry sync carries every agent on it, a
	// legacy row without a host is adoptable, and the stored host is the
	// session's — even when the client left it blank.
	register(t, backend, "thr-legacy", "lane-b", "", "active", now.Add(-time.Minute))
	if err := attacker.UpsertThreads([]ThreadRecord{
		{ThreadID: "thr-legacy", Agent: "lane-b", Status: "active", LastSeenAt: later, Payload: []byte(`{}`)},
		{ThreadID: "thr-own", Agent: "attacker", Status: "active", LastSeenAt: later, Payload: []byte(`{}`)},
	}); err != nil {
		t.Fatalf("own-host registry sync must pass: %v", err)
	}
	for _, id := range []string{"thr-legacy", "thr-own"} {
		if b, berr := backend.ThreadBinding(id); berr != nil || b.Host != host {
			t.Fatalf("%s host = %q (%v), want session host %q", id, b.Host, berr, host)
		}
	}
	// And now the attacker, registered on its own host, has an audience.
	if _, _, err := client("attacker", "thr-own").SendGuarded(SendReq{From: "attacker", To: "b", Title: "x", Type: "proposal", Instructions: "x"}); err != nil {
		t.Fatalf("own registered thread must pass: %v", err)
	}
	// ListThreads carries the host over the wire.
	all, lerr := attacker.ListThreads()
	if lerr != nil {
		t.Fatal(lerr)
	}
	hosts := map[string]string{}
	for _, r := range all {
		hosts[r.ThreadID] = r.Host
	}
	if hosts["thr-victim"] != "other-host" || hosts["thr-own"] != host {
		t.Fatalf("ListThreads must carry host: %v", hosts)
	}
}

// SSA 2026-09-10 (PR #724 P2): the gate never fails open — an unknown mode is
// a construction error and an empty mode is the documented default (log).
func TestRuleOfRaModeIsValidatedAtConstruction(t *testing.T) {
	backend := newDst(t)
	if _, err := Handler(backend, ServerOptions{Token: "t0k", RuleOfRa: "enfroce"}); err == nil || !strings.Contains(err.Error(), "enfroce") {
		t.Fatalf("misspelled mode must refuse to serve, got %v", err)
	}
	var logs strings.Builder
	_, client := ruleHarness(t, "", &logs)
	if _, _, err := client("lane-x", "").SendGuarded(SendReq{From: "lane-x", To: "b", Title: "x", Type: "proposal", Instructions: "x"}); err != nil {
		t.Fatalf("default mode is log, not enforce: %v", err)
	}
	if !strings.Contains(logs.String(), "WOULD REFUSE SendGuarded from lane-x") {
		t.Fatalf("default mode must log; logs=%q", logs.String())
	}
}

// Resume keeps the origin host and fills a legacy blank.
func TestResumeThreadCASFillsHostOnlyWhenBlank(t *testing.T) {
	backend := newDst(t)
	now := time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC)
	register(t, backend, "thr-r", "lane-r", "", "suspended", now)
	seen := now.UTC().Format(time.RFC3339Nano)
	later := now.Add(time.Second).UTC().Format(time.RFC3339Nano)
	if err := backend.ResumeThreadCAS(ThreadRecord{ThreadID: "thr-r", Agent: "lane-r", Status: "active", LastSeenAt: later, Payload: []byte(`{}`), Host: "m5"}, seen); err != nil {
		t.Fatal(err)
	}
	if b, _ := backend.ThreadBinding("thr-r"); b.Host != "m5" || b.Status != "active" {
		t.Fatalf("resume must fill a blank host: %+v", b)
	}
	if err := backend.UpsertThreads([]ThreadRecord{{ThreadID: "thr-r", Agent: "lane-r", Status: "suspended", LastSeenAt: later, Payload: []byte(`{}`), Host: "m5"}}); err != nil {
		t.Fatal(err)
	}
	if err := backend.ResumeThreadCAS(ThreadRecord{ThreadID: "thr-r", Agent: "lane-r", Status: "active", LastSeenAt: now.Add(2 * time.Second).UTC().Format(time.RFC3339Nano), Payload: []byte(`{}`)}, later); err != nil {
		t.Fatal(err)
	}
	if b, _ := backend.ThreadBinding("thr-r"); b.Host != "m5" {
		t.Fatalf("resume with a blank host must keep the origin host: %+v", b)
	}
}

// interposer runs a hook right after every ThreadBinding lookup — the window
// between the server's authority check and its write.
type interposer struct {
	Store
	afterBinding func()
}

func (i *interposer) ThreadBinding(id string) (ThreadBinding, error) {
	b, err := i.Store.ThreadBinding(id)
	if i.afterBinding != nil {
		i.afterBinding()
	}
	return b, err
}

// SSA 2026-09-10 r2 (PR #724 P1): host ownership lives INSIDE the mutation.
// Another host commits its adoption of a blank legacy row (or inserts an
// absent id) between the authority lookup and the write; the caller's write
// must then lose on every path — CAS, bulk sync, resume, delete — and the
// first host keeps the row.
func TestHostAuthoritySurvivesCompetingAdoption(t *testing.T) {
	backend := newDst(t)
	ip := &interposer{Store: backend}
	var logs strings.Builder
	_, client := ruleHarnessOn(t, "enforce", &logs, ip)
	now := time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC)
	t0 := now.Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	t1 := now.UTC().Format(time.RFC3339Nano)
	t2 := now.Add(time.Second).UTC().Format(time.RFC3339Nano)
	rec := func(id, status, seen string) ThreadRecord {
		// payload sorts after the registered `{}` so an equal-fence adoption applies.
		return ThreadRecord{ThreadID: id, Agent: "victim", Status: status, LastSeenAt: seen, Payload: []byte(`{}{}`), Host: "other-host"}
	}
	attacker := client("attacker", "")
	ownedByOther := func(id string) {
		t.Helper()
		b, err := backend.ThreadBinding(id)
		if err != nil || b.Host != "other-host" || b.Agent != "victim" {
			t.Fatalf("%s must stay with the first host: %+v %v", id, b, err)
		}
	}

	// blank legacy rows, one per verb: the other host adopts each after our lookup.
	for _, id := range []string{"thr-cas", "thr-bulk", "thr-del"} {
		register(t, backend, id, "victim", "", "active", now.Add(-time.Minute))
	}
	ip.afterBinding = func() { _, _ = backend.UpsertThreadCAS(rec("thr-cas", "active", t1)) }
	if ok, err := attacker.UpsertThreadCAS(ThreadRecord{ThreadID: "thr-cas", Agent: "attacker", Status: "active", LastSeenAt: t2, Payload: []byte(`{}`)}); err != nil || ok {
		t.Fatalf("stale adoption must lose the CAS: ok=%v err=%v", ok, err)
	}
	ownedByOther("thr-cas")
	ip.afterBinding = func() { _, _ = backend.UpsertThreadCAS(rec("thr-bulk", "active", t1)) }
	if err := attacker.UpsertThreads([]ThreadRecord{{ThreadID: "thr-bulk", Agent: "attacker", Status: "active", LastSeenAt: t2, Payload: []byte(`{}`)}}); err != nil {
		t.Fatal(err)
	}
	ownedByOther("thr-bulk")
	ip.afterBinding = func() { _, _ = backend.UpsertThreadCAS(rec("thr-del", "active", t0)) } // same fence, new owner
	if deleted, err := attacker.DeleteThreadCAS("thr-del", "active", t0, ""); err != nil || deleted {
		t.Fatalf("stale delete must lose: deleted=%v err=%v", deleted, err)
	}
	ownedByOther("thr-del")

	// absent id: the other host inserts it after our lookup.
	ip.afterBinding = func() { _, _ = backend.UpsertThreadCAS(rec("thr-absent", "active", t1)) }
	if ok, err := attacker.UpsertThreadCAS(ThreadRecord{ThreadID: "thr-absent", Agent: "attacker", Status: "active", LastSeenAt: t2, Payload: []byte(`{}`)}); err != nil || ok {
		t.Fatalf("racing insert must lose: ok=%v err=%v", ok, err)
	}
	ownedByOther("thr-absent")

	// suspended blank row: UpsertThreads never touches a suspended row, so the
	// only adoption path is a resume — the other host resumes it after our
	// lookup, our resume loses its fence, and the row is theirs and active.
	register(t, backend, "thr-susp", "victim", "", "suspended", now.Add(-time.Minute))
	ip.afterBinding = func() { _ = backend.ResumeThreadCAS(rec("thr-susp", "active", t1), t0) }
	if err := attacker.ResumeThreadCAS(ThreadRecord{ThreadID: "thr-susp", Agent: "attacker", Status: "active", LastSeenAt: t2, Payload: []byte(`{}`)}, t0); err == nil {
		t.Fatal("resume across a competing adoption must lose its fence")
	}
	ownedByOther("thr-susp")
	if b, _ := backend.ThreadBinding("thr-susp"); b.Status != "active" || b.LastSeenAt != t1 {
		t.Fatalf("row must be the first host's resume: %+v", b)
	}
	// And at the store itself: a suspended row on another host cannot be resumed from here.
	register(t, backend, "thr-theirs", "victim", "other-host", "suspended", now.Add(-time.Minute))
	if err := backend.ResumeThreadCAS(ThreadRecord{ThreadID: "thr-theirs", Agent: "victim", Status: "active", LastSeenAt: t2, Payload: []byte(`{}`), Host: "this-host"}, t0); err == nil {
		t.Fatal("store-level resume must carry the host predicate")
	}
	if b, _ := backend.ThreadBinding("thr-theirs"); b.Status != "suspended" || b.Host != "other-host" {
		t.Fatalf("row must be untouched: %+v", b)
	}
}

// The audience log records every gated call at mutation time (allowed,
// would_refuse, refused) and AudienceSince reports the failures and the live
// sessions that carried no thread — a finished session that was registered at
// the time passes; a session unregistered at the time fails.
func TestAudienceLogRecordsMutationTimeTruth(t *testing.T) {
	var logs strings.Builder
	backend, client := ruleHarness(t, "log", &logs)
	host, _ := os.Hostname()
	now := time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC)
	register(t, backend, "thr-ok", "lane-ok", host, "active", now.Add(-time.Minute))
	if _, _, err := client("lane-ok", "thr-ok").SendGuarded(SendReq{From: "lane-ok", To: "b", Title: "a", Type: "proposal", Instructions: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := client("lane-bad", "").SendGuarded(SendReq{From: "lane-bad", To: "b", Title: "b", Type: "proposal", Instructions: "x"}); err != nil {
		t.Fatal(err) // log mode allows
	}
	// The registered thread finishes afterwards; its earlier mutation still passes the audit.
	register(t, backend, "thr-ok", "lane-ok", host, "closed", now)
	rep, err := backend.AudienceSince("2026-09-10T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Gated != 2 || rep.Allowed != 1 || len(rep.Failures) != 1 || rep.Failures[0].Agent != "lane-bad" || rep.Failures[0].Verdict != "would_refuse" {
		t.Fatalf("report = %+v", rep)
	}
	if len(rep.Unbound) != 1 || !strings.HasPrefix(rep.Unbound[0], "lane-bad@") {
		t.Fatalf("live coverage gap must name the unbound session: %v", rep.Unbound)
	}
	// Reads are not gated and not logged.
	if _, err := client("lane-bad", "").Inbox("lane-bad"); err != nil {
		t.Fatal(err)
	}
	rep2, _ := backend.AudienceSince("2026-09-10T00:00:00Z")
	if rep2.Gated != 2 {
		t.Fatalf("reads must not be logged: gated=%d", rep2.Gated)
	}
	// Over the wire, the report is readable by an unregistered session.
	if wr, err := client("lane-bad", "").AudienceSince("2026-09-10T00:00:00Z"); err != nil || wr.Gated != 2 {
		t.Fatalf("AudienceSince over the wire: %+v %v", wr, err)
	}
}

// auditFault makes RecordAudience fail on demand — SSA's fault injection.
type auditFault struct {
	Store
	fail bool
}

func (a *auditFault) RecordAudience(e AudienceEntry) error {
	if a.fail {
		return errors.New("disk full")
	}
	return a.Store.RecordAudience(e)
}

// SSA 2026-09-10 (PR #726): (1) a failed audit write refuses the mutation
// instead of letting an unrecorded mutation through; (2) live coverage comes
// from the sessions table, so a threadless session that only reads is named;
// (3) an empty window reports Recorded=false and carries the gate mode.
func TestAudienceIsCompleteOrRefuses(t *testing.T) {
	backend := newDst(t)
	af := &auditFault{Store: backend}
	var logs strings.Builder
	_, client := ruleHarnessOn(t, "log", &logs, af)

	// (2) a live, threadless session that only calls Inbox appears in coverage…
	reader := client("lane-reader", "")
	if _, err := reader.Inbox("lane-reader"); err != nil {
		t.Fatal(err)
	}
	rep, err := reader.AudienceSince("2026-09-10T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Recorded || rep.Gated != 0 || rep.Mode != "log" {
		t.Fatalf("empty history must say so and carry the mode: %+v", rep)
	}
	if len(rep.Unbound) != 1 || !strings.HasPrefix(rep.Unbound[0], "lane-reader@") {
		t.Fatalf("a live threadless session must be in the coverage table even with no mutation: %v", rep.Unbound)
	}
	// …and a revoked one drops out.
	if err := backend.RevokeSession(strings.Fields(rep.Unbound[0])[1]); err != nil {
		t.Fatal(err)
	}
	if rep2, _ := client("lane-x", "thr-none").AudienceSince("2026-09-10T00:00:00Z"); len(rep2.Unbound) != 0 {
		t.Fatalf("a revoked session leaves coverage, and a session minted with a thread id is not threadless: %v", rep2.Unbound)
	}

	// (1) audit write fails → the mutation is refused and nothing is committed.
	af.fail = true
	writer := client("lane-writer", "")
	if _, _, err := writer.SendGuarded(SendReq{From: "lane-writer", To: "b", Title: "lost", Type: "proposal", Instructions: "x"}); err == nil || !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("a mutation the audit cannot record must be refused with ErrServiceUnavailable, got %v", err)
	}
	af.fail = false
	if items, _ := backend.Inbox("b"); len(items) != 0 {
		t.Fatalf("refused mutation must not commit: %d items", len(items))
	}
	if rep3, _ := writer.AudienceSince("2026-09-10T00:00:00Z"); rep3.Recorded {
		t.Fatalf("nothing was recorded during the fault: %+v", rep3)
	}
	if !strings.Contains(logs.String(), "audience log write failed, refusing SendGuarded") {
		t.Fatalf("the refusal must be logged: %q", logs.String())
	}
}

// SSA 2026-09-10 (PR #726 P2): the log's timestamp is fixed-width, so a
// fractional-second `since` excludes an older whole-second row.
func TestAudienceSinceIsChronologicalAtFractionalSeconds(t *testing.T) {
	backend := newDst(t)
	if err := backend.RecordAudience(AudienceEntry{TS: "2026-09-10T15:00:00Z", Method: "SendGuarded", SessionID: "s", Agent: "a", Host: "h", Verdict: "would_refuse"}); err != nil {
		t.Fatal(err)
	}
	if rep, _ := backend.AudienceSince("2026-09-10T15:00:00.5Z"); rep.Gated != 0 {
		t.Fatalf("a row at 15:00:00Z must not match since=15:00:00.5Z: %+v", rep)
	}
	if rep, _ := backend.AudienceSince("2026-09-10T14:59:59.999999999Z"); rep.Gated != 1 || rep.Failures[0].TS != "2026-09-10T15:00:00.000000000Z" {
		t.Fatalf("stored ts must be fixed width and included: %+v", rep)
	}
}

// SSA 2026-09-10 (PR #726 r2): a malformed `since` is a normal error over the
// wire, never a panic; and the live-coverage threshold excludes a session at
// the whole second before a fractional `since`.
func TestAudienceSinceValidatesAndCeilsFraction(t *testing.T) {
	backend := newDst(t)
	var logs strings.Builder
	_, client := ruleHarnessOn(t, "log", &logs, backend)

	// invalid since — store and wire both return an error, no panic.
	if _, err := backend.AudienceSince("bad"); err == nil {
		t.Fatal("store: invalid since must error")
	}
	if _, err := backend.AudienceSince(""); err == nil {
		t.Fatal("store: empty since must error")
	}
	if _, err := client("lane-a", "").AudienceSince("nonsense"); err == nil {
		t.Fatal("wire: invalid since must return an error, not crash the handler")
	}

	// A directly-inserted session at exactly 15:00:00Z (no minted real-clock
	// rows in the way): excluded for a fractional since past that second,
	// included at the whole second.
	fresh := newDst(t)
	if _, err := fresh.db.Exec(`INSERT INTO sessions(session_id,secret,host,agent,runtime_hash,thread_id,created,last_seen,revoked) VALUES('s1','x','h','lane-live','rh','','2026-09-10T15:00:00Z','2026-09-10T15:00:00Z','')`); err != nil {
		t.Fatal(err)
	}
	if rep, err := fresh.AudienceSince("2026-09-10T15:00:00.5Z"); err != nil || len(rep.Unbound) != 0 {
		t.Fatalf("fractional since must exclude the previous whole second: %+v %v", rep.Unbound, err)
	}
	if rep, _ := fresh.AudienceSince("2026-09-10T15:00:00Z"); len(rep.Unbound) != 1 {
		t.Fatalf("whole-second since must include the session: %v", rep.Unbound)
	}
	if rep, _ := fresh.AudienceSince("2026-09-10T14:59:59.5Z"); len(rep.Unbound) != 1 {
		t.Fatalf("since just before must include the session: %v", rep.Unbound)
	}
}

// A registered thread mutates in any live working state — active, idle, or
// blocked ("idle ≠ dead") — but resting/terminal states are refused. The
// heartbeat window is the liveness gate, orthogonal to status.
func TestRuleOfRaAcceptsIdleAndBlockedRefusesRestingStates(t *testing.T) {
	var logs strings.Builder
	backend, client := ruleHarness(t, "enforce", &logs)
	host, _ := os.Hostname()
	now := time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC)
	fresh := now.Add(-time.Minute)

	send := func(agent, thread string) error {
		_, _, err := client(agent, thread).SendGuarded(SendReq{From: agent, To: "b", Title: "x", Type: "proposal", Instructions: "y"})
		return err
	}

	// live working states pass.
	for _, st := range []string{"active", "idle", "blocked"} {
		id := "thr-" + st
		register(t, backend, id, "lane-"+st, host, st, fresh)
		if err := send("lane-"+st, id); err != nil {
			t.Fatalf("a %s thread must be allowed to mutate: %v", st, err)
		}
	}

	// resting / terminal states are refused.
	for _, st := range []string{"suspended", "closed", "reaped", "stale-heartbeat"} {
		id := "thr-rest-" + st
		register(t, backend, id, "lane-rest-"+st, host, st, fresh)
		if err := send("lane-rest-"+st, id); !errors.Is(err, ErrUnregistered) {
			t.Fatalf("a %s thread must be refused, got %v", st, err)
		}
	}

	// a fresh heartbeat does not rescue a stale one: an idle thread with an old
	// heartbeat is still refused by the freshness gate.
	register(t, backend, "thr-idle-stale", "lane-idle-stale", host, "idle", now.Add(-time.Hour))
	if err := send("lane-idle-stale", "thr-idle-stale"); !errors.Is(err, ErrUnregistered) {
		t.Fatal("an idle thread with a stale heartbeat must still be refused")
	}
}
