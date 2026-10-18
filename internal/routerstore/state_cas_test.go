package routerstore

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
)

func TestStateCASPresenceAndLegacyFence(t *testing.T) {
	s := newTestStore(t)
	if ok, err := s.CompareAndSwapState("maat:reservations", "", false, "first"); err != nil || !ok {
		t.Fatalf("create: %v %v", ok, err)
	}
	for _, c := range []struct {
		old    string
		exists bool
		want   bool
	}{{"", false, false}, {"wrong", true, false}, {"first", true, true}} {
		ok, err := s.CompareAndSwapState("maat:reservations", c.old, c.exists, "second")
		if err != nil || ok != c.want {
			t.Fatalf("CAS %+v: %v %v", c, ok, err)
		}
	}
	if err := s.SetState("maat:reservations", "lost"); err == nil {
		t.Fatal("legacy overwrite accepted")
	}
	got, exists, err := s.GetState("maat:reservations")
	if err != nil || !exists || got != "second" {
		t.Fatalf("fence changed state: %q %v", got, err)
	}
	if err := s.SetState("other", ""); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.CompareAndSwapState("other", "", false, "bad"); err != nil || ok {
		t.Fatalf("empty != absent: %v %v", ok, err)
	}
	if ok, err := s.CompareAndSwapState("other", "", true, "good"); err != nil || !ok {
		t.Fatalf("empty value CAS: %v %v", ok, err)
	}
}
func TestStateCASConcurrentSingleWinner(t *testing.T) {
	s := newTestStore(t)
	var wg sync.WaitGroup
	results := make(chan bool, 16)
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := s.CompareAndSwapState("race", "", false, "winner")
			results <- ok
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	winners := 0
	for ok := range results {
		if ok {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("winners %d", winners)
	}
}
func TestRemoteStateCAS(t *testing.T) {
	backend := newTestStore(t)
	handler, err := Handler(backend, ServerOptions{Token: "test-cas"})
	if err != nil {
		t.Fatal(err)
	}
	rs := NewRemoteStore("http://cas.test", "test-cas")
	rs.sessionDir = ""
	rs.client = &http.Client{Transport: casHandlerTransport{handler}}
	if ok, err := rs.CompareAndSwapState("maat:reservations", "", false, "first"); err != nil || !ok {
		t.Fatalf("remote create: %v %v", ok, err)
	}
	if ok, err := rs.CompareAndSwapState("maat:reservations", "stale", true, "lost"); err != nil || ok {
		t.Fatalf("remote conflict: %v %v", ok, err)
	}
	if err := rs.SetState("maat:reservations", "lost"); err == nil {
		t.Fatal("legacy remote write accepted")
	}
	if got, _, err := backend.GetState("maat:reservations"); err != nil || got != "first" {
		t.Fatalf("remote state lost: %q %v", got, err)
	}
}

// Exercise the signed wire codec and real service handler without a listener.
type casHandlerTransport struct{ handler http.Handler }

func (tr casHandlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	tr.handler.ServeHTTP(recorder, req)
	return recorder.Result(), nil
}

func TestStateCASIndependentConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cas.db")
	first, err := OpenPath(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := OpenPath(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if ok, err := first.CompareAndSwapState("two-handles", "", false, "start"); err != nil || !ok {
		t.Fatalf("initial: %v %v", ok, err)
	}
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	errs := make(chan error, 2)
	for i, store := range []*SQLiteStore{first, second} {
		wg.Add(1)
		go func(i int, store *SQLiteStore) {
			defer wg.Done()
			ok, err := store.CompareAndSwapState("two-handles", "start", true, fmt.Sprint(i))
			results <- ok
			errs <- err
		}(i, store)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	winners := 0
	for ok := range results {
		if ok {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("independent-handle winners: %d", winners)
	}
}
