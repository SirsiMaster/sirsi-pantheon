package schedule

import (
	"strings"
	"testing"
)

func TestStudioAliasHistoricalPool(t *testing.T) {
	for _, first := range []string{"studio", "m5-studio"} {
		t.Run(first, func(t *testing.T) {
			other := "studio"
			if first == other {
				other = "m5-studio"
			}
			l := fixedLedger("2026-09-24T10:00:00Z")
			req := mkReq(first, "old", "2026-09-24T10:00:00Z", "2026-09-24T10:30:00Z", RegimeQuiet)
			a, err := l.Reserve(req, false)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{first, other} {
				row, err := l.WhoIsOn(name)
				if err != nil || row == nil || row.Resource != first {
					t.Fatalf("who %s: %+v %v", name, row, err)
				}
				rows, err := l.LiveHolders(name)
				if err != nil || len(rows) != 1 {
					t.Fatalf("holders %s: %v %v", name, rows, err)
				}
				cov, err := l.Coverage(name, 1)
				if err != nil || cov.Resource != name || cov.Current == nil || cov.Current.Resource != first {
					t.Fatalf("coverage: %+v %v", cov, err)
				}
			}
			req.Resource = other
			req.Holder = "new"
			b, err := l.Reserve(req, false)
			if err != nil || b.Reservation.Share != ShareFloor || b.Reservation.Resource != other {
				t.Fatalf("floor %+v %v", b, err)
			}
			req.Holder = "queued"
			q, err := l.Reserve(req, true)
			if err != nil || q.Reservation.Status != StatusQueued {
				t.Fatalf("queue %+v %v", q, err)
			}
			for _, name := range []string{"studio", "m5-studio"} {
				rows, err := l.Status(name)
				if err != nil || len(rows) != 3 {
					t.Fatalf("status %v %v", rows, err)
				}
			}
			for _, name := range []string{"rail@studio", "rail@m5-studio", "studio-gpu", "m5", "Studio"} {
				if sameResource(first, name) {
					t.Fatalf("unexpected alias %s", name)
				}
			}
			c, err := l.RequestCede(Cede{Resource: first, Requester: "ask", Holder: "old", Ask: "machine", Minutes: 1, Reason: "test"})
			if err != nil {
				t.Fatal(err)
			}
			rows, err := l.ListCedes(CedeFilter{Resource: other})
			if err != nil || len(rows) != 1 || rows[0].Resource != first || rows[0].ID != c.ID {
				t.Fatalf("cedes %v %v", rows, err)
			}
			if _, err = l.Release(a.Reservation.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStudioAliasCapacity(t *testing.T) {
	for _, name := range []string{"studio", "m5-studio"} {
		l := fixedLedger("2026-09-24T10:00:00Z")
		if err := l.SetCapacity(name, 16); err != nil {
			t.Fatal(err)
		}
		if err := l.SetCapacityMemGB(name, 64); err != nil {
			t.Fatal(err)
		}
		for _, read := range []string{"studio", "m5-studio"} {
			if got, err := l.Capacity(read); err != nil || got != 16 {
				t.Fatalf("capacity %d %v", got, err)
			}
			if got, err := l.MemoryCapacityGB(read); err != nil || got != 64 {
				t.Fatalf("memory %d %v", got, err)
			}
		}
	}
	for _, m := range []map[string]int{{"m5-studio": 16}, {"studio": 16}, {"studio": 16, "m5-studio": 24}} {
		for _, name := range []string{"studio", "m5-studio"} {
			if poolCapacity(m, name) != 16 {
				t.Fatal(m, name)
			}
		}
	}
}

func TestStudioAliasStoredHistoryAndConflict(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	raw := `{"future":"retain","reservations":[{"id":"legacy","resource":"m5-studio","holder":"old","regime":"quiet","share":"full","status":"active","start":"2026-09-24T10:00:00Z","est_end":"2026-09-24T10:30:00Z","heartbeat_at":"2026-09-24T10:00:00Z","lease_ttl_sec":120,"future":"row-retain"}]}`
	if err := l.store.SetState(stateKey, raw); err != nil {
		t.Fatal(err)
	}
	SetActivityProbe(func(string) ([]Actor, error) { return nil, nil })
	defer SetActivityProbe(nil)
	rep, err := l.CheckConflicts("studio", "m5-studio")
	if err != nil || rep.Holder != "old" {
		t.Fatalf("conflict %+v %v", rep, err)
	}
	for _, name := range []string{"rail@studio", "rail@m5-studio", "studio-gpu", "m5"} {
		out, err := l.Reserve(mkReq(name, "new", "2026-09-24T10:00:00Z", "2026-09-24T10:30:00Z", RegimeBuild), false)
		if err != nil || out.Reservation.Share != ShareFull {
			t.Fatalf("unrelated %+v %v", out, err)
		}
	}
	rows, err := l.Status("studio")
	if err != nil || len(rows) != 1 || rows[0].Resource != "m5-studio" || rows[0].ID != "legacy" {
		t.Fatalf("history %+v %v", rows, err)
	}
	stored, _, err := l.store.GetState(stateKey)
	if err != nil || !strings.Contains(stored, `"future":"row-retain"`) || !strings.Contains(stored, `"future":"retain"`) {
		t.Fatalf("history lost %s %v", stored, err)
	}
}
