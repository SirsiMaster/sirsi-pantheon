package schedule

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const (
	floorNow  = "2026-10-22T10:00:00Z"
	floorEnd  = "2026-10-22T10:30:00Z"
	guideText = "book through the pod authority"
)

// delegatedLedger returns a ledger with "studio" delegated to pod studio-m5.
func delegatedLedger(t *testing.T) (*Ledger, Delegation) {
	t.Helper()
	l := fixedLedger(floorNow)
	d, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio", floorNow, "2026-10-22T18:00:00Z", "studio"))
	if err != nil {
		t.Fatal(err)
	}
	return l, d
}

// authorityRow seeds a pod-authority row directly (as ReconcileAuthority would).
func authorityRow(t *testing.T, l *Ledger, d Delegation, regime Regime, edit func(*Reservation)) {
	t.Helper()
	r := mkReq("studio", "mercury", floorNow, floorEnd, regime)
	r.ID, r.Authority, r.Epoch, r.Seq = "auth-hold", "pod:"+d.Pod, d.Epoch, 1
	r.Status, r.Share = StatusActive, ShareFull
	r.Created, r.HeartbeatAt = floorNow, floorNow
	if edit != nil {
		edit(&r)
	}
	if _, err := l.UpsertReservation(r); err != nil {
		t.Fatal(err)
	}
}

func TestReserve_DelegatedResourceGrantsFloor(t *testing.T) {
	for _, tc := range []struct {
		name         string
		hold         bool // seed an authority BUILD hold
		wantConflict bool
	}{
		{"no authority rows", false, false},
		{"beside authority build hold", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, d := delegatedLedger(t)
			if tc.hold {
				authorityRow(t, l, d, RegimeBuild, nil)
			}
			floor, err := l.FloorShare("studio")
			if err != nil {
				t.Fatal(err)
			}
			res, err := l.Reserve(mkReq("studio", "claude-io", floorNow, floorEnd, RegimeBuild), false)
			if err != nil {
				t.Fatalf("a non-authority caller must get the floor, not a refusal: %v", err)
			}
			got := res.Reservation
			if !res.Granted || got == nil || got.Status != StatusActive || got.Share != ShareFloor || got.Cores != floor {
				t.Fatalf("want an active floor grant (cores %d), got %+v", floor, res)
			}
			if got.Authority != "" || got.Epoch != 0 || got.Seq != 0 {
				t.Fatalf("floor row must not carry authority stamps: %+v", got)
			}
			if !strings.Contains(got.Reason, "delegated to pod studio-m5") || !strings.Contains(got.Reason, guideText) {
				t.Fatalf("reason should explain the delegation: %q", got.Reason)
			}
			if (res.Conflict != nil) != tc.wantConflict {
				t.Fatalf("conflict = %+v, want present=%v", res.Conflict, tc.wantConflict)
			}
			rows, err := l.Status("studio")
			if err != nil {
				t.Fatal(err)
			}
			var found bool
			for _, r := range rows {
				found = found || r.ID == got.ID
			}
			if !found {
				t.Fatalf("floor row not persisted: %+v", rows)
			}
		})
	}
}

func TestReserve_DelegatedResourceRefusals(t *testing.T) {
	for _, tc := range []struct {
		name   string
		setup  func(*testing.T, *Ledger, Delegation)
		now    string // optional clock override
		regime Regime
		queue  bool
		want   string // distinguishing error substring
	}{
		{"1a expired inside handback margin", nil, "2026-10-22T18:02:00Z", RegimeBuild, false, "handback margin"},
		{"1a revoked", func(t *testing.T, l *Ledger, _ Delegation) {
			if _, err := l.Revoke("studio-m5"); err != nil {
				t.Fatal(err)
			}
		}, "", RegimeBuild, false, "handback margin"},
		{"1b quiet request", nil, "", RegimeQuiet, false, "quiet window cannot be promised"},
		{"1c queue", nil, "", RegimeBuild, true, "cannot queue"},
		{"1d active quiet hold", func(t *testing.T, l *Ledger, d Delegation) { authorityRow(t, l, d, RegimeQuiet, nil) }, "", RegimeBuild, false, "protects the window"},
		{"1d active loaded hold", func(t *testing.T, l *Ledger, d Delegation) { authorityRow(t, l, d, RegimeLoaded, nil) }, "", RegimeBuild, false, "protects the window"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, d := delegatedLedger(t)
			if tc.setup != nil {
				tc.setup(t, l, d)
			}
			if tc.now != "" {
				now := at(tc.now)
				l.now = func() time.Time { return now }
			}
			_, err := l.Reserve(mkReq("studio", "claude-io", floorNow, floorEnd, tc.regime), tc.queue)
			if err == nil {
				t.Fatal("want a refusal")
			}
			if !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), guideText) {
				t.Fatalf("wrong refusal: %v", err)
			}
			rows, _ := l.Status("studio")
			for _, r := range rows {
				if r.Holder == "claude-io" {
					t.Fatalf("a refusal must save nothing: %+v", r)
				}
			}
		})
	}
}

func TestReserve_DelegatedHoldDoesNotProtect(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Reservation)
	}{
		{"lease-expired quiet hold", func(r *Reservation) { r.HeartbeatAt = "2026-10-22T09:00:00Z" }},
		{"released quiet hold", func(r *Reservation) { r.Status = StatusReleased }},
		{"non-overlapping window", func(r *Reservation) { r.Start, r.EstEnd = "2026-10-22T12:00:00Z", "2026-10-22T12:30:00Z" }},
		{"stale-epoch quiet hold", func(r *Reservation) { r.Epoch++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, d := delegatedLedger(t)
			authorityRow(t, l, d, RegimeQuiet, tc.edit)
			res, err := l.Reserve(mkReq("studio", "claude-io", floorNow, floorEnd, RegimeBuild), false)
			if err != nil || !res.Granted || res.Reservation.Share != ShareFloor {
				t.Fatalf("hold must not protect the window; want floor grant, got %+v err=%v", res, err)
			}
		})
	}
}

func TestLegacyWrite_DelegatedResourceOnlyAdmitsFloorRow(t *testing.T) {
	l, d := delegatedLedger(t)
	floor, err := l.FloorShare("studio")
	if err != nil {
		t.Fatal(err)
	}
	row := func(edit func(*Reservation)) Reservation {
		r := mkReq("studio", "claude-io", floorNow, floorEnd, RegimeBuild)
		r.ID, r.Status, r.Share, r.Cores = "legacy-1", StatusActive, ShareFloor, floor
		r.Created, r.HeartbeatAt = floorNow, floorNow
		if edit != nil {
			edit(&r)
		}
		return r
	}
	for _, tc := range []struct {
		name    string
		r       Reservation
		wantErr bool
	}{
		{"floor row", row(nil), false},
		{"full share", row(func(r *Reservation) { r.Share = ShareFull }), true},
		{"unbounded floor", row(func(r *Reservation) { r.Cores = 0 }), true},
		{"over-floor cores", row(func(r *Reservation) { r.Cores = floor + 1 }), true},
		{"authority stamped", row(func(r *Reservation) { r.Authority = "pod:" + d.Pod; r.Epoch = d.Epoch }), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(map[string][]Reservation{"reservations": {tc.r}})
			err := l.CheckLegacyReservationWrite("", string(raw))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
	t.Run("floor row once delegation expired", func(t *testing.T) {
		l.now = func() time.Time { return at("2026-10-22T18:02:00Z") }
		raw, _ := json.Marshal(map[string][]Reservation{"reservations": {row(nil)}})
		if err := l.CheckLegacyReservationWrite("", string(raw)); err == nil {
			t.Fatal("an expired delegation must not admit legacy floor rows")
		}
	})
	t.Run("full row cannot replace an existing floor row", func(t *testing.T) {
		l.now = func() time.Time { return at(floorNow) }
		old, _ := json.Marshal(map[string][]Reservation{"reservations": {row(nil)}})
		full, _ := json.Marshal(map[string][]Reservation{"reservations": {row(func(r *Reservation) { r.Share = ShareFull })}})
		if err := l.CheckLegacyReservationWrite(string(old), string(full)); err == nil {
			t.Fatal("floor row upgraded to full on a delegated resource")
		}
	})
}

// A holder that finished must be able to hand its floor row back even after
// the delegation ends (handback margin); capacity returns, nothing else moves.
func TestLegacyWrite_FloorRowReleaseInHandbackMargin(t *testing.T) {
	floorRow := func(l *Ledger, edit func(*Reservation)) Reservation {
		floor, err := l.FloorShare("studio")
		if err != nil {
			t.Fatal(err)
		}
		r := mkReq("studio", "claude-io", floorNow, floorEnd, RegimeBuild)
		r.ID, r.Status, r.Share, r.Cores = "legacy-1", StatusActive, ShareFloor, floor
		r.Created, r.HeartbeatAt = floorNow, floorNow
		if edit != nil {
			edit(&r)
		}
		return r
	}
	doc := func(rows ...Reservation) string {
		raw, _ := json.Marshal(map[string][]Reservation{"reservations": rows})
		return string(raw)
	}
	code := 0
	phases := []struct {
		name   string
		margin bool
		setup  func(*testing.T, *Ledger)
	}{
		{"live", false, func(*testing.T, *Ledger) {}},
		{"expired by time, inside margin", true, func(_ *testing.T, l *Ledger) {
			l.now = func() time.Time { return at("2026-10-22T18:02:00Z") }
		}},
		{"revoked", true, func(t *testing.T, l *Ledger) {
			if _, err := l.Revoke("studio-m5"); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, p := range phases {
		t.Run(p.name, func(t *testing.T) {
			l, d := delegatedLedger(t)
			p.setup(t, l)
			stamp := func(r *Reservation) { r.Authority, r.Epoch, r.Seq = "pod:"+d.Pod, d.Epoch, 1 }
			active := doc(floorRow(l, nil))
			released := doc(floorRow(l, func(r *Reservation) { r.Status = StatusReleased }))
			for _, tc := range []struct {
				name             string
				oldRaw, newRaw   string
				refusedInMargin  bool
				refusedWhileLive bool
			}{
				{"release", active, released, false, false},
				{"release with exit code", active, doc(floorRow(l, func(r *Reservation) { r.Status = StatusReleased; r.ExitCode = &code })), false, false},
				{"done", active, doc(floorRow(l, func(r *Reservation) { r.Status = StatusDone })), false, false},
				{"expired projection", active, doc(floorRow(l, func(r *Reservation) { r.Status = StatusExpired })), false, false},
				{"invalidated", active, doc(floorRow(l, func(r *Reservation) { r.Status = StatusInvalidated; r.InvalidatedBy = "x" })), false, false},
				{"release but over-floor cores", active, doc(floorRow(l, func(r *Reservation) { r.Status = StatusReleased; r.Cores += 9 })), true, true},
				{"release but window changed", active, doc(floorRow(l, func(r *Reservation) { r.Status = StatusReleased; r.EstEnd = "2026-10-22T11:00:00Z" })), true, false},
				{"release but holder changed", active, doc(floorRow(l, func(r *Reservation) { r.Status = StatusReleased; r.Holder = "other" })), true, false},
				{"heartbeat", active, doc(floorRow(l, func(r *Reservation) { r.HeartbeatAt = "2026-10-22T10:01:00Z" })), true, false},
				{"extend", active, doc(floorRow(l, func(r *Reservation) { r.EstEnd = "2026-10-22T11:00:00Z" })), true, false},
				{"share upgrade", active, doc(floorRow(l, func(r *Reservation) { r.Share = ShareFull })), true, true},
				{"new floor row", active, doc(floorRow(l, nil), floorRow(l, func(r *Reservation) { r.ID = "legacy-2" })), true, false},
				{"removal", active, doc(), true, true},
				{"resurrect released row", released, active, true, true},
				{"authority-stamped release", doc(floorRow(l, stamp)), doc(floorRow(l, func(r *Reservation) { stamp(r); r.Status = StatusReleased })), true, true},
			} {
				want := tc.refusedWhileLive
				if p.margin {
					want = tc.refusedInMargin
				}
				if err := l.CheckLegacyReservationWrite(tc.oldRaw, tc.newRaw); (err != nil) != want {
					t.Errorf("%s: err=%v wantRefused=%v", tc.name, err, want)
				}
			}
		})
	}
}
