package routerstore

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSSAAtomicScopeRefusalIsForbidden(t *testing.T) {
	w := httptest.NewRecorder()
	(&server{}).writeCallErr(w, "DeleteThreadCAS", fmt.Errorf("atomic predicate: %w", ErrLaneScope))
	if w.Code != http.StatusForbidden || !bytes.Contains(w.Body.Bytes(), []byte("ErrLaneScope")) {
		t.Fatalf("scope denial must be 403, not a retryable server fault: %d %s", w.Code, w.Body.String())
	}
}

func TestSSAListThreadsLeaksForeignPayload(t *testing.T) {
	f := newSSAFixture(t, []string{"cloud-a"})
	marker := []byte(`{"suspend_payload":{"resume_prompt":"PRIVATE_OWNER_WORK"}}`)
	if err := f.backend.UpsertThreads([]ThreadRecord{{ThreadID: "foreign", Agent: "ra", Status: "active", Host: "mac-host", LastSeenAt: "2026-10-12T00:00:00Z", Payload: marker}}); err != nil {
		t.Fatal(err)
	}
	rows, err := f.client().ListThreads()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Agent == "ra" && bytes.Equal(r.Payload, marker) {
			t.Fatalf("CONFIDENTIALITY BREACH: scoped cloud-a read foreign raw resume payload")
		}
	}
}

func TestSSAThreadOverwriteForeignAgent(t *testing.T) {
	for _, host := range []string{"", "cloud-host"} {
		for _, method := range []string{"cas", "bulk", "resume"} {
			t.Run(host+"/"+method, func(t *testing.T) {
				f := newSSAFixture(t, []string{"cloud-a"})
				status := "active"
				if method == "resume" {
					status = "suspended"
				}
				old := ThreadRecord{ThreadID: "victim", Agent: "ra", Host: host, Status: status, LastSeenAt: "2026-10-12T00:00:00Z", Payload: []byte(`{}`)}
				if err := f.backend.UpsertThreads([]ThreadRecord{old}); err != nil {
					t.Fatal(err)
				}
				next := ThreadRecord{ThreadID: old.ThreadID, Agent: "cloud-a", Status: "active", LastSeenAt: "2026-10-12T00:00:01Z", Payload: []byte(`{}`)}
				rs := f.client()
				switch method {
				case "cas":
					_, _ = rs.UpsertThreadCAS(next)
				case "bulk":
					_ = rs.UpsertThreads([]ThreadRecord{next})
				case "resume":
					_ = rs.ResumeThreadCAS(next, old.LastSeenAt)
				}
				got, err := f.backend.ThreadBinding(old.ThreadID)
				if err != nil {
					t.Fatal(err)
				}
				if got.Agent != "ra" {
					t.Fatalf("AUTHORITY BREACH: scoped cloud-a rewrote ra thread: %+v", got)
				}
			})
		}
	}
}

type ssaTransport struct{ handler http.Handler }

func (tr ssaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	tr.handler.ServeHTTP(w, req)
	return w.Result(), nil
}

type ssaFixture struct {
	backend *SQLiteStore
	handler http.Handler
	token   string
}

func newSSAFixture(t *testing.T, scope []string) *ssaFixture {
	b := newTestStore(t)
	b.notifyDir = t.TempDir()
	h, err := Handler(b, ServerOptions{Token: "test-bootstrap"})
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := b.MintScopedHostToken("cloud-host", "test", scope)
	if err != nil {
		t.Fatal(err)
	}
	return &ssaFixture{b, h, tok}
}
func (f *ssaFixture) client() *RemoteStore {
	r := NewRemoteStore("http://isolated.invalid", f.token)
	r.sessionDir = ""
	r.host = "cloud-host"
	r.agent = "cloud-a"
	r.threadID = ""
	r.client = &http.Client{Transport: ssaTransport{f.handler}}
	return r
}

type ssaRaceStore struct {
	*SQLiteStore
	backend *SQLiteStore
	armed   bool
}

func (s *ssaRaceStore) ThreadBinding(id string) (ThreadBinding, error) {
	b, e := s.SQLiteStore.ThreadBinding(id)
	if s.armed && id == "race-victim" && e == nil {
		s.armed = false
		r := ThreadRecord{ThreadID: id, Agent: "ra", Host: "cloud-host", Status: "active", LastSeenAt: "2026-10-12T00:00:00.500000000Z", Payload: []byte(`{}`)}
		if err := s.backend.UpsertThreads([]ThreadRecord{r}); err != nil {
			return b, err
		}
		got, err := s.backend.ThreadBinding(id)
		if err != nil || got.Agent != "ra" {
			return b, fmt.Errorf("race injection did not change owner: %+v %v", got, err)
		}
	}
	return b, e
}
func TestSSAConcurrentExistingAgentChange(t *testing.T) {
	f := newSSAFixture(t, []string{"cloud-a"})
	r := ThreadRecord{ThreadID: "race-victim", Agent: "cloud-a", Host: "cloud-host", Status: "active", LastSeenAt: "2026-10-12T00:00:00.000000000Z", Payload: []byte(`{}`)}
	if err := f.backend.UpsertThreads([]ThreadRecord{r}); err != nil {
		t.Fatal(err)
	}
	w := &ssaRaceStore{SQLiteStore: f.backend, backend: f.backend}
	h, err := Handler(w, ServerOptions{Token: "test-bootstrap"})
	if err != nil {
		t.Fatal(err)
	}
	f.handler = h
	client := f.client()
	w.armed = true
	r.LastSeenAt = "2026-10-12T00:00:01.000000000Z"
	err = client.UpsertThreads([]ThreadRecord{r})
	got, e := f.backend.ThreadBinding(r.ThreadID)
	if e != nil {
		t.Fatal(e)
	}
	if got.Agent != "ra" {
		t.Fatalf("ATOMIC AUTHORITY BREACH: concurrent foreign agent change overwritten %+v err=%v", got, err)
	}
}

func TestSSAScopedDeleteBindsCurrentAgent(t *testing.T) {
	f := newSSAFixture(t, []string{"cloud-a"})
	r := ThreadRecord{ThreadID: "delete-victim", Agent: "ra", Host: "cloud-host", Status: "active", LastSeenAt: "2026-10-12T00:00:00Z", Payload: []byte(`{}`)}
	if err := f.backend.UpsertThreads([]ThreadRecord{r}); err != nil {
		t.Fatal(err)
	}
	deleted, err := f.backend.DeleteThreadCASScoped(r.ThreadID, r.Status, r.LastSeenAt, r.Host, []string{"cloud-a"})
	if deleted || !errors.Is(err, ErrLaneScope) {
		t.Fatalf("foreign delete accepted: %v %v", deleted, err)
	}
	if _, err := f.backend.ThreadBinding(r.ThreadID); err != nil {
		t.Fatal(err)
	}
	deleted, err = f.backend.DeleteThreadCASScoped(r.ThreadID, r.Status, r.LastSeenAt, r.Host, []string{"ra"})
	if !deleted || err != nil {
		t.Fatalf("own delete refused: %v %v", deleted, err)
	}
}

type ssaNoAtomicStore struct{ Store }

func TestSSAScopedMutationRequiresAtomicBackend(t *testing.T) {
	f := newSSAFixture(t, []string{"cloud-a"})
	h, err := Handler(ssaNoAtomicStore{f.backend}, ServerOptions{Token: "test-bootstrap"})
	if err != nil {
		t.Fatal(err)
	}
	f.handler = h
	err = f.client().UpsertThreads([]ThreadRecord{{ThreadID: "must-not-insert", Agent: "cloud-a", Status: "active", LastSeenAt: "2026-10-12T00:00:00Z", Payload: []byte(`{}`)}})
	if err == nil {
		t.Fatal("missing atomic capability silently fell back")
	}
	if _, err := f.backend.ThreadBinding("must-not-insert"); !errors.Is(err, ErrThreadUnknown) {
		t.Fatalf("unexpected mutation: %v", err)
	}
}

func TestSSAScopedBulkForeignConflictRollsBackEarlierInsert(t *testing.T) {
	f := newSSAFixture(t, []string{"cloud-a"})
	old := ThreadRecord{ThreadID: "bulk-victim", Agent: "ra", Host: "cloud-host", Status: "active", LastSeenAt: "2026-10-12T00:00:00Z", Payload: []byte(`{}`)}
	if err := f.backend.UpsertThreads([]ThreadRecord{old}); err != nil {
		t.Fatal(err)
	}
	err := f.backend.UpsertThreadsScoped([]ThreadRecord{
		{ThreadID: "bulk-new", Agent: "cloud-a", Status: "active", LastSeenAt: "2026-10-12T00:00:01Z", Payload: []byte(`{}`)},
		{ThreadID: old.ThreadID, Agent: "cloud-a", Host: old.Host, Status: "active", LastSeenAt: "2026-10-12T00:00:01Z", Payload: []byte(`{}`)},
	}, []string{"cloud-a"})
	if !errors.Is(err, ErrLaneScope) {
		t.Fatalf("conflicting bulk accepted: %v", err)
	}
	if _, err := f.backend.ThreadBinding("bulk-new"); !errors.Is(err, ErrThreadUnknown) {
		t.Fatalf("partial bulk committed: %v", err)
	}
}
