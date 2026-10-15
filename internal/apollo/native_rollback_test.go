package apollo

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

func TestSSARejectedRankLeavesNoQueuedReservation(t *testing.T) {
	a, req := nativeAuthorityFixture(t)
	foreign, err := a.ledger.Reserve(schedule.Reservation{Resource: "mem.m1", Holder: "foreign", Work: "other", Regime: schedule.RegimeLoaded, Start: a.now().Format(time.RFC3339), EstEnd: req.ExpiresAt.Format(time.RFC3339), LeaseTTLSec: 90}, false)
	if err != nil || foreign.Reservation == nil {
		t.Fatalf("fixture: %v", err)
	}
	if _, err = a.Admit(context.Background(), req); err == nil {
		t.Fatal("contended admission accepted")
	}
	foreignFound := false
	for _, resource := range []string{"mem.m5", "mem.m1"} {
		rows, err := a.ledger.Status(resource)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if row.ID == foreign.Reservation.ID && row.Status == schedule.StatusActive {
				foreignFound = true
			}
			if row.Work == "apollo-native:"+req.ExecutionID && (row.Status == schedule.StatusActive || row.Status == schedule.StatusQueued) {
				t.Fatalf("rejected admission left actionable reservation: resource=%s status=%s id=%s", resource, row.Status, row.ID)
			}
		}
	}
	if !foreignFound {
		t.Fatal("foreign active hold changed")
	}
}

// Every refusal must clean up both earlier grants and the current returned row.
func TestNativeAdmissionRollbackOutcomes(t *testing.T) {
	for _, mode := range []string{"queued", "floor", "reserve-error", "release-error", "save-error"} {
		t.Run(mode, func(t *testing.T) {
			a, req := nativeAuthorityFixture(t)
			ledger := a.ledger
			fault := &rollbackLedger{nativeReservationLedger: ledger, mode: mode}
			a.ledger = fault
			_, err := a.Admit(context.Background(), req)
			if err == nil {
				t.Fatal("refused admission succeeded")
			}
			if mode == "release-error" {
				if !strings.Contains(err.Error(), "cleanup unavailable") || len(fault.released) != 2 {
					t.Fatalf("cleanup errors/attempts lost: %v %v", err, fault.released)
				}
				return
			}
			if mode == "save-error" && !strings.Contains(err.Error(), "persistence unavailable") {
				t.Fatalf("persistence failure lost: %v", err)
			}
			for _, rank := range req.Ranks {
				rows, readErr := ledger.Status(rank.Resource)
				if readErr != nil {
					t.Fatal(readErr)
				}
				for _, row := range rows {
					if row.Work == "apollo-native:"+req.ExecutionID && (row.Status == schedule.StatusActive || row.Status == schedule.StatusQueued) {
						t.Fatalf("actionable row left after %s: %#v", mode, row)
					}
				}
			}
		})
	}
}

type rollbackLedger struct {
	nativeReservationLedger
	mode     string
	calls    int
	released []string
}

func (l *rollbackLedger) Reserve(req schedule.Reservation, queue bool) (schedule.ReserveResult, error) {
	l.calls++
	if l.calls == 2 && l.mode == "save-error" {
		return schedule.ReserveResult{}, fmt.Errorf("persistence unavailable")
	}
	result, err := l.nativeReservationLedger.Reserve(req, queue)
	if l.calls != 2 || err != nil {
		return result, err
	}
	switch l.mode {
	case "queued", "release-error":
		result.Granted = false
		result.Queued = true
	case "floor":
		result.Reservation.Share = schedule.ShareFloor
	case "reserve-error":
		return result, fmt.Errorf("reserve response unavailable")
	}
	return result, nil
}
func (l *rollbackLedger) Release(id string) (*schedule.Reservation, error) {
	l.released = append(l.released, id)
	if l.mode == "release-error" {
		return nil, fmt.Errorf("cleanup unavailable: %s", id)
	}
	return l.nativeReservationLedger.Release(id)
}
