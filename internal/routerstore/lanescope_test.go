package routerstore

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
