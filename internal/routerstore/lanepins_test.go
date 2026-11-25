package routerstore

// Schema v30: operator-set lane-to-Mac pins and the REPORT-ONLY verdict counted at a served Wait.
// Every refusal or no-effect claim sits beside a positive control. Synthetic names only. Runs on SQLite and, under
// SIRSI_TEST_PG_DSN, on Postgres (openBackendStore).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// pinProbeStore counts and can fail/panic the evaluator's one hot-path read.
type pinProbeStore struct {
	Store
	calls atomic.Int64
	mode  atomic.Int32 // 0 real, 1 error, 2 panic
}

// mintSessionForToken forwards the token-attributed mint the handler type-asserts for.
func (p *pinProbeStore) mintSessionForToken(tokenID, host, agent, runtimeHash, threadID string) (Session, error) {
	return p.Store.(*SQLiteStore).mintSessionForToken(tokenID, host, agent, runtimeHash, threadID)
}

func (p *pinProbeStore) ActiveLanePinMachines(agent string) ([]string, error) {
	p.calls.Add(1)
	switch p.mode.Load() {
	case 1:
		return nil, errors.New("synthetic pin read failure")
	case 2:
		panic("synthetic pin read panic")
	}
	return p.Store.ActiveLanePinMachines(agent)
}

type pinHarness struct {
	backend *SQLiteStore
	probe   *pinProbeStore
	srv     *httptest.Server
	s       *server
	nowNs   atomic.Int64
}

func (h *pinHarness) now() time.Time { return time.Unix(0, h.nowNs.Load()).UTC() }

func newPinHarness(t *testing.T) *pinHarness {
	t.Helper()
	h := &pinHarness{}
	h.nowNs.Store(time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC).UnixNano())
	h.backend = openBackendStore(t, filepath.Join(t.TempDir(), "router.db"))
	t.Cleanup(func() { _ = h.backend.Close() })
	h.backend.notifyDir = t.TempDir()
	h.probe = &pinProbeStore{Store: h.backend}
	handler, s, err := newHandler(h.probe, ServerOptions{Token: "host-token", MaxWait: 2 * time.Second, now: h.now})
	if err != nil {
		t.Fatal(err)
	}
	h.s = s
	h.srv = httptest.NewServer(handler)
	t.Cleanup(h.srv.Close)
	return h
}

// hostClient authenticates with a minted host token for host (optionally adopting machineID first).
func (h *pinHarness) hostClient(t *testing.T, host, machineID string) *RemoteStore {
	t.Helper()
	plain, _, err := h.backend.MintHostToken(host, "")
	if err != nil {
		t.Fatal(err)
	}
	if machineID != "" {
		if err := h.backend.AdoptTokenMachineID(host, machineID); err != nil {
			t.Fatal(err)
		}
	}
	rs := NewRemoteStore(h.srv.URL, plain)
	rs.sessionDir = ""
	rs.host, rs.agent = host, "caller-"+host
	rs.now = h.now
	return rs
}

func (h *pinHarness) wait(t *testing.T, rs *RemoteStore, lane string) bool {
	t.Helper()
	woke, err := rs.Wait(context.Background(), lane, 30*time.Millisecond)
	if err != nil {
		t.Fatalf("Wait(%s): %v", lane, err)
	}
	return woke
}

func (h *pinHarness) count(lane string, v LanePinVerdict) uint64 {
	return h.s.pins.snapshot()[lane+"|"+string(v)]
}

func TestEvaluateLanePinVerdicts(t *testing.T) {
	cases := []struct {
		name   string
		caller string
		pins   []string
		want   LanePinVerdict
	}{
		{"match", "mach-a", []string{"mach-a"}, LanePinMatch},
		{"match among several pins", "mach-b", []string{"mach-a", "mach-b"}, LanePinMatch},
		{"mismatch", "mach-c", []string{"mach-a", "mach-b"}, LanePinMismatch},
		{"unpinned with a machine id", "mach-a", nil, LanePinUnpinned},
		{"unpinned without a machine id", "", nil, LanePinUnpinned},
		{"pinned lane, token has no machine id", "", []string{"mach-a"}, LanePinNoMachineID},
	}
	for _, c := range cases {
		if got := EvaluateLanePin(c.caller, c.pins); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestLanePinStoreSemantics(t *testing.T) {
	s := openBackendStore(t, filepath.Join(t.TempDir(), "router.db"))
	t.Cleanup(func() { _ = s.Close() })
	p1, err := s.SetLanePin("lane-a", "mach-1", "ra", "primary")
	if err != nil || p1.Created == "" || p1.Retired != "" {
		t.Fatalf("set: %+v %v", p1, err)
	}
	// idempotent: the original row (set_by, reason, created) is returned, nothing is added
	again, err := s.SetLanePin("lane-a", "mach-1", "someone-else", "other")
	if err != nil || again.SetBy != "ra" || again.Reason != "primary" || again.Created != p1.Created {
		t.Fatalf("idempotent set must keep the original: %+v %v", again, err)
	}
	// a lane may hold several active pins
	if _, err := s.SetLanePin("lane-a", "mach-2", "ra", ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ActiveLanePinMachines("lane-a"); strings.Join(got, ",") != "mach-1,mach-2" {
		t.Fatalf("active pins = %v, want both machines", got)
	}
	// retire keeps the row; a retired pin is ignored by the evaluator's read
	if err := s.RetireLanePin("lane-a", "mach-1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ActiveLanePinMachines("lane-a"); strings.Join(got, ",") != "mach-2" {
		t.Fatalf("retired pin must be ignored: %v", got)
	}
	all, _ := s.ListLanePins(true)
	live, _ := s.ListLanePins(false)
	if len(all) != 2 || len(live) != 1 || all[0].Retired == "" && all[1].Retired == "" {
		t.Fatalf("history must be kept: all=%+v live=%+v", all, live)
	}
	// retiring again, or an unknown pin, is ErrLanePinUnknown (and changes nothing)
	if err := s.RetireLanePin("lane-a", "mach-1"); !errors.Is(err, ErrLanePinUnknown) {
		t.Fatalf("second retire: %v", err)
	}
	if err := s.RetireLanePin("lane-zzz", "mach-1"); !errors.Is(err, ErrLanePinUnknown) {
		t.Fatalf("unknown retire: %v", err)
	}
	// a retired pin does not block re-pinning, and history grows rather than being overwritten
	if _, err := s.SetLanePin("lane-a", "mach-1", "ra", "back"); err != nil {
		t.Fatalf("re-pin after retire: %v", err)
	}
	if all, _ = s.ListLanePins(true); len(all) != 3 {
		t.Fatalf("history rows = %d, want 3", len(all))
	}
	// other lanes are untouched
	if got, _ := s.ActiveLanePinMachines("lane-b"); len(got) != 0 {
		t.Fatalf("lane-b leaked pins: %v", got)
	}
	// invalid input is refused (negative controls beside the valid sets above)
	for _, bad := range [][4]string{
		{"", "m", "ra", ""}, {"a", "", "ra", ""}, {"a", "m", "", ""},
		{"has space", "m", "ra", ""}, {"a", "m\tx", "ra", ""}, {strings.Repeat("x", lanePinMaxIdent+1), "m", "ra", ""},
		{"a", "m", "ra", strings.Repeat("r", lanePinMaxReason+1)},
	} {
		if _, err := s.SetLanePin(bad[0], bad[1], bad[2], bad[3]); !errors.Is(err, ErrLanePinInvalid) {
			t.Errorf("SetLanePin(%q) = %v, want ErrLanePinInvalid", bad, err)
		}
	}
}

// The partial unique index is the backstop behind SetLanePin's read-then-insert: two ACTIVE rows for one
// (agent, machine) must be impossible even for a racing writer.
func TestLanePinActiveRowIsUniqueAtTheDatabase(t *testing.T) {
	s := openBackendStore(t, filepath.Join(t.TempDir(), "router.db"))
	t.Cleanup(func() { _ = s.Close() })
	ins := func(created, retired string) error {
		_, err := s.db.Exec(`INSERT INTO lane_machine_pins(agent,machine_id,set_by,reason,created,retired) VALUES('l','m','ra','',?,?)`, created, retired)
		return err
	}
	if err := ins("c1", ""); err != nil {
		t.Fatal(err)
	}
	if err := ins("c2", ""); err == nil {
		t.Fatal("a second active row for the same (agent, machine) must be refused")
	}
	if err := ins("c3", "2026-11-25T00:00:00Z"); err != nil {
		t.Fatalf("a retired row must not collide with the active one: %v", err)
	}
}

// Lanes cannot set, retire or read pins: not over HTTP (any token), not through the relay.
func TestLanePinOperationsAreNeitherServedNorRelayed(t *testing.T) {
	h := newPinHarness(t)
	lane := h.hostClient(t, "m1", "mach-lane")
	bodies := map[string]string{
		"SetLanePin":            `{"args":["lane-x","mach-evil","lane","self-assigned"]}`,
		"RetireLanePin":         `{"args":["lane-x","mach-lane"]}`,
		"ListLanePins":          `{"args":[true]}`,
		"ActiveLanePinMachines": `{"args":["lane-x"]}`,
	}
	if _, err := h.backend.SetLanePin("lane-x", "mach-lane", "ra", "operator"); err != nil {
		t.Fatal(err)
	}
	before, _ := h.backend.ListLanePins(true)
	for method, body := range bodies {
		for _, tok := range []string{"host-token", lane.token} {
			req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/v1/call/"+method, bytes.NewReader([]byte(body)))
			req.Header.Set("Authorization", "Bearer "+tok)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("%s over the wire = %d, want 404", method, resp.StatusCode)
			}
		}
		if !notServed[method] || !spoolRefused[method] {
			t.Errorf("%s must be in notServed and spoolRefused", method)
		}
	}
	after, _ := h.backend.ListLanePins(true)
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Fatalf("a refused call changed the pins: %v -> %v", before, after)
	}
	// through the relay: refused by name, never forwarded
	spool := t.TempDir()
	rl := &Relay{Spool: spool, Base: h.srv.URL, Token: "host-token", Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = rl.Serve(ctx) }()
	t.Setenv("SIRSI_AGENT_ID", "lane-x")
	t.Setenv("HOME", t.TempDir())
	rs := NewRemoteStore("spool://"+spool, "")
	if _, err := rs.SetLanePin("lane-x", "mach-evil", "lane", ""); err == nil || !strings.Contains(err.Error(), "never forwarded") {
		t.Fatalf("SetLanePin through the relay must be refused: %v", err)
	}
	if err := rs.RetireLanePin("lane-x", "mach-lane"); err == nil || !strings.Contains(err.Error(), "never forwarded") {
		t.Fatalf("RetireLanePin through the relay must be refused: %v", err)
	}
	if final, _ := h.backend.ListLanePins(true); fmt.Sprint(final) != fmt.Sprint(before) {
		t.Fatalf("a relayed attempt changed the pins: %v", final)
	}
}

// The verdicts are counted at a served Wait from the TOKEN's machine id, and the Wait outcome is identical for
// every verdict: a mismatch changes nothing.
func TestLanePinReportOnlyVerdictsAtServedWait(t *testing.T) {
	h := newPinHarness(t)
	if _, err := h.backend.SetLanePin("lane-x", "mach-a", "ra", ""); err != nil {
		t.Fatal(err)
	}
	match := h.hostClient(t, "m1", "mach-a")
	mismatch := h.hostClient(t, "m2", "mach-b")
	nomid := h.hostClient(t, "m3", "")
	boot := NewRemoteStore(h.srv.URL, "host-token")
	boot.sessionDir, boot.agent, boot.now = "", "boot", h.now

	// no work waiting: every caller sees the same (false) outcome
	for name, rs := range map[string]*RemoteStore{"match": match, "mismatch": mismatch, "no-machine-id": nomid, "bootstrap": boot} {
		if h.wait(t, rs, "lane-x") {
			t.Fatalf("%s: Wait woke with an empty inbox", name)
		}
	}
	// work waiting: every caller sees the same (true) outcome, mismatch included
	if _, err := h.backend.Send("sender", "lane-x", "t", "review", "i"); err != nil {
		t.Fatal(err)
	}
	for name, rs := range map[string]*RemoteStore{"match": match, "mismatch": mismatch, "no-machine-id": nomid, "bootstrap": boot} {
		if !h.wait(t, rs, "lane-x") {
			t.Fatalf("%s: Wait must still wake for waiting work", name)
		}
		if items, err := rs.Inbox("lane-x"); err != nil || len(items) != 1 {
			t.Fatalf("%s: Inbox changed: %v %v", name, items, err)
		}
	}
	if got := h.count("lane-x", LanePinMatch); got != 2 {
		t.Errorf("match count = %d, want 2", got)
	}
	if got := h.count("lane-x", LanePinMismatch); got != 2 {
		t.Errorf("mismatch count = %d, want 2", got)
	}
	if got := h.count("lane-x", LanePinNoMachineID); got != 4 { // m3 token and the bootstrap token, twice each
		t.Errorf("no-machine-id count = %d, want 4", got)
	}
	// a lane with no pin: unpinned, under the single shared label
	h.wait(t, match, "lane-free")
	if got := h.count(lanePinUnpinnedLbl, LanePinUnpinned); got != 1 {
		t.Errorf("unpinned count = %d, want 1", got)
	}
	// nothing but Wait counts: an Inbox call by the mismatching token adds no verdict
	snap := fmt.Sprint(h.s.pins.snapshot())
	if _, err := mismatch.Inbox("lane-x"); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(h.s.pins.snapshot()) != snap {
		t.Error("only a served Wait may count a verdict")
	}
}

// The seam exists and is closed: no verdict refuses today.
func TestLanePinEnforcementSeamIsOffForEveryVerdict(t *testing.T) {
	h := newPinHarness(t)
	for _, v := range []LanePinVerdict{LanePinMatch, LanePinMismatch, LanePinUnpinned, LanePinNoMachineID, lanePinError, lanePinSkipped} {
		if h.s.lanePinRefuses(v) {
			t.Errorf("report-only: %q must not refuse", v)
		}
	}
	if _, err := h.backend.SetLanePin("lane-x", "mach-a", "ra", ""); err != nil {
		t.Fatal(err)
	}
	v, refuse := h.s.observeLanePin("lane-x", "mach-other")
	if v != LanePinMismatch || refuse {
		t.Fatalf("mismatch must be observed and not refused: %q refuse=%v", v, refuse)
	}
}

// A retired pin is ignored (after the cache window); the cache bounds the hot-path cost.
func TestLanePinRetiredIgnoredAndCacheBoundsQueries(t *testing.T) {
	h := newPinHarness(t)
	if _, err := h.backend.SetLanePin("lane-x", "mach-a", "ra", ""); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		if v, _ := h.s.observeLanePin("lane-x", "mach-a"); v != LanePinMatch {
			t.Fatalf("verdict %q", v)
		}
	}
	if n := h.probe.calls.Load(); n != 1 {
		t.Fatalf("50 observations inside the TTL cost %d reads, want 1", n)
	}
	if err := h.backend.RetireLanePin("lane-x", "mach-a"); err != nil {
		t.Fatal(err)
	}
	if v, _ := h.s.observeLanePin("lane-x", "mach-a"); v != LanePinMatch {
		t.Fatalf("within the cache window the old verdict stands, got %q", v) // documented lag
	}
	h.nowNs.Add(int64(lanePinCacheTTL + time.Second))
	if v, _ := h.s.observeLanePin("lane-x", "mach-a"); v != LanePinUnpinned {
		t.Fatalf("retired pin must be ignored once re-read, got %q", v)
	}
	if n := h.probe.calls.Load(); n != 2 {
		t.Fatalf("reads = %d, want 2 (one per TTL)", n)
	}
}

// A failure to evaluate (error or panic) is counted and never fails or alters the request.
func TestLanePinEvaluationFailureChangesNothing(t *testing.T) {
	h := newPinHarness(t)
	rs := h.hostClient(t, "m1", "mach-a")
	if _, err := h.backend.Send("sender", "lane-x", "t", "review", "i"); err != nil {
		t.Fatal(err)
	}
	if !h.wait(t, rs, "lane-x") { // positive control with a healthy evaluator
		t.Fatal("control: Wait must wake")
	}
	for mode, name := range map[int32]string{1: "error", 2: "panic"} {
		h.probe.mode.Store(mode)
		h.nowNs.Add(int64(lanePinCacheTTL + time.Second)) // expire the cache so the read happens
		if !h.wait(t, rs, "lane-x") {
			t.Fatalf("%s in the evaluator changed the Wait outcome", name)
		}
	}
	if got := h.count(lanePinUnpinnedLbl, lanePinError); got < 1 {
		t.Errorf("a failed read must be counted, got %d", got)
	}
	// and the failed read is cached briefly, so a sick database is not hammered
	h.probe.mode.Store(1)
	h.nowNs.Add(int64(lanePinCacheTTL + time.Second))
	h.probe.calls.Store(0)
	for i := 0; i < 20; i++ {
		h.s.observeLanePin("lane-x", "mach-a")
	}
	if n := h.probe.calls.Load(); n != 1 {
		t.Errorf("failed reads inside the error window = %d, want 1", n)
	}
}

// Counters and the cache are bounded however many lane names a caller invents.
func TestLanePinCountersAndCacheAreBounded(t *testing.T) {
	h := newPinHarness(t)
	for i := 0; i < 3; i++ {
		if _, err := h.backend.SetLanePin(fmt.Sprintf("real-%d", i), "mach-a", "ra", ""); err != nil {
			t.Fatal(err)
		}
		h.s.observeLanePin(fmt.Sprintf("real-%d", i), "mach-a")
	}
	for i := 0; i < 5000; i++ {
		h.s.observeLanePin(fmt.Sprintf("invented-%d", i), "mach-a")
	}
	h.s.observeLanePin(strings.Repeat("x", 4096), "mach-a") // an unusable name is ignored, not stored
	snap := h.s.pins.snapshot()
	if len(snap) > 3+4 { // three pinned lanes + a handful of aggregate labels
		t.Fatalf("counter keys = %d (%v); invented lane names must not mint keys", len(snap), snap)
	}
	if snap[lanePinUnpinnedLbl+"|"+string(LanePinUnpinned)] == 0 || snap[lanePinOverflowLbl+"|"+string(lanePinSkipped)] == 0 {
		t.Fatalf("unpinned and overflow aggregates must absorb the invented names: %v", snap)
	}
	if n := len(h.s.pins.cache); n > lanePinCacheMax {
		t.Fatalf("cache entries = %d, cap %d", n, lanePinCacheMax)
	}
	// a pinned lane still evaluates correctly after the flood once entries expire
	h.nowNs.Add(int64(lanePinCacheTTL + time.Second))
	if v, _ := h.s.observeLanePin("real-1", "mach-a"); v != LanePinMatch {
		t.Fatalf("pinned lane after flood: %q", v)
	}
}

// A v29 ledger upgrades in place: the table appears empty (no backfill, no lane is pinned by the upgrade).
func TestV29ToV30AddsLanePinsEmpty(t *testing.T) {
	if pgTestDSN() != "" {
		t.Skip("SQLite upgrade path; the Postgres upgrade is scripts/router-service/test-apply-schema-upgrade.sh")
	}
	path := filepath.Join(t.TempDir(), "v29.db")
	s, err := OpenPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DROP INDEX idx_lane_machine_pins_active; DROP TABLE lane_machine_pins; PRAGMA user_version=29;`); err != nil {
		t.Fatalf("rewind to v29: %v", err)
	}
	_ = s.Close()
	s2, err := OpenPath(path)
	if err != nil {
		t.Fatalf("v29 to v30 upgrade must apply cleanly: %v", err)
	}
	defer func() { _ = s2.Close() }()
	if v, e := ReadSchemaVersion(path); e != nil || v != 30 {
		t.Fatalf("post-upgrade version = %d (%v), want 30", v, e)
	}
	if pins, err := s2.ListLanePins(true); err != nil || len(pins) != 0 {
		t.Fatalf("upgrade must not pin any lane: %v %v", pins, err)
	}
}
