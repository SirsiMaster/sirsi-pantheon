package dashboard

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
)

func TestFleetReadinessRefreshPreservesHistory(t *testing.T) {
	ft := NewFleetTracker(map[string]bool{"lane": true})
	now := time.Now()
	ft.Observe(snapOf("lane", task("one", "pending")), now)
	got := ft.ObserveWithRoutability(snapOf("lane", task("one", "in-progress")), now, map[string]bool{"lane": false})
	if len(got.Lanes) != 1 || !got.Lanes[0].Routable || len(got.Activity) != 1 {
		t.Fatalf("fresh readiness/history lost: %+v", got)
	}
	got = ft.ObserveWithRoutability(snapOf("lane", task("one", "in-progress")), now, map[string]bool{"lane": true})
	if got.Lanes[0].Routable || len(got.Activity) != 1 {
		t.Fatalf("stale readiness/history lost: %+v", got)
	}
}

func TestFleetReadinessFailureIsUnavailable(t *testing.T) {
	for _, result := range []struct {
		value map[string]bool
		err   error
	}{{nil, errors.New("offline")}, {nil, nil}} {
		s := New(Config{FleetFn: func() (ledger.Snapshot, error) { return snapOf("lane", task("one", "pending")), nil }, RoutabilityFn: func() (map[string]bool, error) { return result.value, result.err }})
		w := httptest.NewRecorder()
		s.apiFleet(w, httptest.NewRequest("GET", "/api/fleet", nil))
		if w.Code != 502 {
			t.Fatalf("unavailable readiness returned %d: %s", w.Code, w.Body.String())
		}
	}
}

func TestFleetReadinessUnknownLaneIsNotHealthy(t *testing.T) {
	got := NewFleetTracker(nil).ObserveWithRoutability(snapOf("undeclared", task("one", "pending")), time.Now(), map[string]bool{"declared": false})
	if len(got.Lanes) != 1 || got.Lanes[0].Routable {
		t.Fatalf("unknown lane treated as routable: %+v", got)
	}
}
