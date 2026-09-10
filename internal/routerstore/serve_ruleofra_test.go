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
		if ver != 19 {
			t.Fatalf("schema version %d, want 19", ver)
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
