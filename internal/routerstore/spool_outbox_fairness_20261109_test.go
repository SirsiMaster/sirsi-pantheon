package routerstore

// SOURCE ONLY / UNRUN. Private files and channel-gated in-memory transports;
// no listener, canonical service, credential, real reservation or host probe.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestRelayOutboxPassYieldsToSameLaneMaatRead(t *testing.T) {
	spool, lane := t.TempDir(), "fairness-lane"
	_, responses := seedFairnessOutbox(t, spool, lane, "001", "002", "003")
	gates := map[string]chan struct{}{}
	once := map[string]*sync.Once{}
	for _, id := range []string{"001", "002", "003"} {
		gates[id], once[id] = make(chan struct{}), &sync.Once{}
	}
	release := func(id string) { once[id].Do(func() { close(gates[id]) }) }
	entered, readEntered := make(chan string, 3), make(chan struct{}, 1)
	var mu sync.Mutex
	var order []string
	active, peak, readCalls := 0, 0, 0
	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v1/call/MaatRead" {
			mu.Lock()
			readCalls++
			mu.Unlock()
			select {
			case readEntered <- struct{}{}:
			default:
			}
			return fairnessPositiveResponse(), nil
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		id := bodyID(string(body))
		gate, ok := gates[id]
		if !ok {
			return nil, errors.New("unexpected inert held ID")
		}
		mu.Lock()
		order = append(order, id)
		active++
		if active > peak {
			peak = active
		}
		mu.Unlock()
		entered <- id
		<-gate
		mu.Lock()
		active--
		mu.Unlock()
		return fairnessPositiveResponse(), nil
	})
	rl := fairnessRelay(spool, rt)
	ctx, cancel := context.WithCancel(context.Background())
	wake, done := make(chan struct{}, 1), make(chan struct{})
	var duties sync.WaitGroup // Direct worker call: only its fresh request adds.
	go func() { rl.laneWorkerTracked(ctx, lane, wake, &duties); close(done) }()
	t.Cleanup(func() {
		cancel()
		for id := range gates {
			release(id)
		}
		awaitFairnessDone(t, done)
		joined := make(chan struct{})
		go func() { rl.workers.Wait(); duties.Wait(); close(joined) }()
		awaitFairnessDone(t, joined)
	})
	wake <- struct{}{}
	awaitFairnessHeld(t, entered, "001")
	reqDir := filepath.Join(spool, lane, "req")
	if err := os.MkdirAll(reqDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(filepath.Join(reqDir, "fresh-read.json"), spoolRequest{Method: "MaatRead", Body: []byte(`{"args":[]}`)}, 0o600); err != nil {
		t.Fatal(err)
	}
	wake <- struct{}{} // Coalesced before001 settles; no discovery timing assumption.
	release("001")
	awaitFairnessHeld(t, entered, "002")
	select {
	case <-readEntered:
	case <-time.After(time.Second):
		t.Fatal("fresh MaatRead not picked up while second held retry remained gated (old-quantum causal control)")
	}
	//002 remains gated, so the worker cannot add another fresh duty here.
	// Wait begins AFTER the read entered, proving its Add preceded this Wait.
	readDone := make(chan struct{})
	go func() { duties.Wait(); close(readDone) }()
	awaitFairnessDone(t, readDone)
	raw, err := os.ReadFile(filepath.Join(spool, lane, "res", "fresh-read.json"))
	if err != nil {
		t.Fatal("fresh read response was not published before held tail release")
	}
	var response spoolResponse
	if err := json.Unmarshal(raw, &response); err != nil || response.Status != http.StatusOK {
		t.Fatal("fresh read did not receive its own positive fixture response")
	}
	mu.Lock()
	prefix, prefixPeak := append([]string(nil), order...), peak
	mu.Unlock()
	if !reflect.DeepEqual(prefix, []string{"001", "002"}) || prefixPeak != 1 {
		t.Fatalf("held prefix=%v peak=%d; tail bypassed or gained concurrency", prefix, prefixPeak)
	}
	if _, err := os.Stat(filepath.Join(spool, lane, "outbox", "003.json")); err != nil {
		t.Fatal("untouched held tail lost while002 was gated")
	}
	assertFairnessResponses(t, spool, lane, responses)
	release("002")
	awaitFairnessHeld(t, entered, "003")
	release("003")
	cancel() // Already-started003 must settle; no new forward may begin.
	awaitFairnessDone(t, done)
	mu.Lock()
	got, gotPeak, gotReads := append([]string(nil), order...), peak, readCalls
	mu.Unlock()
	if !reflect.DeepEqual(got, []string{"001", "002", "003"}) || gotPeak != 1 || gotReads != 1 {
		t.Fatalf("held order=%v peak=%d fresh reads=%d", got, gotPeak, gotReads)
	}
	for _, dir := range []string{"req", "outbox", "inflight"} {
		files, err := filepath.Glob(filepath.Join(spool, lane, dir, "*.json"))
		if err != nil || len(files) != 0 {
			t.Fatalf("unsettled %s fixture custody: %v %v", dir, files, err)
		}
	}
	rl.dutyMu.Lock()
	count := rl.dutyCount
	rl.dutyMu.Unlock()
	if count != 0 || rl.settlementFailed.Load() {
		t.Fatal("fixture retained active duties or settlement debt")
	}
	assertFairnessResponses(t, spool, lane, responses)
}

func TestRelayOutboxPassBudgetResumesSortedPrefix(t *testing.T) {
	spool, lane := t.TempDir(), "fairness-lane"
	envelopes, responses := seedFairnessOutbox(t, spool, lane, "001", "002", "003")
	rt := &scriptedTransport{}
	rl := fairnessRelay(spool, rt)
	ids := []string{"001", "002", "003"}
	for i := range ids {
		remaining, yielded := rl.drainOutboxPass(context.Background(), lane, 1)
		if remaining != 2-i || yielded != (i < 2) || !reflect.DeepEqual(rt.order(), ids[:i+1]) {
			t.Fatalf("pass%d: remaining=%d yielded=%t order=%v", i, remaining, yielded, rt.order())
		}
		for _, id := range ids[i+1:] {
			got, err := os.ReadFile(filepath.Join(spool, lane, "outbox", id+".json"))
			if err != nil || !bytes.Equal(got, envelopes[id]) {
				t.Fatalf("untouched tail %s changed: %v", id, err)
			}
		}
		assertFairnessResponses(t, spool, lane, responses)
	}
	for _, mode := range []string{"unlimited_wrapper", "serveOnce"} {
		t.Run(mode, func(t *testing.T) {
			spool := t.TempDir()
			_, responses := seedFairnessOutbox(t, spool, lane, ids...)
			rt := &scriptedTransport{}
			rl := fairnessRelay(spool, rt)
			if mode == "unlimited_wrapper" {
				if remaining := rl.drainOutbox(context.Background(), lane); remaining != 0 {
					t.Fatalf("legacy unlimited wrapper left %d unresolved", remaining)
				}
			} else if delivered := rl.serveOnce(); delivered != 3 {
				t.Fatalf("legacy serveOnce delivered=%d", delivered)
			}
			if !reflect.DeepEqual(rt.order(), ids) {
				t.Fatalf("legacy full drain order=%v", rt.order())
			}
			assertFairnessResponses(t, spool, lane, responses)
		})
	}
}

func TestRelayOutboxPassNonDeliveryDoesNotImmediateContinue(t *testing.T) {
	for _, tc := range []struct {
		name, custody string
		calls         int
		remaining     int
		debt          bool
	}{
		{"never_sent_held", "outbox", 2, 2, false},
		{"post_send_unknown", "failed", 1, 2, false},
		{"unknown_parking_failure", "inflight", 1, 2, true},
		{"positive_cleanup_failure", "inflight", 1, 2, true},
		{"paused", "outbox", 0, 2, false},
		{"cancel_after_active_positive", "", 1, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spool, lane := t.TempDir(), "fairness-lane"
			envelopes, responses := seedFairnessOutbox(t, spool, lane, "001", "002")
			for _, dir := range []string{"inflight", "failed"} {
				if dir == "failed" && tc.name == "unknown_parking_failure" {
					if err := os.WriteFile(filepath.Join(spool, lane, dir), []byte("owned fixture obstruction"), 0o600); err != nil {
						t.Fatal(err)
					}
				} else if err := os.MkdirAll(filepath.Join(spool, lane, dir), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				body, err := io.ReadAll(r.Body)
				if err != nil {
					return nil, err
				}
				if bodyID(string(body)) != "001" {
					t.Error("attempted later held item past the non-delivery frontier")
				}
				switch tc.name {
				case "never_sent_held":
					return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("inert dial refusal")}
				case "post_send_unknown", "unknown_parking_failure":
					return nil, &net.OpError{Op: "write", Net: "tcp", Err: errors.New("inert post-send loss")}
				case "cancel_after_active_positive":
					cancel()
				}
				return fairnessPositiveResponse(), nil
			})
			rl := fairnessRelay(spool, rt)
			if tc.name == "paused" {
				rl.drainPaused = true // Owned fixture before any concurrent worker.
			}
			if tc.name == "positive_cleanup_failure" {
				rl.removeInflightFn = func(string) error { return errors.New("owned cleanup refusal") }
			}
			remaining, yielded := rl.drainOutboxPass(ctx, lane, 1)
			if remaining != tc.remaining || yielded || calls != tc.calls || rl.settlementFailed.Load() != tc.debt {
				t.Fatalf("remaining=%d yielded=%t calls=%d debt=%t", remaining, yielded, calls, rl.settlementFailed.Load())
			}
			for _, dir := range []string{"outbox", "inflight", "failed"} {
				if dir == "failed" && tc.name == "unknown_parking_failure" {
					got, err := os.ReadFile(filepath.Join(spool, lane, dir))
					if err != nil || string(got) != "owned fixture obstruction" {
						t.Fatal("parking failure changed its preexisting obstruction")
					}
					continue
				}
				got, err := os.ReadFile(filepath.Join(spool, lane, dir, "001.json"))
				if dir == tc.custody {
					if err != nil || !bytes.Equal(got, envelopes["001"]) {
						t.Fatalf("original envelope lost/changed in %s: %v", dir, err)
					}
				} else if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("unexpected first-envelope custody in %s: %v", dir, err)
				}
			}
			got, err := os.ReadFile(filepath.Join(spool, lane, "outbox", "002.json"))
			if err != nil || !bytes.Equal(got, envelopes["002"]) {
				t.Fatal("non-delivery/cancellation touched the later held envelope")
			}
			assertFairnessResponses(t, spool, lane, responses)
		})
	}
	for _, transition := range []string{"held", "unknown", "cleanup_failure", "empty", "paused", "canceled"} {
		t.Run("worker_yield_then_"+transition, func(t *testing.T) {
			fairnessWorkerTransition(t, transition)
		})
	}
}

// An unbuffered wake handshake observes the WORKER returning to its normal
// wait after a positive yield and a later non-yield result. A stale true local
// flag cannot receive this wake, even if the helper correctly returns false.
func fairnessWorkerTransition(t *testing.T, transition string) {
	t.Helper()
	spool, lane := t.TempDir(), "fairness-transition"
	ids := []string{"001", "002"}
	if transition == "canceled" {
		ids = append(ids, "003")
	}
	envelopes, responses := seedFairnessOutbox(t, spool, lane, ids...)
	ctx, cancel := context.WithCancel(context.Background())
	first, second := make(chan struct{}, 1), make(chan struct{}, 1)
	extra := make(chan struct{})
	var extraOnce sync.Once
	free := func() { extraOnce.Do(func() { close(extra) }) }
	var mu sync.Mutex
	var calls []string
	secondCalls := 0
	var rl *Relay
	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		id := bodyID(string(body))
		mu.Lock()
		calls = append(calls, id)
		if id == "002" {
			secondCalls++
		}
		n := secondCalls
		mu.Unlock()
		if id == "001" {
			if transition == "paused" {
				rl.dutyMu.Lock()
				rl.drainPaused = true
				rl.dutyMu.Unlock()
			}
			select {
			case first <- struct{}{}:
			default:
			}
			return fairnessPositiveResponse(), nil
		}
		if id != "002" {
			return nil, errors.New("unexpected later transition fixture ID")
		}
		select {
		case second <- struct{}{}:
		default:
		}
		// Bound a broken held-result continuation without a CPU retry loop.
		// An explicit probe wake is allowed to initiate another old-style retry.
		if transition == "held" && n > 2 {
			<-extra
		}
		switch transition {
		case "held":
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("inert transition dial refusal")}
		case "unknown":
			return nil, &net.OpError{Op: "write", Net: "tcp", Err: errors.New("inert transition post-send loss")}
		case "canceled":
			cancel() //001 already yielded;002 is active and must settle normally.
		}
		return fairnessPositiveResponse(), nil
	})
	rl = fairnessRelay(spool, rt)
	if transition == "cleanup_failure" {
		rl.removeInflightFn = func(path string) error {
			if filepath.Base(path) == "002.json" {
				return errors.New("inert transition cleanup refusal")
			}
			return os.Remove(path) // Only a newly created owned fixture file.
		}
	}
	wake, done := make(chan struct{}), make(chan struct{})
	probeDone := make(chan struct{})
	probeStarted := false
	go func() { rl.laneWorkerTracked(ctx, lane, wake, nil); close(done) }()
	t.Cleanup(func() {
		cancel()
		free()
		awaitFairnessDone(t, done)
		if probeStarted {
			awaitFairnessDone(t, probeDone)
		}
	})
	select {
	case wake <- struct{}{}:
	case <-time.After(time.Second):
		t.Fatal("transition worker did not accept its initial wake")
	}
	entered := second
	if transition == "paused" {
		entered = first //002 is refused by beginDuty, before the transport.
	}
	awaitFairnessDone(t, entered)
	if transition != "canceled" {
		resumed := make(chan struct{})
		probeStarted = true
		go func() {
			defer close(probeDone)
			select {
			case wake <- struct{}{}:
				close(resumed)
			case <-ctx.Done():
			}
		}()
		select {
		case <-resumed:
		case <-time.After(time.Second):
			t.Fatal("worker did not clear positive continuation after " + transition)
		}
	}
	cancel()
	free()
	awaitFairnessDone(t, done)
	mu.Lock()
	got := append([]string(nil), calls...)
	mu.Unlock()
	if len(got) == 0 || got[0] != "001" {
		t.Fatalf("missing positive prefix: %v", got)
	}
	for _, id := range got[1:] {
		if id != "002" || transition == "paused" {
			t.Fatalf("replayed positive head or crossed stopped frontier: %v", got)
		}
	}
	if transition == "canceled" {
		if !reflect.DeepEqual(got, []string{"001", "002"}) {
			t.Fatalf("cancellation crossed the remaining frontier: %v", got)
		}
		tail, err := os.ReadFile(filepath.Join(spool, lane, "outbox", "003.json"))
		if err != nil || !bytes.Equal(tail, envelopes["003"]) {
			t.Fatal("canceled continued pass changed untouched003")
		}
	}
	if transition == "empty" || transition == "unknown" || transition == "cleanup_failure" {
		if !reflect.DeepEqual(got, []string{"001", "002"}) {
			t.Fatalf("settled/parked head was replayed: %v", got)
		}
	}
	if transition == "empty" {
		for _, dir := range []string{"outbox", "inflight", "failed"} {
			files, err := filepath.Glob(filepath.Join(spool, lane, dir, "*.json"))
			if err != nil || len(files) != 0 {
				t.Fatalf("empty continuation retained %s custody: %v %v", dir, files, err)
			}
		}
	}
	if transition == "held" || transition == "paused" {
		held, err := os.ReadFile(filepath.Join(spool, lane, "outbox", "002.json"))
		if err != nil || !bytes.Equal(held, envelopes["002"]) {
			t.Fatal("held/paused worker lost original002 custody")
		}
	}
	if transition == "unknown" || transition == "cleanup_failure" {
		dir := "failed"
		if transition == "cleanup_failure" {
			dir = "inflight"
		}
		held, err := os.ReadFile(filepath.Join(spool, lane, dir, "002.json"))
		if err != nil || !bytes.Equal(held, envelopes["002"]) {
			t.Fatal("worker lost unknown/cleanup custody")
		}
	}
	if rl.settlementFailed.Load() != (transition == "cleanup_failure") {
		t.Fatal("worker changed sticky settlement debt")
	}
	rl.dutyMu.Lock()
	count := rl.dutyCount
	rl.dutyMu.Unlock()
	if count != 0 {
		t.Fatal("transition worker retained admitted duties")
	}
	assertFairnessResponses(t, spool, lane, responses)
}

func fairnessRelay(spool string, rt http.RoundTripper) *Relay {
	return &Relay{Spool: spool, Base: "http://fixture.invalid", Token: "inert-fixture", Client: &http.Client{Transport: rt},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), now: time.Now}
}

func fairnessPositiveResponse() *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader([]byte(`{"result":[]}`)))}
}

func seedFairnessOutbox(t *testing.T, spool, lane string, ids ...string) (map[string][]byte, map[string][]byte) {
	t.Helper()
	seedOutbox(t, filepath.Join(spool, lane), ids...)
	res := filepath.Join(spool, lane, "res")
	if err := os.MkdirAll(res, 0o700); err != nil {
		t.Fatal(err)
	}
	envelopes, responses := map[string][]byte{}, map[string][]byte{}
	for _, id := range ids {
		var err error
		envelopes[id], err = os.ReadFile(filepath.Join(spool, lane, "outbox", id+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := writeAtomic(filepath.Join(res, id+".json"), queuedForRetryResponse(id), 0o600); err != nil {
			t.Fatal(err)
		}
		responses[id], err = os.ReadFile(filepath.Join(res, id+".json"))
		if err != nil {
			t.Fatal(err)
		}
	}
	return envelopes, responses
}

func assertFairnessResponses(t *testing.T, spool, lane string, want map[string][]byte) {
	t.Helper()
	for id, original := range want {
		got, err := os.ReadFile(filepath.Join(spool, lane, "res", id+".json"))
		if err != nil || !bytes.Equal(got, original) {
			t.Fatalf("original caller202 response %s changed: %v", id, err)
		}
	}
}

func awaitFairnessHeld(t *testing.T, entered <-chan string, want string) {
	t.Helper()
	select {
	case got := <-entered:
		if got != want {
			t.Fatalf("held order=%s want=%s", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("inert held forward %s did not enter", want)
	}
}

func awaitFairnessDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("owned fairness fixture did not settle")
	}
}
