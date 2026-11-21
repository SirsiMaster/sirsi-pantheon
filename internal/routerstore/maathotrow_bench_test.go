package routerstore

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

// TestMaatReserveLatencyHotRow measures one Reserve through the real Ledger
// path against a reservations document of ~8,000 finished rows (~4 MB, the live
// size on 2026-11-11) plus 20 active rows, before and after the archive moves
// the finished rows out. Timing only: opt in with SIRSI_BENCH_HOTROW=1.
func TestMaatReserveLatencyHotRow(t *testing.T) {
	if os.Getenv("SIRSI_BENCH_HOTROW") == "" {
		t.Skip("set SIRSI_BENCH_HOTROW=1 to run the timing")
	}
	s := openBackendStore(t, filepath.Join(t.TempDir(), "bench.db"))
	defer s.Close()
	now := s.clock().UTC()
	pad := strings.Repeat("x", 250) // ~506 B/row, as live
	build := func(endedAgo time.Duration, spreadMin int) []map[string]any {
		var rows []map[string]any
		for i := 0; i < 8000; i++ {
			startAgo := endedAgo + time.Duration(i%spreadMin)*time.Minute + 5*time.Minute
			r := hotRowRow(now, "released", startAgo, endedAgo+time.Duration(i%spreadMin)*time.Minute, map[string]any{"work": pad, "exit_code": 0})
			r["id"] = fmt.Sprintf("%s-%d", r["id"], i)
			rows = append(rows, r)
		}
		for i := 0; i < 20; i++ {
			r := hotRowRow(now, "active", time.Duration(i+1)*time.Minute, -1, map[string]any{"resource": fmt.Sprintf("rail-%d", i), "holder": fmt.Sprintf("lane-%d", i)})
			r["est_end"] = ts(now.Add(time.Hour))
			r["heartbeat_at"] = ts(now)
			r["lease_ttl_sec"] = 3600
			rows = append(rows, r)
		}
		return rows
	}
	timeReserves := func(label string) {
		var d []time.Duration
		for i := 0; i < 15; i++ {
			t0 := time.Now()
			if _, err := s.MaatReserve(schedule.Reservation{Resource: fmt.Sprintf("m1-%s-%d", label, i), Holder: "bench", Regime: schedule.RegimeBuild, Start: ts(time.Now()), EstEnd: ts(time.Now().Add(time.Hour))}, false); err != nil {
				t.Fatal(err)
			}
			d = append(d, time.Since(t0))
		}
		sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
		raw, _, _ := s.GetState(maatReservationsKey)
		t.Logf("%-7s hot doc %7d B   Reserve p50 %v  p90 %v  max %v", label, len(raw), d[len(d)/2], d[len(d)*9/10], d[len(d)-1])
	}

	// BEFORE: every finished row ended 10 minutes or less ago, so all stay hot,
	// the same document the service rewrites today.
	seedMaatRows(t, s, build(time.Minute, 100)...)
	timeReserves("before")

	// AFTER: the same rows ended 3h+ ago; the first call moves them out.
	seedMaatRows(t, s, build(3*time.Hour, 2400)...)
	t0 := time.Now()
	if _, err := s.MaatRead("status", "resource-none"); err != nil {
		t.Fatal(err)
	}
	t.Logf("one-time archive pass moving 8000 rows: %v", time.Since(t0))
	timeReserves("after")

	// Steady state: 20 more rows age out into the already-large day rows.
	aged := build(3*time.Hour, 2400)
	steady := append(aged[:19:19], aged[8000:]...)
	steady = append(steady, hotRowRow(now, "released", 3*time.Hour+time.Minute, 2*time.Hour+time.Minute, map[string]any{"id": "steady-0"}))
	seedMaatRows(t, s, steady...)
	t0 = time.Now()
	if _, err := s.MaatRead("status", "resource-none"); err != nil {
		t.Fatal(err)
	}
	t.Logf("archive pass, steady (moves rows into existing day rows): %v", time.Since(t0))

	// The cost of the id fallback on a hit and on a miss.
	t0 = time.Now()
	rows, err := s.MaatRead("reservation", "steady-0")
	t.Logf("exact-id fallback hit: %v (rows=%d err=%v)", time.Since(t0), len(rows), err)
	t0 = time.Now()
	_, _ = s.MaatRead("reservation", "20261111T000000Z-nope")
	t.Logf("exact-id fallback miss: %v", time.Since(t0))
}
