package schedule

import "testing"

func TestLoadedAdmissionAfterBoundedCPUFloor(t *testing.T) {
	for _, tc := range []struct {
		name     string
		change   func(*Reservation)
		incoming Regime
		want     string
	}{
		{"bounded CPU build", func(*Reservation) {}, RegimeLoaded, ShareFull},
		{"unbounded CPU", func(r *Reservation) { r.Cores = 0 }, RegimeLoaded, ShareFloor},
		{"oversized CPU", func(r *Reservation) { r.Cores = 99 }, RegimeLoaded, ShareFloor},
		{"loaded floor", func(r *Reservation) { r.Regime = RegimeLoaded }, RegimeLoaded, ShareFloor},
		{"quiet floor", func(r *Reservation) { r.Regime = RegimeQuiet }, RegimeLoaded, ShareFloor},
		{"full build", func(r *Reservation) { r.Share = ShareFull }, RegimeLoaded, ShareFloor},
		{"unknown share", func(r *Reservation) { r.Share = "unknown" }, RegimeLoaded, ShareFloor},
		{"memory ask", func(r *Reservation) { r.MemGB = 1 }, RegimeLoaded, ShareFloor},
		{"quiet incoming still waits", func(*Reservation) {}, RegimeQuiet, ShareFloor},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := fixedLedger("2026-10-18T23:00:00Z")
			r := mkReq("m5", "cpu", "2026-10-18T22:59:00Z", "2026-10-18T23:10:00Z", RegimeBuild)
			r.ID, r.Status, r.Share, r.Cores = "cpu-row", StatusActive, ShareFloor, 2
			r.HeartbeatAt = "2026-10-18T23:00:00Z"
			tc.change(&r)
			if err := l.save("", false, []Reservation{r}); err != nil {
				t.Fatal(err)
			}
			out, err := l.Reserve(mkReq("m5", "model", "2026-10-18T23:00:00Z", "2026-10-18T23:05:00Z", tc.incoming), false)
			if err != nil {
				t.Fatal(err)
			}
			if out.Reservation.Share != tc.want {
				t.Fatalf("got %s, want %s: %+v", out.Reservation.Share, tc.want, out)
			}
		})
	}
}
