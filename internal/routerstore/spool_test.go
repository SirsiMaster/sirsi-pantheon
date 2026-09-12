package routerstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// End to end through files: a real Handler behind httptest stands in for the
// service, the Relay forwards with the host token, and a RemoteStore over
// spool:// (no token) mints a session, signs and works the ledger.
func TestSpoolRelayEndToEnd(t *testing.T) {
	backend := newDst(t)
	h, err := Handler(backend, ServerOptions{Token: "h0st", MaxWait: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	spool := t.TempDir()
	logBuf := &strings.Builder{}
	rl := &Relay{Spool: spool, Base: srv.URL, Token: "h0st", Log: slog.New(slog.NewTextHandler(logBuf, nil))}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = rl.Serve(ctx) }()

	t.Setenv("SIRSI_AGENT_ID", "lane-x")
	t.Setenv("HOME", t.TempDir()) // session cache dir
	rs := NewRemoteStore("spool://"+spool, "")
	if rs.token != "" {
		t.Fatal("spool client must hold no token")
	}
	id, _, err := rs.SendGuarded(SendReq{From: "lane-x", To: "b", Title: "via spool", Type: "proposal", Instructions: "x"})
	if err != nil {
		t.Fatalf("send through spool: %v\nrelay log:\n%s", err, logBuf.String())
	}
	it, err := rs.Get(id)
	if err != nil || it.Title != "via spool" {
		t.Fatalf("get through spool: %v %+v", err, it)
	}
	// Files are cleaned up after each round trip.
	if left, _ := filepath.Glob(filepath.Join(spool, "lane-x", "*", "*.json")); len(left) != 0 {
		t.Fatalf("spool not cleaned: %v", left)
	}
	// The relay log never carries a token or a signature.
	if l := logBuf.String(); strings.Contains(l, "h0st") || strings.Contains(l, "X-Sirsi-Signature") || !strings.Contains(l, "method=SendGuarded") {
		t.Fatalf("relay log must be secret-free and name the method:\n%s", l)
	}
	// Token methods are refused by name, never forwarded.
	var out []any
	if err := rs.call("ListHostTokens", nil, &out); err == nil || !strings.Contains(err.Error(), "never forwarded") {
		t.Fatalf("ListHostTokens must be refused by the relay: %v", err)
	}
}

// Without a relay the lane fails loudly and leaves nothing behind.
func TestSpoolWithoutRelayTimesOut(t *testing.T) {
	spool := t.TempDir()
	tr := newSpoolTransport(spool, "lane-y")
	tr.wait = 400 * time.Millisecond
	t.Setenv("SIRSI_AGENT_ID", "lane-y")
	t.Setenv("HOME", t.TempDir())
	rs := NewRemoteStore("spool://"+spool, "")
	rs.client.Transport = tr
	_, err := rs.Get("nope")
	if err == nil || !strings.Contains(err.Error(), "relay serve") || !strings.Contains(err.Error(), "MintSession id ") {
		t.Fatalf("want a loud no-relay error, got %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(spool, "lane-y", "req", "*.json")); len(left) != 0 {
		t.Fatalf("request left behind after timeout: %v", left)
	}
}

// Resolve accepts spool:// without a token and still refuses http(s) without one.
func TestResolveSpoolNeedsNoToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SIRSI_ROUTER_TOKEN", "")
	t.Setenv("SIRSI_ROUTER_URL", "spool://"+t.TempDir())
	s, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(*RemoteStore); !ok {
		t.Fatalf("got %T", s)
	}
	t.Setenv("SIRSI_ROUTER_URL", "https://router.invalid")
	if _, err := Resolve(); err == nil {
		t.Fatal("https without token must still be refused")
	}
}

// At-most-once: a relay that dies after consuming (in-flight file present) must
// not re-forward on restart; the caller receives outcome-unknown and the
// service sees zero calls for it. A request another relay already consumed is
// never forwarded either.
func TestSpoolRelayNeverReplaysInflight(t *testing.T) {
	spool := t.TempDir()
	var calls atomic.Int32
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); _, _ = w.Write([]byte(`{"result":[]}`)) }))
	t.Cleanup(svc.Close)
	lane := filepath.Join(spool, "lane-z")
	for _, d := range []string{"req", "inflight", "res"} {
		if err := os.MkdirAll(filepath.Join(lane, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// A previous relay consumed this Send and died before publishing.
	if err := writeAtomic(filepath.Join(lane, "inflight", "1-1-dead.json"), spoolRequest{Method: "SendGuarded", Headers: map[string]string{}, Body: []byte(`{"args":[]}`)}, 0o600); err != nil {
		t.Fatal(err)
	}
	rl := &Relay{Spool: spool, Base: svc.URL, Token: "t", Log: slog.New(slog.NewTextHandler(&strings.Builder{}, nil)), Client: svc.Client(), now: time.Now}
	if n := rl.recoverInflight(); n != 1 {
		t.Fatalf("recover: %d", n)
	}
	b, err := os.ReadFile(filepath.Join(lane, "res", "1-1-dead.json"))
	var sr spoolResponse
	if err != nil || json.Unmarshal(b, &sr) != nil || sr.Status != http.StatusBadGateway || !strings.Contains(string(sr.Body), "OUTCOME UNKNOWN") || !strings.Contains(string(sr.Body), "SendGuarded 1-1-dead") {
		t.Fatalf("in-flight file must yield outcome-unknown naming method+id: %s %v", b, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("restart must never re-forward: %d calls", calls.Load())
	}
	if _, err := os.Stat(filepath.Join(lane, "inflight", "1-1-dead.json")); !os.IsNotExist(err) {
		t.Fatal("in-flight file must be removed after reporting")
	}
	// Consume happens before forward: a request that vanishes between glob and
	// rename (another relay took it) is not forwarded.
	if err := writeAtomic(filepath.Join(lane, "req", "1-2-live.json"), spoolRequest{Method: "ListAll", Headers: map[string]string{}, Body: []byte(`{"args":[]}`)}, 0o600); err != nil {
		t.Fatal(err)
	}
	if n := rl.serveOnce(); n != 1 || calls.Load() != 1 {
		t.Fatalf("live request: handled=%d calls=%d", n, calls.Load())
	}
	if left, _ := filepath.Glob(filepath.Join(lane, "inflight", "*")); len(left) != 0 {
		t.Fatalf("in-flight must be deleted after publish: %v", left)
	}
}

// Response lost after the forward (relay published nothing): the lane reports
// OUTCOME UNKNOWN naming the method and id, and does not retry by itself.
func TestSpoolLostResponseIsOutcomeUnknown(t *testing.T) {
	spool := t.TempDir()
	tr := newSpoolTransport(spool, "lane-w")
	tr.wait = 600 * time.Millisecond
	// A "relay" that consumes (renames into inflight) but never answers.
	go func() {
		for i := 0; i < 40; i++ {
			files, _ := filepath.Glob(filepath.Join(spool, "lane-w", "req", "*.json"))
			for _, f := range files {
				_ = os.MkdirAll(filepath.Join(spool, "lane-w", "inflight"), 0o700)
				_ = os.Rename(f, filepath.Join(spool, "lane-w", "inflight", filepath.Base(f)))
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	req, _ := http.NewRequest(http.MethodPost, "http://spool/v1/call/SendGuarded", strings.NewReader(`{"args":[]}`))
	_, err := tr.RoundTrip(req)
	if err == nil || !strings.Contains(err.Error(), "OUTCOME UNKNOWN") || !strings.Contains(err.Error(), "SendGuarded id ") {
		t.Fatalf("want outcome-unknown naming method+id, got %v", err)
	}
}

// A canceled or short-deadline context (RemoteStore's own 5 s per call) after
// a relay consumed the request must surface OUTCOME UNKNOWN with method and id,
// never a bare "context deadline exceeded"; before consumption it is withdrawn
// and reported as not sent.
func TestSpoolCancelledMutationNamesOutcome(t *testing.T) {
	spool := t.TempDir()
	tr := newSpoolTransport(spool, "lane-c")
	// (a) nothing consumes it: withdrawn, nothing sent.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://spool/v1/call/SendGuarded", strings.NewReader(`{"args":[]}`))
	_, err := tr.RoundTrip(req)
	if err == nil || !strings.Contains(err.Error(), "SendGuarded id ") || !strings.Contains(err.Error(), "nothing was sent") {
		t.Fatalf("unconsumed + canceled must be withdrawn and named: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(spool, "lane-c", "req", "*.json")); len(left) != 0 {
		t.Fatalf("withdrawn request left behind: %v", left)
	}
	// (b) consumed (renamed into inflight) but unanswered: OUTCOME UNKNOWN.
	go func() {
		for i := 0; i < 50; i++ {
			files, _ := filepath.Glob(filepath.Join(spool, "lane-c", "req", "*.json"))
			for _, f := range files {
				_ = os.MkdirAll(filepath.Join(spool, "lane-c", "inflight"), 0o700)
				_ = os.Rename(f, filepath.Join(spool, "lane-c", "inflight", filepath.Base(f)))
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel2()
	req2, _ := http.NewRequestWithContext(ctx2, http.MethodPost, "http://spool/v1/call/SendGuarded", strings.NewReader(`{"args":[]}`))
	_, err = tr.RoundTrip(req2)
	if err == nil || !strings.Contains(err.Error(), "OUTCOME UNKNOWN") || !strings.Contains(err.Error(), "SendGuarded id ") || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("consumed + canceled must be outcome-unknown naming method, id and cause: %v", err)
	}
}

// Bounds are enforced on the DECODED body, not the base64 envelope: 4 MiB minus
// one byte is forwarded; 4 MiB plus one byte is refused with 413 before any
// forward.
func TestSpoolBodyBoundaryOnDecodedBytes(t *testing.T) {
	spool := t.TempDir()
	var calls atomic.Int32
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"result":[]}`))
	}))
	t.Cleanup(svc.Close)
	rl := &Relay{Spool: spool, Base: svc.URL, Token: "t", Log: slog.New(slog.NewTextHandler(&strings.Builder{}, nil)), Client: svc.Client(), now: time.Now}
	lane := filepath.Join(spool, "lane-b")
	for _, d := range []string{"req", "res"} {
		if err := os.MkdirAll(filepath.Join(lane, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	big := bytes.Repeat([]byte("x"), spoolMaxBody-1)
	if err := writeAtomic(filepath.Join(lane, "req", "1-1-ok.json"), spoolRequest{Method: "ListAll", Headers: map[string]string{}, Body: big}, 0o600); err != nil {
		t.Fatal(err)
	}
	rl.serveOnce()
	b, _ := os.ReadFile(filepath.Join(lane, "res", "1-1-ok.json"))
	var sr spoolResponse
	if json.Unmarshal(b, &sr) != nil || sr.Status != 200 || calls.Load() != 1 {
		t.Fatalf("4 MiB-1 must be forwarded: status=%d calls=%d", sr.Status, calls.Load())
	}
	over := bytes.Repeat([]byte("x"), spoolMaxBody+1)
	if err := writeAtomic(filepath.Join(lane, "req", "1-2-over.json"), spoolRequest{Method: "ListAll", Headers: map[string]string{}, Body: over}, 0o600); err != nil {
		t.Fatal(err)
	}
	rl.serveOnce()
	b, _ = os.ReadFile(filepath.Join(lane, "res", "1-2-over.json"))
	if json.Unmarshal(b, &sr) != nil || sr.Status != http.StatusRequestEntityTooLarge || calls.Load() != 1 {
		t.Fatalf("4 MiB+1 must be refused with 413 before forwarding: status=%d calls=%d", sr.Status, calls.Load())
	}
}

// A request whose consumption fails (inflight/ is not writable) is never
// forwarded and stays where it is.
func TestSpoolFailedConsumeIsNotForwarded(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores modes")
	}
	spool := t.TempDir()
	var calls atomic.Int32
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); _, _ = w.Write([]byte(`{"result":[]}`)) }))
	t.Cleanup(svc.Close)
	rl := &Relay{Spool: spool, Base: svc.URL, Token: "t", Log: slog.New(slog.NewTextHandler(&strings.Builder{}, nil)), Client: svc.Client(), now: time.Now}
	lane := filepath.Join(spool, "lane-f")
	for _, d := range []string{"req", "inflight"} {
		if err := os.MkdirAll(filepath.Join(lane, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeAtomic(filepath.Join(lane, "req", "1-1-x.json"), spoolRequest{Method: "SendGuarded", Headers: map[string]string{}, Body: []byte(`{"args":[]}`)}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(lane, "inflight"), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(lane, "inflight"), 0o700) })
	if n := rl.serveOnce(); n != 0 || calls.Load() != 0 {
		t.Fatalf("failed consume must not forward: handled=%d calls=%d", n, calls.Load())
	}
	if _, err := os.Stat(filepath.Join(lane, "req", "1-1-x.json")); err != nil {
		t.Fatal("request must remain in req/ when consumption fails")
	}
}

// The in-flight cap is atomic across callers: the 65th concurrent publisher is
// refused before writing anything.
func TestSpoolInFlightCapIsAtomic(t *testing.T) {
	spool := t.TempDir()
	tr := newSpoolTransport(spool, "lane-s")
	slots := filepath.Join(tr.dir, "slots")
	if err := os.MkdirAll(slots, 0o700); err != nil {
		t.Fatal(err)
	}
	var held []string
	for i := 0; i < spoolMaxInFlight; i++ {
		p, err := acquireSlot(slots)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, p)
	}
	if _, err := acquireSlot(slots); err == nil || !strings.Contains(err.Error(), "in flight") {
		t.Fatalf("65th must be refused: %v", err)
	}
	_ = os.Remove(held[0])
	if _, err := acquireSlot(slots); err != nil {
		t.Fatalf("released slot must be reusable: %v", err)
	}
}

// Post-forward failures are uncertain for the caller: exactly one upstream
// commit followed by (a) a connection reset and (b) a truncated response body
// must each surface OUTCOME UNKNOWN with method and id, never a plain
// "service unreachable"/"read" error.
func TestSpoolCommitThenResponseFailureIsOutcomeUnknown(t *testing.T) {
	spool := t.TempDir()
	var commits atomic.Int32
	var mode atomic.Value
	mode.Store("reset")
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		commits.Add(1)
		switch mode.Load().(string) {
		case "reset":
			if hj, ok := w.(http.Hijacker); ok {
				c, _, _ := hj.Hijack()
				_ = c.Close() // commit happened, connection dies before any response
				return
			}
		case "truncate":
			w.Header().Set("Content-Length", "1000")
			_, _ = w.Write([]byte(`{"result":`)) // shorter than declared → unexpected EOF
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
			if hj, ok := w.(http.Hijacker); ok {
				c, _, _ := hj.Hijack()
				_ = c.Close()
			}
		}
	}))
	t.Cleanup(svc.Close)
	rl := &Relay{Spool: spool, Base: svc.URL, Token: "t", Log: slog.New(slog.NewTextHandler(&strings.Builder{}, nil)), Client: svc.Client(), now: time.Now}
	lane := filepath.Join(spool, "lane-u")
	for _, d := range []string{"req", "res"} {
		if err := os.MkdirAll(filepath.Join(lane, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for i, m := range []string{"reset", "truncate"} {
		mode.Store(m)
		id := fmt.Sprintf("1-%d-%s", i+1, m)
		if err := writeAtomic(filepath.Join(lane, "req", id+".json"), spoolRequest{Method: "SendGuarded", Headers: map[string]string{}, Body: []byte(`{"args":[]}`)}, 0o600); err != nil {
			t.Fatal(err)
		}
		rl.serveOnce()
		b, _ := os.ReadFile(filepath.Join(lane, "res", id+".json"))
		var sr spoolResponse
		if json.Unmarshal(b, &sr) != nil || sr.Status != http.StatusBadGateway || !strings.Contains(string(sr.Body), "OUTCOME UNKNOWN") || !strings.Contains(string(sr.Body), "SendGuarded id "+id) {
			t.Fatalf("%s: want OUTCOME UNKNOWN naming method+id, got status=%d body=%s", m, sr.Status, sr.Body)
		}
	}
	if commits.Load() != 2 {
		t.Fatalf("each request must reach upstream exactly once: %d", commits.Load())
	}
}

// Recovery reads lane-authored in-flight files bounded; an oversized or
// malformed one still yields a correlated outcome-unknown, no allocation
// beyond the envelope bound, and no forward.
func TestSpoolRecoveryIsBoundedAndCorrelated(t *testing.T) {
	spool := t.TempDir()
	var calls atomic.Int32
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1) }))
	t.Cleanup(svc.Close)
	rl := &Relay{Spool: spool, Base: svc.URL, Token: "t", Log: slog.New(slog.NewTextHandler(&strings.Builder{}, nil)), Client: svc.Client(), now: time.Now}
	lane := filepath.Join(spool, "lane-r")
	if err := os.MkdirAll(filepath.Join(lane, "inflight"), 0o700); err != nil {
		t.Fatal(err)
	}
	// Oversized: envelope bound + 1 MiB of garbage; malformed: not JSON.
	big := make([]byte, spoolMaxEnvelope+1<<20)
	if err := os.WriteFile(filepath.Join(lane, "inflight", "1-1-big.json"), big, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lane, "inflight", "1-2-bad.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if n := rl.recoverInflight(); n != 2 {
		t.Fatalf("recovered %d", n)
	}
	for _, id := range []string{"1-1-big", "1-2-bad"} {
		b, _ := os.ReadFile(filepath.Join(lane, "res", id+".json"))
		var sr spoolResponse
		if json.Unmarshal(b, &sr) != nil || !strings.Contains(string(sr.Body), "OUTCOME UNKNOWN") || !strings.Contains(string(sr.Body), "? "+id) {
			t.Fatalf("%s: want correlated outcome-unknown, got %s", id, b)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("recovery must never forward")
	}
	if left, _ := filepath.Glob(filepath.Join(lane, "inflight", "*")); len(left) != 0 {
		t.Fatalf("in-flight files must be removed: %v", left)
	}
}

// A response file that exists but is unusable (malformed JSON here) means the
// request was consumed and forwarded: the lane must see OUTCOME UNKNOWN with
// method and id, never a plain parse error.
func TestSpoolCorruptResponseIsOutcomeUnknown(t *testing.T) {
	spool := t.TempDir()
	tr := newSpoolTransport(spool, "lane-k")
	tr.wait = 2 * time.Second
	go func() { // a "relay" that consumes and answers with garbage
		for i := 0; i < 100; i++ {
			files, _ := filepath.Glob(filepath.Join(spool, "lane-k", "req", "*.json"))
			for _, f := range files {
				id := strings.TrimSuffix(filepath.Base(f), ".json")
				_ = os.MkdirAll(filepath.Join(spool, "lane-k", "res"), 0o700)
				_ = os.Remove(f)
				_ = os.WriteFile(filepath.Join(spool, "lane-k", "res", id+".json"), []byte("{garbage"), 0o600)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	req, _ := http.NewRequest(http.MethodPost, "http://spool/v1/call/SendGuarded", strings.NewReader(`{"args":[]}`))
	_, err := tr.RoundTrip(req)
	if err == nil || !strings.Contains(err.Error(), "OUTCOME UNKNOWN") || !strings.Contains(err.Error(), "SendGuarded id ") || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("corrupt response must be outcome-unknown naming method+id: %v", err)
	}
}

// Lanes never wait on each other: with lane-slow holding the upstream for
// 1.5 s, a request that ARRIVES afterwards on lane-fast is answered in well
// under that time, and slow still completes.
func TestSpoolRelayNewLaneIsNotBlockedBySlowLane(t *testing.T) {
	spool := t.TempDir()
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Sirsi-Session") == "slow" {
			time.Sleep(1500 * time.Millisecond)
		}
		_, _ = w.Write([]byte(`{"result":[]}`))
	}))
	t.Cleanup(svc.Close)
	rl := &Relay{Spool: spool, Base: svc.URL, Token: "t", Log: slog.New(slog.NewTextHandler(&strings.Builder{}, nil)), Client: svc.Client(), now: time.Now}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = rl.Serve(ctx) }()
	for _, lane := range []string{"lane-slow", "lane-fast"} {
		if err := os.MkdirAll(filepath.Join(spool, lane, "req"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeAtomic(filepath.Join(spool, "lane-slow", "req", "1-1-s.json"), spoolRequest{Method: "ListAll", Headers: map[string]string{"X-Sirsi-Session": "slow"}, Body: []byte(`{"args":[]}`)}, 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond) // slow is now held upstream
	start := time.Now()
	if err := writeAtomic(filepath.Join(spool, "lane-fast", "req", "1-1-f.json"), spoolRequest{Method: "ListAll", Headers: map[string]string{}, Body: []byte(`{"args":[]}`)}, 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(1200 * time.Millisecond)
	for {
		if _, err := os.Stat(filepath.Join(spool, "lane-fast", "res", "1-1-f.json")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fast lane blocked behind the slow lane")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if el := time.Since(start); el > 1000*time.Millisecond {
		t.Fatalf("fast lane took %s", el)
	}
	deadline = time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(spool, "lane-slow", "res", "1-1-s.json")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("slow lane never completed")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A backlog inside one lane must not spin the discovery loop, and cancellation
// must return promptly while that backlog is still pending.
func TestSpoolRelayBacklogDoesNotSpinAndCancels(t *testing.T) {
	spool := t.TempDir()
	release := make(chan struct{})
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { <-release; _, _ = w.Write([]byte(`{"result":[]}`)) }))
	t.Cleanup(svc.Close)
	rl := &Relay{Spool: spool, Base: svc.URL, Token: "t", Log: slog.New(slog.NewTextHandler(&strings.Builder{}, nil)), Client: svc.Client(), now: time.Now}
	if err := os.MkdirAll(filepath.Join(spool, "lane-q", "req"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"1-1-a", "1-2-b"} { // two pending in the SAME lane
		if err := writeAtomic(filepath.Join(spool, "lane-q", "req", id+".json"), spoolRequest{Method: "ListAll", Headers: map[string]string{}, Body: []byte(`{"args":[]}`)}, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = rl.Serve(ctx); close(done) }()
	time.Sleep(600 * time.Millisecond) // first request held upstream, second queued behind it
	if n := rl.scans.Load(); n > 8 {
		t.Fatalf("discovery spun: %d scans in 600 ms with a %s poll", n, spoolPoll)
	}
	start := time.Now()
	cancel()
	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Serve did not return within 300 ms of cancellation while a backlog was pending")
	}
	_ = start
	close(release) // let the held worker finish before TempDir cleanup
	time.Sleep(100 * time.Millisecond)
}

// TestRelayHTTPClientDialsFreshNoKeepAlive proves ONLY that the relay's forward
// client opens a new connection per request (no keep-alive reuse): three
// sequential requests open three NEW server connections. That mitigates the
// SUSPECTED wedge where a long-lived relay's pooled HTTPS connection goes
// half-open (e.g. macOS idle/sleep) and hangs the next forward — this test does
// not itself reproduce that idle/sleep causality.
func TestRelayHTTPClientDialsFreshNoKeepAlive(t *testing.T) {
	var newConns atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Config.ConnState = func(_ net.Conn, st http.ConnState) {
		if st == http.StateNew {
			newConns.Add(1)
		}
	}
	srv.Start()
	defer srv.Close()

	c := newRelayHTTPClient()
	for i := 0; i < 3; i++ {
		resp, err := c.Get(srv.URL)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	if got := newConns.Load(); got != 3 {
		t.Fatalf("relay client must dial fresh each forward (no keep-alive reuse): new connections=%d, want 3", got)
	}
}

// TestRelayHTTPClientPreservesProxy (rs-30 review): disabling keep-alives must
// NOT drop proxy resolution. Cloning the default transport preserves its Proxy
// function (ProxyFromEnvironment); a zero-value Transport would have Proxy==nil
// and silently bypass HTTPS_PROXY. (ProxyFromEnvironment caches the env once per
// process, so this asserts the function is preserved rather than resolving a
// runtime-set proxy, which t.Setenv cannot reach.)
func TestRelayHTTPClientPreservesProxy(t *testing.T) {
	tr, ok := newRelayHTTPClient().Transport.(*http.Transport)
	if !ok {
		t.Fatalf("relay transport = %T, want *http.Transport", newRelayHTTPClient().Transport)
	}
	if !tr.DisableKeepAlives {
		t.Fatal("relay transport must keep DisableKeepAlives=true")
	}
	if tr.Proxy == nil {
		t.Fatal("relay transport dropped its Proxy function — a configured HTTPS_PROXY would be bypassed; clone the default transport")
	}
}

// TestLaneDirModeDefaultsUnchanged: with SIRSI_RELAY_TRUST_GROUP unset (every
// existing deployment, and every other test in this file), resolvedLaneModes
// must return exactly the single-uid defaults — the widened mode is opt-in
// only.
func TestLaneDirModeDefaultsUnchanged(t *testing.T) {
	t.Setenv("SIRSI_RELAY_TRUST_GROUP", "")
	dirMode, fileMode, gid, err := resolvedLaneModes()
	if err != nil {
		t.Fatalf("resolvedLaneModes() with no trust group: %v", err)
	}
	if dirMode != 0o700 || fileMode != 0o600 || gid != -1 {
		t.Fatalf("resolvedLaneModes() with no trust group = (%o,%o,%d), want (0700,0600,-1)", dirMode, fileMode, gid)
	}
}

// TestResolvedLaneModesUnknownGroupFailsClosed (SSA review, PR #753): a
// misconfigured or typo'd SIRSI_RELAY_TRUST_GROUP must be a hard error, never
// silently treated as "any non-empty string widens permissions" — the first
// version of this feature did exactly that, so a typo'd group name still
// produced group-writable directories inherited from whatever group the
// parent's setgid bit happened to carry, verified against nothing.
func TestResolvedLaneModesUnknownGroupFailsClosed(t *testing.T) {
	t.Setenv("SIRSI_RELAY_TRUST_GROUP", "sirsi-nonexistent-group-xyz")
	if _, _, _, err := resolvedLaneModes(); err == nil {
		t.Fatal("unresolvable trust group must be refused, not silently ignored or silently trusted")
	}
}

// TestMkdirTrustedRefusesToWidenAMismatchedExistingGroup (SSA review, PR
// #753): an existing directory whose ACTUAL group does not match the
// resolved trust group must never be widened — that would grant group-write
// to whatever unrelated group the directory happens to carry (e.g. it
// predates the spool's setgid bit) rather than the specific, verified trust
// group the operator configured.
func TestMkdirTrustedRefusesToWidenAMismatchedExistingGroup(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "lane")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("cannot read raw stat_t on this platform")
	}
	actualGID := int(sys.Gid)
	wrongGID := actualGID + 1 // guaranteed not to match
	if merr := mkdirTrusted(target, 0o770, wrongGID); merr == nil {
		t.Fatal("mkdirTrusted must refuse to widen a directory whose actual group does not match the configured trust group")
	}
	st2, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if st2.Mode().Perm() != 0o700 {
		t.Fatalf("mode was changed despite the group mismatch: now %o, want untouched at 0700", st2.Mode().Perm())
	}
	// Negative control on the test itself: the CORRECT gid must succeed.
	if err := mkdirTrusted(target, 0o770, actualGID); err != nil {
		t.Fatalf("mkdirTrusted with the CORRECT gid must succeed: %v", err)
	}
}

// TestLaneCreatesGroupWritableDirsWhenTrustGroupConfigured: a lane's own
// RoundTrip call creates req/res/slots at 0770, not 0700, when
// SIRSI_RELAY_TRUST_GROUP is set in ITS OWN environment — proving the actual
// directories a real round trip creates get the widened mode, not just the
// helper function in isolation. This is the fix for the exact bug that shipped
// first: a relay running under a separate service account locked out of a
// lane's freshly created directory because the lane never knew to leave room
// for it.
func TestLaneCreatesGroupWritableDirsWhenTrustGroupConfigured(t *testing.T) {
	// Must be a REAL resolvable group now that resolvedLaneModes fails closed
	// on an unresolvable name (the fix for TestResolvedLaneModesUnknownGroupFailsClosed
	// above) — reuse the current process's own primary group, which any fresh
	// t.TempDir() subdirectory will actually carry, so the gid-match check in
	// mkdirTrusted passes honestly rather than being bypassed by trustGID<0.
	trustGroup, terr := currentPrimaryGroupName()
	if terr != nil {
		t.Skipf("cannot resolve current primary group: %v", terr)
	}
	t.Setenv("SIRSI_RELAY_TRUST_GROUP", trustGroup)
	spool := t.TempDir()
	tr := newSpoolTransport(spool, "trust-mode-lane")
	req := httptest.NewRequest(http.MethodPost, "http://spool/v1/call/Status", nil)
	go func() { _, _ = tr.RoundTrip(req) }() // will time out waiting for a response; only the mkdir matters here
	deadline := time.Now().Add(2 * time.Second)
	var st os.FileInfo
	for time.Now().Before(deadline) {
		if s, err := os.Stat(filepath.Join(spool, "trust-mode-lane", "req")); err == nil {
			st = s
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if st == nil {
		t.Fatal("req dir was never created")
	}
	if st.Mode().Perm() != 0o770 {
		t.Fatalf("req dir mode = %o, want 0770 when SIRSI_RELAY_TRUST_GROUP is set", st.Mode().Perm())
	}
}

// TestWriteAtomicModeSurvivesUmask: writeAtomic's mode parameter must be the
// file's ACTUAL final mode, not merely the mode requested at creation time —
// WriteFile's mode, like MkdirAll's, is masked by the process umask exactly
// like open(2)/creat(2). A umask of the common 022 would silently turn a
// requested 0640 into 0640 unaffected (022 only strips write bits, and 0640
// has none to strip) — but proving the ACTUAL bytes-on-disk mode, not just
// trusting the requested value, is what actually matters here: this is the
// exact bug class (request mode != actual mode) that broke the directory side
// of this feature, so the file side gets the same explicit proof.
func TestWriteAtomicModeSurvivesUmask(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.json")
	if err := writeAtomic(path, spoolResponse{Status: 200}, 0o640); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o640 {
		t.Fatalf("writeAtomic mode = %o, want exactly 0640 regardless of umask", st.Mode().Perm())
	}
}

// TestLaneFileModeDefaultsUnchanged: with SIRSI_RELAY_TRUST_GROUP unset, lane
// request files stay 0600 — the historical default, readable only by their
// own writer. This is the fix for the actual deployed bug: a directory being
// group-writable does NOT make files inside it group-readable, so a relay
// running under a different uid than its lane clients could see a response
// file exist (via directory listing) yet be refused reading its content.
func TestLaneFileModeDefaultsUnchanged(t *testing.T) {
	t.Setenv("SIRSI_RELAY_TRUST_GROUP", "")
	if _, fileMode, _, err := resolvedLaneModes(); err != nil || fileMode != 0o600 {
		t.Fatalf("resolvedLaneModes() with no trust group: fileMode=%o err=%v, want 0600 nil", fileMode, err)
	}
	trustGroup, terr := currentPrimaryGroupName()
	if terr != nil {
		t.Skipf("cannot resolve current primary group: %v", terr)
	}
	t.Setenv("SIRSI_RELAY_TRUST_GROUP", trustGroup)
	if _, fileMode, gid, err := resolvedLaneModes(); err != nil || fileMode != 0o640 || gid < 0 {
		t.Fatalf("resolvedLaneModes() with trust group set: fileMode=%o gid=%d err=%v, want 0640 >=0 nil", fileMode, gid, err)
	}
}

// TestSpoolRelayEndToEndWithTrustGroupWritesReadableFiles: the actual
// production bug, reproduced and fixed. Without SIRSI_RELAY_TRUST_GROUP set,
// this is identical to TestSpoolRelayEndToEnd; WITH it set (simulating a lane
// and a relay that are different processes potentially running as different
// uids, though this test cannot fork a real second uid — see the package
// doc on CheckSpoolDirTrustingGroup for that boundary), every file the relay
// and the lane exchange must be at least 0640, not the 0600 default that
// made the real deployment's response files unreadable across uids even
// though their containing directories were correctly group-writable.
func TestSpoolRelayEndToEndWithTrustGroupWritesReadableFiles(t *testing.T) {
	// The relay's own TrustGroup must resolve via user.LookupGroup (unlike the
	// client side, which only checks its env var is non-empty) — reuse the
	// current real primary group so Serve() doesn't fail closed on a
	// nonexistent name and silently exit before ever consuming anything.
	trustGroup, err := currentPrimaryGroupName()
	if err != nil {
		t.Skipf("cannot resolve current primary group: %v", err)
	}
	t.Setenv("SIRSI_RELAY_TRUST_GROUP", trustGroup)
	backend := newDst(t)
	h, herr := Handler(backend, ServerOptions{Token: "h0st", MaxWait: 3 * time.Second})
	if herr != nil {
		t.Fatal(herr)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	spool := t.TempDir()
	logBuf := &strings.Builder{}
	rl := &Relay{Spool: spool, Base: srv.URL, Token: "h0st", TrustGroup: trustGroup, Log: slog.New(slog.NewTextHandler(logBuf, nil))}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		if serveErr := rl.Serve(ctx); serveErr != nil {
			t.Logf("relay.Serve exited: %v", serveErr)
		}
	}()

	t.Setenv("SIRSI_AGENT_ID", "lane-trust")
	t.Setenv("HOME", t.TempDir()) // session cache dir
	rs := NewRemoteStore("spool://"+spool, "")
	if _, err := rs.ListAll(context.Background()); err != nil {
		t.Fatalf("ListAll over trust-group spool: %v\nrelay log:\n%s", err, logBuf.String())
	}
	// Request and response files are deliberately self-cleaning on a
	// successful round trip (TestSpoolRelayEndToEnd's own comment: "Files are
	// cleaned up after each round trip") — nothing survives to inspect here
	// by design, so THIS test's job is only proving the round trip still
	// succeeds end-to-end with a trust group engaged (mode 0640 is not
	// silently breaking anything). The actual mode-correctness claim is
	// proved directly by TestWriteAtomicModeSurvivesUmask and
	// TestLaneFileModeDefaultsUnchanged, which inspect files before cleanup
	// removes them.
}

// TestPublishNeverAttemptsToChmodAnExistingResDir is the exact production
// bug, reproduced directly: publish() must NOT try to modify the mode of an
// already-existing res/ directory. In the real deployment this directory is
// created by the LANE CLIENT, not the relay; when the relay runs under a
// different uid (a dedicated service account), a chmod on a directory it
// does not own fails with EPERM — and the original code's
// "if mkdirErr == nil { write }" structure swallowed that failure with zero
// log output, so requests were silently forwarded successfully while their
// responses vanished into nothing. This test proves publish() leaves an
// existing res/ directory's mode completely untouched (proving no chmod
// attempt happens against it) and still successfully writes the response.
func TestPublishNeverAttemptsToChmodAnExistingResDir(t *testing.T) {
	spool := t.TempDir()
	resDir := filepath.Join(spool, "lane-p", "res")
	if err := os.MkdirAll(resDir, 0o750); err != nil { // an unusual, distinctive mode
		t.Fatal(err)
	}
	trustGroup, terr := currentPrimaryGroupName()
	if terr != nil {
		t.Skipf("cannot resolve current primary group: %v", terr)
	}
	rl := &Relay{Spool: spool, TrustGroup: trustGroup, Log: slog.Default()}
	rl.publish("lane-p", "id1", spoolResponse{Status: 200, Body: []byte("{}")})

	st, err := os.Stat(resDir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o750 {
		t.Fatalf("publish() modified an existing res/ dir's mode: now %o, want untouched at 0750 — it must never chmod a directory it may not own", st.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(resDir, "id1.json")); err != nil {
		t.Fatalf("publish() must still write the response into the existing directory: %v", err)
	}
}

// TestPublishLogsWhenItCannotCreateResDir: the fresh-creation fallback path
// must never fail silently — an unlogged failure here is exactly what let a
// forwarded-and-succeeded request vanish with no response and no trace in
// the real incident this fix responds to.
func TestPublishLogsWhenItCannotCreateResDir(t *testing.T) {
	lockedSpool := t.TempDir()
	if err := os.Chmod(lockedSpool, 0o500); err != nil { // no write bit: MkdirAll below must fail
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(lockedSpool, 0o700) }) // let t.TempDir() clean up
	logBuf := &strings.Builder{}
	rl := &Relay{Spool: lockedSpool, Log: slog.New(slog.NewTextHandler(logBuf, nil))}
	rl.publish("lane-locked", "id1", spoolResponse{Status: 200})

	if _, err := os.Stat(filepath.Join(lockedSpool, "lane-locked", "res", "id1.json")); !os.IsNotExist(err) {
		t.Fatalf("response must not exist when its directory could not be created: %v", err)
	}
	if l := logBuf.String(); !strings.Contains(l, "create response dir") {
		t.Fatalf("a failed resDir creation must be logged, not swallowed silently:\n%s", l)
	}
}

// TestPublishFallbackCreatesGroupAccessibleResDir (SSA review, PR #753): the
// FRESH-creation branch of publish() — res/ genuinely does not exist yet —
// must create it at the widened mode (0770) when a trust group is
// configured, not the plain single-uid 0700 the first version of this fix
// hardcoded unconditionally. That version fixed the "chmod on an existing,
// unowned directory" bug correctly but, in doing so, dropped trust-awareness
// from the fresh-creation path entirely — the one path where widening is
// actually safe, since the relay just created (and therefore owns) it. The
// result was the fallback path becoming the one thing a trust-group-enabled
// client could never read.
func TestPublishFallbackCreatesGroupAccessibleResDir(t *testing.T) {
	trustGroup, terr := currentPrimaryGroupName()
	if terr != nil {
		t.Skipf("cannot resolve current primary group: %v", terr)
	}
	spool := t.TempDir() // fresh: lane-fresh/res does NOT exist yet
	rl := &Relay{Spool: spool, TrustGroup: trustGroup, Log: slog.Default()}
	rl.publish("lane-fresh", "id1", spoolResponse{Status: 200, Body: []byte("{}")})

	resDir := filepath.Join(spool, "lane-fresh", "res")
	st, err := os.Stat(resDir)
	if err != nil {
		t.Fatalf("publish() must create res/ on the fresh path: %v", err)
	}
	if st.Mode().Perm() != 0o770 {
		t.Fatalf("freshly created res/ mode = %o, want 0770 under a configured trust group — the client must be able to read it", st.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(resDir, "id1.json")); err != nil {
		t.Fatalf("response file must exist: %v", err)
	}
}
