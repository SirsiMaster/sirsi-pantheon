package routerstore

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	calls := 0
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; _, _ = w.Write([]byte(`{"result":[]}`)) }))
	t.Cleanup(svc.Close)
	lane := filepath.Join(spool, "lane-z")
	for _, d := range []string{"req", "inflight", "res"} {
		if err := os.MkdirAll(filepath.Join(lane, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// A previous relay consumed this Send and died before publishing.
	if err := writeAtomic(filepath.Join(lane, "inflight", "1-1-dead.json"), spoolRequest{Method: "SendGuarded", Headers: map[string]string{}, Body: []byte(`{"args":[]}`)}); err != nil {
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
	if calls != 0 {
		t.Fatalf("restart must never re-forward: %d calls", calls)
	}
	if _, err := os.Stat(filepath.Join(lane, "inflight", "1-1-dead.json")); !os.IsNotExist(err) {
		t.Fatal("in-flight file must be removed after reporting")
	}
	// Consume happens before forward: a request that vanishes between glob and
	// rename (another relay took it) is not forwarded.
	if err := writeAtomic(filepath.Join(lane, "req", "1-2-live.json"), spoolRequest{Method: "ListAll", Headers: map[string]string{}, Body: []byte(`{"args":[]}`)}); err != nil {
		t.Fatal(err)
	}
	if n := rl.serveOnce(); n != 1 || calls != 1 {
		t.Fatalf("live request: handled=%d calls=%d", n, calls)
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

// A cancelled or short-deadline context (RemoteStore's own 5 s per call) after
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
		t.Fatalf("unconsumed + cancelled must be withdrawn and named: %v", err)
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
		t.Fatalf("consumed + cancelled must be outcome-unknown naming method, id and cause: %v", err)
	}
}

// Bounds are enforced on the DECODED body, not the base64 envelope: 4 MiB minus
// one byte is forwarded; 4 MiB plus one byte is refused with 413 before any
// forward.
func TestSpoolBodyBoundaryOnDecodedBytes(t *testing.T) {
	spool := t.TempDir()
	calls := 0
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
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
	if err := writeAtomic(filepath.Join(lane, "req", "1-1-ok.json"), spoolRequest{Method: "ListAll", Headers: map[string]string{}, Body: big}); err != nil {
		t.Fatal(err)
	}
	rl.serveOnce()
	b, _ := os.ReadFile(filepath.Join(lane, "res", "1-1-ok.json"))
	var sr spoolResponse
	if json.Unmarshal(b, &sr) != nil || sr.Status != 200 || calls != 1 {
		t.Fatalf("4 MiB-1 must be forwarded: status=%d calls=%d", sr.Status, calls)
	}
	over := bytes.Repeat([]byte("x"), spoolMaxBody+1)
	if err := writeAtomic(filepath.Join(lane, "req", "1-2-over.json"), spoolRequest{Method: "ListAll", Headers: map[string]string{}, Body: over}); err != nil {
		t.Fatal(err)
	}
	rl.serveOnce()
	b, _ = os.ReadFile(filepath.Join(lane, "res", "1-2-over.json"))
	if json.Unmarshal(b, &sr) != nil || sr.Status != http.StatusRequestEntityTooLarge || calls != 1 {
		t.Fatalf("4 MiB+1 must be refused with 413 before forwarding: status=%d calls=%d", sr.Status, calls)
	}
}

// A request whose consumption fails (inflight/ is not writable) is never
// forwarded and stays where it is.
func TestSpoolFailedConsumeIsNotForwarded(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores modes")
	}
	spool := t.TempDir()
	calls := 0
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; _, _ = w.Write([]byte(`{"result":[]}`)) }))
	t.Cleanup(svc.Close)
	rl := &Relay{Spool: spool, Base: svc.URL, Token: "t", Log: slog.New(slog.NewTextHandler(&strings.Builder{}, nil)), Client: svc.Client(), now: time.Now}
	lane := filepath.Join(spool, "lane-f")
	for _, d := range []string{"req", "inflight"} {
		if err := os.MkdirAll(filepath.Join(lane, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeAtomic(filepath.Join(lane, "req", "1-1-x.json"), spoolRequest{Method: "SendGuarded", Headers: map[string]string{}, Body: []byte(`{"args":[]}`)}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(lane, "inflight"), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(lane, "inflight"), 0o700) })
	if n := rl.serveOnce(); n != 0 || calls != 0 {
		t.Fatalf("failed consume must not forward: handled=%d calls=%d", n, calls)
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
