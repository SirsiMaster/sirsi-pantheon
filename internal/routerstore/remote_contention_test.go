package routerstore

// G3 (ROUTER_SERVICE_GOAL): two nodes contend for one task through the HTTP service, many
// times, with the service slowed to about twice its normal worst case and with a database
// outage in the middle; exactly one wins every round. The two clients are separate
// RemoteStores (separate sessions, separate retry loops) over real HTTP to one service.
// What this is NOT: two machines. The live two-Mac run stays an owner-scheduled rehearsal.
//
// Negative control: the same loop against two SEPARATE services (the split-brain G3 names)
// must show both nodes "winning", or the counter cannot detect the failure it exists for.

import (
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// slowService serves a backend over httptest with injectable latency and outage.
type slowService struct {
	backend *SQLiteStore
	srv     *httptest.Server
	maxLag  atomic.Int64 // nanoseconds; each request sleeps a random 0..maxLag
	down    atomic.Bool  // while true every request is a 503, as a dead database produces
	rng     *rand.Rand
	rngMu   sync.Mutex
}

func newSlowService(t *testing.T) *slowService {
	t.Helper()
	backend := openBackendStore(t, filepath.Join(t.TempDir(), "router.db"))
	t.Cleanup(func() { _ = backend.Close() })
	backend.notifyDir = t.TempDir()
	h, err := Handler(backend, ServerOptions{Token: "t0k", MaxWait: 3 * time.Second})
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	s := &slowService{backend: backend, rng: rand.New(rand.NewSource(62))}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if lag := s.maxLag.Load(); lag > 0 {
			s.rngMu.Lock()
			d := time.Duration(s.rng.Int63n(lag + 1))
			s.rngMu.Unlock()
			time.Sleep(d)
		}
		if s.down.Load() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"name":"","message":"database unavailable"}}`))
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *slowService) client() *RemoteStore {
	rs := NewRemoteStore(s.srv.URL, "t0k")
	rs.sessionDir = "" // never touch ~/.sirsi/sessions from a test
	return rs
}

// claimWithRetry is what a real node does: back off and retry while the service is
// unavailable, and report whether it won.
func claimWithRetry(rs *RemoteStore, id string, deadline time.Duration) (won bool, err error) {
	stop := time.Now().Add(deadline)
	for {
		_, err = rs.ClaimTask("contender", id, "worker", "thread", time.Minute)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, ErrNoClaimableTask):
			return false, nil
		case errors.Is(err, ErrServiceUnavailable):
			if time.Now().After(stop) {
				return false, fmt.Errorf("still unavailable after %s: %w", deadline, err)
			}
			time.Sleep(20 * time.Millisecond)
		default:
			return false, err
		}
	}
}

// raceForTask has both clients race for task id and returns the number of winners.
func raceForTask(t *testing.T, a, b *RemoteStore, id string, deadline time.Duration) int {
	t.Helper()
	var wg sync.WaitGroup
	var wins atomic.Int32
	for _, c := range []*RemoteStore{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			won, err := claimWithRetry(c, id, deadline)
			if err != nil {
				t.Errorf("task %s: %v", id, err)
				return
			}
			if won {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	return int(wins.Load())
}

func TestTwoNodesContendThroughTheServiceExactlyOnceUnderDelayAndOutage(t *testing.T) {
	rounds, outage := 1000, 30*time.Second
	if testing.Short() {
		rounds, outage = 300, 1500*time.Millisecond // CI keeps the same shape, scaled
	}
	svc := newSlowService(t)
	a, b := svc.client(), svc.client()
	for i := 0; i < rounds; i++ {
		if err := svc.backend.AddTask(Task{Agent: "contender", TaskID: fmt.Sprintf("r%04d", i), Subject: "contend"}); err != nil {
			t.Fatal(err)
		}
	}
	// Measure the service's own worst case on a few uncontended calls, then run at twice it.
	var worst time.Duration
	for i := 0; i < 20; i++ {
		start := time.Now()
		if _, err := a.Get("missing-" + fmt.Sprint(i)); !errors.Is(err, ErrNotFound) {
			t.Fatalf("baseline call: %v", err)
		}
		if d := time.Since(start); d > worst {
			worst = d
		}
	}
	lag := 2 * worst
	if lag < 5*time.Millisecond {
		lag = 5 * time.Millisecond
	}
	svc.maxLag.Store(int64(lag))
	t.Logf("baseline worst %s; injected latency 0..%s; %d rounds; outage %s mid-run", worst, lag, rounds, outage)

	outageAt := rounds / 2
	for i := 0; i < rounds; i++ {
		if i == outageAt {
			svc.down.Store(true)
			time.AfterFunc(outage, func() { svc.down.Store(false) })
		}
		if w := raceForTask(t, a, b, fmt.Sprintf("r%04d", i), outage+30*time.Second); w != 1 {
			t.Fatalf("round %d: %d winners through one service, want exactly 1", i, w)
		}
	}
}

// The negative control: two separate services (two ledgers) let both nodes win. If this
// counted zero split-brain rounds the test above would prove nothing.
func TestContentionCounterDetectsSplitBrain(t *testing.T) {
	x, y := newSlowService(t), newSlowService(t)
	a, b := x.client(), y.client()
	split := 0
	for i := 0; i < 25; i++ {
		id := fmt.Sprintf("c%02d", i)
		for _, s := range []*slowService{x, y} {
			if err := s.backend.AddTask(Task{Agent: "contender", TaskID: id, Subject: "contend"}); err != nil {
				t.Fatal(err)
			}
		}
		if raceForTask(t, a, b, id, 10*time.Second) == 2 {
			split++
		}
	}
	if split == 0 {
		t.Fatal("two separate ledgers never produced two winners: the counter cannot detect split-brain")
	}
}
