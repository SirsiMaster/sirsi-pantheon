package routerstore

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// scopedFixture serves a real backend and returns a client factory for a token confined to scope.
type scopedFixture struct {
	backend *SQLiteStore
	url     string
	token   string
	host    string
}

func newScopedFixture(t *testing.T, scope []string) *scopedFixture {
	t.Helper()
	backend := openBackendStore(t, filepath.Join(t.TempDir(), "router.db"))
	t.Cleanup(func() { _ = backend.Close() })
	backend.notifyDir = t.TempDir()
	h, err := Handler(backend, ServerOptions{Token: "t0k", MaxWait: 3 * time.Second})
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	tok, rec, err := backend.MintScopedHostToken("cloud-host", "cloud sandbox", scope)
	if err != nil {
		t.Fatalf("MintScopedHostToken: %v", err)
	}
	if len(rec.Agents) != len(scope) {
		t.Fatalf("minted record carries scope %v, want %v", rec.Agents, scope)
	}
	return &scopedFixture{backend: backend, url: srv.URL, token: tok, host: "cloud-host"}
}

// client returns a RemoteStore authenticating with the scoped token as the given agent.
func (f *scopedFixture) client(agent string) *RemoteStore {
	rs := NewRemoteStore(f.url, f.token)
	rs.sessionDir = ""
	rs.host = f.host
	rs.agent = agent
	rs.threadID = ""
	return rs
}

func wantLaneScope(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: allowed, want a lane-scope refusal", what)
	}
	if !strings.Contains(err.Error(), "lane scope") {
		t.Fatalf("%s: refused for the wrong reason: %v", what, err)
	}
}

// A scoped token cannot mint a session for an agent outside its scope; it can for one inside it.
func TestScopedTokenMintsSessionsOnlyForItsAgents(t *testing.T) {
	f := newScopedFixture(t, []string{"cloud-a", "cloud-b"})
	if _, err := f.client("cloud-a").Inbox("cloud-a"); err != nil {
		t.Fatalf("in-scope agent refused: %v", err)
	}
	_, err := f.client("ra").Inbox("ra")
	wantLaneScope(t, "session for ra", err)
	_, err = f.client("owner").Inbox("owner")
	wantLaneScope(t, "session for owner", err)
}

// A scoped session cannot send as another agent (impersonation) but can send to anyone.
func TestScopedSessionCannotSendAsAnotherAgent(t *testing.T) {
	f := newScopedFixture(t, []string{"cloud-a"})
	rs := f.client("cloud-a")
	if _, err := rs.Send("cloud-a", "mac-lane", "hello from the cloud", "proposal", "body"); err != nil {
		t.Fatalf("send as itself to a Mac lane refused: %v", err)
	}
	_, err := rs.Send("owner", "mac-lane", "forged", "decision", "body")
	wantLaneScope(t, "send as owner", err)
	_, err = rs.Send("ra", "mac-lane", "forged", "decision", "body")
	wantLaneScope(t, "send as ra", err)
	_, _, err = rs.SendGuarded(SendReq{From: "ra", To: "mac-lane", Title: "forged", Type: "decision", Instructions: "b"})
	wantLaneScope(t, "SendGuarded as ra", err)
	if _, _, err = rs.SendGuarded(SendReq{From: "cloud-a", To: "mac-lane", Title: "ok", Type: "proposal", Instructions: "b"}); err != nil {
		t.Fatalf("SendGuarded as itself refused: %v", err)
	}
}

// The whole-ledger reads are closed to a scoped token, and so is another lane's inbox.
func TestScopedSessionCannotReadTheWholeLedgerOrOthersInboxes(t *testing.T) {
	f := newScopedFixture(t, []string{"cloud-a"})
	if _, err := f.backend.Send("mac-lane", "ra", "private to ra", "decision", "secret"); err != nil {
		t.Fatal(err)
	}
	rs := f.client("cloud-a")
	ctx := context.Background()
	_, err := rs.ListAll(ctx)
	wantLaneScope(t, "ListAll", err)
	_, err = rs.ListPage(ctx, "", 10)
	wantLaneScope(t, "ListPage", err)
	_, err = rs.ListActive(ctx)
	wantLaneScope(t, "ListActive", err)
	_, err = rs.ListSince(ctx, "2000-01-01T00:00:00Z")
	wantLaneScope(t, "ListSince", err)
	_, err = rs.CountClosed(ctx)
	wantLaneScope(t, "CountClosed", err)
	_, err = rs.Inbox("ra")
	wantLaneScope(t, "Inbox(ra)", err)
	if _, err = rs.Inbox("cloud-a"); err != nil {
		t.Fatalf("own inbox refused: %v", err)
	}
}

// A scoped session sees and works only items it is a party to; reading/closing a stranger's item is refused
// with the same error as a missing item, so ids cannot be probed.
func TestScopedSessionSeesOnlyItsOwnItems(t *testing.T) {
	f := newScopedFixture(t, []string{"cloud-a"})
	strangers, _ := f.backend.Send("mac-lane", "ra", "ra private", "decision", "secret")
	toMe, _ := f.backend.Send("mac-lane", "cloud-a", "for the cloud lane", "proposal", "body")
	fromMe, _ := f.backend.Send("cloud-a", "mac-lane", "i sent this", "proposal", "body")
	rs := f.client("cloud-a")

	_, err := rs.Get(strangers)
	wantLaneScope(t, "Get(stranger item)", err)
	_, err = rs.Get("20000101-000000-no-such-item")
	wantLaneScope(t, "Get(missing item)", err)
	if _, err = rs.Get(toMe); err != nil {
		t.Fatalf("Get(item to me) refused: %v", err)
	}
	if _, err = rs.Get(fromMe); err != nil {
		t.Fatalf("Get(item from me) refused: %v", err)
	}
	wantLaneScope(t, "CloseItem(stranger)", rs.CloseItem(strangers, "x"))
	wantLaneScope(t, "CloseItem(item I SENT, recipient is another lane)", rs.CloseItem(fromMe, "x"))
	if err = rs.CloseItem(toMe, "done"); err != nil {
		t.Fatalf("CloseItem(item to me) refused: %v", err)
	}
}

// A scoped session cannot register a thread for another agent, and everything outside the lane API is closed.
func TestScopedSessionCannotRegisterForeignThreadsOrCallAdminMethods(t *testing.T) {
	f := newScopedFixture(t, []string{"cloud-a"})
	rs := f.client("cloud-a")
	wantLaneScope(t, "UpsertThreads for ra", rs.UpsertThreads([]ThreadRecord{{ThreadID: "thr-x", Agent: "ra", Status: "active", Host: f.host}}))
	_, err := rs.UpsertThreadCAS(ThreadRecord{ThreadID: "thr-y", Agent: "owner", Status: "active", Host: f.host})
	wantLaneScope(t, "UpsertThreadCAS for owner", err)
	wantLaneScope(t, "ForceOwner", rs.ForceOwner("x", "close", "r"))
	_, err = rs.Counters()
	wantLaneScope(t, "Counters (admin read)", err)
	_, err = rs.ListIdentifiers("ns")
	wantLaneScope(t, "ListIdentifiers", err)
}

// ListThreads is filtered to the token's own agents: a scoped session must not see another lane's
// continuation payload (SSA PR #1079 review, P1).
func TestScopedListThreadsHidesForeignPayloads(t *testing.T) {
	f := newScopedFixture(t, []string{"cloud-a"})
	if err := f.backend.UpsertThreads([]ThreadRecord{
		{ThreadID: "thr-ra", Agent: "ra", Status: "active", Payload: []byte(`{"ResumePrompt":"ra's secret"}`)},
		{ThreadID: "thr-cloud-a", Agent: "cloud-a", Status: "active", Payload: []byte(`{"ResumePrompt":"mine"}`)},
	}); err != nil {
		t.Fatal(err)
	}
	list, err := f.client("cloud-a").ListThreads()
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(list) != 1 || list[0].ThreadID != "thr-cloud-a" {
		t.Fatalf("ListThreads = %+v, want only thr-cloud-a", list)
	}
}

// A scoped token cannot take over an existing thread row owned by another agent by naming its id and
// claiming it for an in-scope agent, even when the row's host is blank (legacy) or the token's own host
// (SSA PR #1079 review, P1).
func TestScopedThreadWriteCannotHijackExistingForeignRow(t *testing.T) {
	f := newScopedFixture(t, []string{"cloud-a"})
	if err := f.backend.UpsertThreads([]ThreadRecord{{ThreadID: "thr-blank", Agent: "ra", Status: "active", Payload: []byte("{}")}}); err != nil {
		t.Fatal(err)
	}
	if err := f.backend.UpsertThreads([]ThreadRecord{{ThreadID: "thr-samehost", Agent: "ra", Status: "active", Host: f.host, Payload: []byte("{}")}}); err != nil {
		t.Fatal(err)
	}
	rs := f.client("cloud-a")
	wantLaneScope(t, "UpsertThreads over blank-host ra row", rs.UpsertThreads([]ThreadRecord{{ThreadID: "thr-blank", Agent: "cloud-a", Status: "active", Host: f.host}}))
	wantLaneScope(t, "UpsertThreads over same-host ra row", rs.UpsertThreads([]ThreadRecord{{ThreadID: "thr-samehost", Agent: "cloud-a", Status: "active", Host: f.host}}))
	_, err := rs.UpsertThreadCAS(ThreadRecord{ThreadID: "thr-blank", Agent: "cloud-a", Status: "active", Host: f.host})
	wantLaneScope(t, "UpsertThreadCAS over blank-host ra row", err)
	// Registering a brand-new thread id for an in-scope agent is still allowed.
	if err := rs.UpsertThreads([]ThreadRecord{{ThreadID: "thr-new", Agent: "cloud-a", Status: "active", Host: f.host, Payload: []byte("{}")}}); err != nil {
		t.Fatalf("UpsertThreads for a new thread refused: %v", err)
	}
}

// The existing-owner scope check must be atomic with the write: a pre-check (read the owner, then call
// the plain write) races a legitimate concurrent write between the two. This fires many out-of-scope
// hijack attempts concurrently against many legitimate same-agent heartbeats on the SAME row and asserts
// the out-of-scope agent never wins a single one, regardless of interleaving (SSA PR #1079 review round 2).
func TestScopedThreadWriteCannotHijackExistingForeignRowConcurrently(t *testing.T) {
	f := newScopedFixture(t, []string{"cloud-a"})
	if err := f.backend.UpsertThreads([]ThreadRecord{{ThreadID: "thr-race", Agent: "ra", Status: "active", Host: f.host, LastSeenAt: "2026-01-01T00:00:00Z", Payload: []byte("{}")}}); err != nil {
		t.Fatal(err)
	}
	rs := f.client("cloud-a")
	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(2)
		seq := i
		go func() {
			defer wg.Done()
			_ = f.backend.UpsertThreads([]ThreadRecord{{ThreadID: "thr-race", Agent: "ra", Status: "active", Host: f.host,
				LastSeenAt: time.Date(2026, 1, 1, 0, 0, seq, 0, time.UTC).Format(time.RFC3339), Payload: []byte("{}")}})
		}()
		go func() {
			defer wg.Done()
			err := rs.UpsertThreads([]ThreadRecord{{ThreadID: "thr-race", Agent: "cloud-a", Status: "active", Host: f.host,
				LastSeenAt: time.Date(2027, 1, 1, 0, 0, seq, 0, time.UTC).Format(time.RFC3339), Payload: []byte("{}")}})
			if err == nil {
				t.Errorf("hijack attempt %d: UpsertThreads over foreign row was not refused", seq)
			} else if !strings.Contains(err.Error(), "lane scope") {
				t.Errorf("hijack attempt %d: got %v, want a lane-scope refusal", seq, err)
			}
		}()
	}
	wg.Wait()
	b, err := f.backend.ThreadBinding("thr-race")
	if err != nil {
		t.Fatalf("ThreadBinding: %v", err)
	}
	if b.Agent != "ra" {
		t.Fatalf("thr-race agent = %q after the race, want ra (an out-of-scope hijack got through)", b.Agent)
	}
}

// An unscoped host token keeps the pre-v25 behavior: no lane API applies, whole-ledger reads work.
func TestUnscopedHostTokenIsUnaffectedByLaneScope(t *testing.T) {
	f := newScopedFixture(t, []string{"cloud-a"})
	tok, rec, err := f.backend.MintHostToken("mac-host", "a Mac")
	if err != nil || len(rec.Agents) != 0 {
		t.Fatalf("MintHostToken: %v, agents=%v", err, rec.Agents)
	}
	rs := NewRemoteStore(f.url, tok)
	rs.sessionDir, rs.host, rs.agent = "", "mac-host", "ra"
	if _, err := f.backend.Send("x", "y", "t", "proposal", "b"); err != nil {
		t.Fatal(err)
	}
	if all, err := rs.ListAll(context.Background()); err != nil || len(all) == 0 {
		t.Fatalf("unscoped ListAll = %d, %v; want the ledger", len(all), err)
	}
	if _, err := rs.Send("ra", "cloud-a", "ok", "proposal", "b"); err != nil {
		t.Fatalf("unscoped send refused: %v", err)
	}
}

// Minting requires at least one agent (an empty list would silently mint an unscoped token), and the scope
// is what LookupHostToken and ListHostTokens report.
func TestMintScopedHostTokenRequiresAgentsAndPersistsScope(t *testing.T) {
	s := newTestStore(t)
	if _, _, err := s.MintScopedHostToken("h", "l", nil); err == nil {
		t.Fatal("scoped mint with no agents succeeded (it would be an unscoped token)")
	}
	if _, _, err := s.MintScopedHostToken("h", "l", []string{" ", ""}); err == nil {
		t.Fatal("scoped mint with only blank agents succeeded")
	}
	tok, rec, err := s.MintScopedHostToken("h", "l", []string{"b", "a", "a"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(rec.Agents, ",") != "a,b" {
		t.Fatalf("scope %v, want sorted de-duplicated [a b]", rec.Agents)
	}
	got, err := s.LookupHostToken(tok)
	if err != nil || strings.Join(got.Agents, ",") != "a,b" {
		t.Fatalf("LookupHostToken scope = %v, %v", got.Agents, err)
	}
	list, err := s.ListHostTokens()
	if err != nil || len(list) != 1 || strings.Join(list[0].Agents, ",") != "a,b" {
		t.Fatalf("ListHostTokens = %+v, %v", list, err)
	}
}

// A refused attempt leaves a "refused" audit row (shown by `router audience`) naming the method, the attempting
// agent and the host, so the owner can see a sandbox probing outside its lane.
func TestLaneScopeRefusalIsAudited(t *testing.T) {
	f := newScopedFixture(t, []string{"cloud-a"})
	rs := f.client("cloud-a")
	_, err := rs.Send("owner", "mac-lane", "forged", "decision", "body")
	wantLaneScope(t, "send as owner", err)
	rep, err := f.backend.AudienceSince("2000-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range rep.Failures {
		if e.Verdict == "refused" && e.Method == "Send" && e.Host == f.host && strings.Contains(e.Reason, `Send as "owner"`) {
			return
		}
	}
	t.Fatalf("no refused audit row for the forged send; failures: %+v", rep.Failures)
}

// A scoped session cannot take over an EXISTING thread that belongs to another agent by upserting a record that
// reuses its id with its own agent name (the record's agent is in scope, the thread it overwrites is not).
func TestScopedSessionCannotRewriteAnotherAgentsThread(t *testing.T) {
	f := newScopedFixture(t, []string{"cloud-a"})
	now := time.Now().UTC().Format(time.RFC3339)
	if err := f.backend.UpsertThreads([]ThreadRecord{{ThreadID: "thr-ra", Agent: "ra", Status: "active", LastSeenAt: now, Host: f.host, Payload: []byte("{}")}}); err != nil {
		t.Fatal(err)
	}
	rs := f.client("cloud-a")
	wantLaneScope(t, "UpsertThreads over ra's thread", rs.UpsertThreads([]ThreadRecord{{ThreadID: "thr-ra", Agent: "cloud-a", Status: "active", LastSeenAt: now, Host: f.host, Payload: []byte("{}")}}))
	_, err := rs.UpsertThreadCAS(ThreadRecord{ThreadID: "thr-ra", Agent: "cloud-a", Status: "active", LastSeenAt: now, Host: f.host, Payload: []byte("{}")})
	wantLaneScope(t, "UpsertThreadCAS over ra's thread", err)
	wantLaneScope(t, "ResumeThreadCAS over ra's thread", rs.ResumeThreadCAS(ThreadRecord{ThreadID: "thr-ra", Agent: "cloud-a", Status: "active", LastSeenAt: now, Host: f.host, Payload: []byte("{}")}, now))
	b, err := f.backend.ThreadBinding("thr-ra")
	if err != nil || b.Agent != "ra" {
		t.Fatalf("ra's thread binding changed to %+v (err %v)", b, err)
	}
	// Its own thread, and a brand-new thread id, still register.
	if err := rs.UpsertThreads([]ThreadRecord{{ThreadID: "thr-new", Agent: "cloud-a", Status: "active", LastSeenAt: now, Host: f.host, Payload: []byte("{}")}}); err != nil {
		t.Fatalf("registering a new thread for itself refused: %v", err)
	}
}
