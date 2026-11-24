package routerstore

import (
	"context"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// answerHTTPFixture drives AnswerHandoff through the real HTTP surface with two
// enrolled participants of one lane (gates 3, 4 and 6).
type answerHTTPFixture struct {
	s      *SQLiteStore
	now    time.Time
	lane   string
	client map[string]*RemoteStore
	token  map[string]string // role -> token id
	mid    map[string]string
}

func newAnswerHTTPFixture(t *testing.T) *answerHTTPFixture {
	t.Helper()
	s := openBackendStore(t, filepath.Join(t.TempDir(), "answer-http.db"))
	t.Cleanup(func() { s.Close() })
	f := &answerHTTPFixture{s: s, now: time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC), lane: "mercury", client: map[string]*RemoteStore{}, token: map[string]string{}, mid: map[string]string{}}
	s.now = func() time.Time { return f.now }
	h, err := Handler(s, ServerOptions{Token: "bootstrap", RuleOfRa: "enforce", now: func() time.Time { return f.now }})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	sha := strings.Repeat("a", 64)
	for i, role := range []string{"lead", "worker"} {
		host, mid := "host-"+role, "machine-"+role
		tok, rec, err := s.MintScopedHostToken(host, role, []string{f.lane})
		if err != nil {
			t.Fatal(err)
		}
		if err = s.AdoptTokenMachineID(host, mid); err != nil {
			t.Fatal(err)
		}
		thread := role + "-thread"
		payload := fmt.Sprintf(`{"pid":%d,"start_time":"start-%d","machine_id":%q}`, 41+i, i, mid)
		if err = s.UpsertThreads([]ThreadRecord{{ThreadID: thread, Agent: f.lane, Host: host, Status: "active", LastSeenAt: f.now.Format(time.RFC3339Nano), Payload: []byte(payload)}}); err != nil {
			t.Fatal(err)
		}
		rs := NewRemoteStore(srv.URL, tok)
		rs.host, rs.agent, rs.runtime, rs.threadID, rs.sessionDir = host, f.lane, sha, thread, ""
		rs.now = func() time.Time { return f.now }
		if _, err = rs.ensureSession(context.Background()); err != nil {
			t.Fatal(err)
		}
		f.client[role], f.token[role], f.mid[role] = rs, rec.ID, mid
	}
	return f
}

func (f *answerHTTPFixture) owner(t *testing.T, role string) AnswerOwner {
	t.Helper()
	o, err := f.s.AnswerOwnerForSession(f.client[role].session.ID, f.token[role], f.mid[role])
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func (f *answerHTTPFixture) call(role string, c AnswerHandoffCall) (AnswerAssignment, error) {
	return f.client[role].AnswerHandoff(context.Background(), f.lane, c)
}

func (f *answerHTTPFixture) seed(t *testing.T) AnswerOwner {
	t.Helper()
	lead := f.owner(t, "lead")
	if err := f.s.CreateAnswerAssignment(f.lane, lead, "canonical/seed", 120); err != nil {
		t.Fatal(err)
	}
	return lead
}

// Gate 4: a lane nobody seeded is untouched legacy traffic; the handoff surface
// refuses it and creates nothing.
func TestAnswerHandoffHTTPUnseededLaneIsLegacyAndUnchanged(t *testing.T) {
	f := newAnswerHTTPFixture(t)
	for _, op := range []string{"read", "generation", "request", "revoke"} {
		if _, err := f.call("lead", AnswerHandoffCall{Op: op}); err == nil {
			t.Fatal("unseeded lane answered", op)
		}
	}
	var n int
	if err := f.s.db.QueryRow(`SELECT COUNT(*) FROM state WHERE key LIKE 'answer-handoff%'`).Scan(&n); err != nil || n != 0 {
		t.Fatal("refused calls left handoff state behind", n, err)
	}
}

// Gate 4: the bootstrap token is not a participant.
func TestAnswerHandoffHTTPBootstrapTokenRefused(t *testing.T) {
	f := newAnswerHTTPFixture(t)
	f.seed(t)
	h, _ := Handler(f.s, ServerOptions{Token: "bootstrap", RuleOfRa: "enforce", now: func() time.Time { return f.now }})
	srv := httptest.NewServer(h)
	defer srv.Close()
	rs := NewRemoteStore(srv.URL, "bootstrap")
	rs.host, rs.agent, rs.runtime, rs.threadID, rs.sessionDir = "host-lead", f.lane, strings.Repeat("a", 64), "lead-thread", ""
	rs.now = func() time.Time { return f.now }
	if _, err := rs.AnswerHandoff(context.Background(), f.lane, AnswerHandoffCall{Op: "read"}); err == nil {
		t.Fatal("bootstrap token answered the handoff surface")
	}
}

// Gate 6: the generation is server-minted. A client that knows every
// self-reported field (machine, session id, pid, start) cannot reproduce it.
func TestAnswerGenerationIsServerMintedOverHTTP(t *testing.T) {
	f := newAnswerHTTPFixture(t)
	lead := f.seed(t)
	got, err := f.call("worker", AnswerHandoffCall{Op: "generation"})
	if err != nil || got.Self == nil {
		t.Fatal("future target cannot learn its own identity", err)
	}
	worker := *got.Self
	if worker != f.owner(t, "worker") {
		t.Fatal("response differs from the stored derivation")
	}
	if len(got.History) != 0 || got.Owner != (AnswerOwner{}) {
		t.Fatal("generation op leaked the assignment")
	}
	forged := answerGeneration("", worker.MachineID, f.client["worker"].session.ID, 42, "start-1")
	if forged == worker.Generation || strings.HasPrefix(worker.Generation, forged) {
		t.Fatal("generation is computable from client-known fields")
	}
	// A request naming the forged target is accepted as data, but the real
	// worker is not that target, so it cannot consent.
	req := AnswerHandoffCall{Op: "request", RequestID: "r1", ExpectedOwner: lead, Target: AnswerOwner{ThreadID: worker.ThreadID, Host: worker.Host, MachineID: worker.MachineID, Generation: forged}, SourceRef: "canonical/request", TTLSeconds: 60}
	if _, err = f.call("lead", req); err != nil {
		t.Fatal(err)
	}
	if _, err = f.call("worker", AnswerHandoffCall{Op: "accept", RequestID: "r1", ExpectedRevision: 1, ExpectedOwner: lead}); err == nil {
		t.Fatal("worker consented to a request naming a forged generation")
	}
	// An unrelated same-lane session still cannot read the assignment.
	if _, err = f.call("worker", AnswerHandoffCall{Op: "read"}); err == nil {
		t.Fatal("non-owner non-target read the assignment")
	}
}

// Gate 4: live transfer race over HTTP. Two concurrent requests on one
// revision: exactly one wins; the lane stays single-owner.
func TestAnswerHandoffHTTPConcurrentRequestsOneWinner(t *testing.T) {
	f := newAnswerHTTPFixture(t)
	lead := f.seed(t)
	worker := f.owner(t, "worker")
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := AnswerHandoffCall{Op: "request", RequestID: fmt.Sprintf("race-%d", i), ExpectedOwner: lead, Target: worker, SourceRef: "canonical/request", TTLSeconds: 60}
			if _, err := f.call("lead", c); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if wins != 1 {
		t.Fatal("concurrent requests won", wins)
	}
	// The target completes consent; activation stays refused over the wire.
	cur, err := f.call("lead", AnswerHandoffCall{Op: "read"})
	if err != nil || cur.Pending == nil {
		t.Fatal(err)
	}
	if _, err = f.call("worker", AnswerHandoffCall{Op: "accept", RequestID: cur.Pending.RequestID, ExpectedRevision: cur.Revision, ExpectedOwner: lead}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.call("worker", AnswerHandoffCall{Op: "activate", RequestID: cur.Pending.RequestID}); err == nil {
		t.Fatal("activation reachable before native gates qualify")
	}
}

// Gate 3: revocation is rechecked inside the state transaction. The actor value
// is what the HTTP layer would have built before the revocation landed.
func TestAnswerHandoffRevocationRecheckedInTransaction(t *testing.T) {
	for _, mode := range []string{"session", "token", "machine-id"} {
		t.Run(mode, func(t *testing.T) {
			f := newAnswerHTTPFixture(t)
			f.seed(t)
			sess := f.client["lead"].session
			actor := answerActor{Session: Session{ID: sess.ID, Host: sess.Host, Agent: f.lane, ThreadID: "lead-thread"}, MachineID: f.mid["lead"], TokenID: f.token["lead"]}
			ctx := context.WithValue(context.Background(), answerActorKey{}, actor)
			if _, err := f.s.AnswerHandoff(ctx, f.lane, AnswerHandoffCall{Op: "read"}); err != nil {
				t.Fatal("control: live actor refused", err)
			}
			switch mode {
			case "session":
				if err := f.s.RevokeSession(sess.ID); err != nil {
					t.Fatal(err)
				}
			case "token":
				// Token revoked directly so the session stays live: only the in-tx token check can refuse.
				if _, err := f.s.exec(`UPDATE host_tokens SET revoked=? WHERE token_id=?`, f.now.Format(time.RFC3339), f.token["lead"]); err != nil {
					t.Fatal(err)
				}
			case "machine-id":
				actor.MachineID = "machine-other"
				ctx = context.WithValue(context.Background(), answerActorKey{}, actor)
			}
			if _, err := f.s.AnswerHandoff(ctx, f.lane, AnswerHandoffCall{Op: "read"}); err == nil {
				t.Fatal("revoked or mismatched actor answered")
			}
			if _, err := f.s.AnswerHandoff(ctx, f.lane, AnswerHandoffCall{Op: "generation"}); err == nil {
				t.Fatal("revoked or mismatched actor learned its generation")
			}
		})
	}
}
