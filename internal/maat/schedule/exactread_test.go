package schedule

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestExactReservationBoundsAndPodRefusal(t *testing.T) {
	s := newFakeStore()
	b, _ := json.Marshal(map[string]any{"reservations": []Reservation{{ID: "huge", Work: strings.Repeat("x", 65536), Status: StatusReleased}}})
	s.kv[stateKey] = string(b)
	if _, e := NewLedger(s).ExactReservation("huge"); e == nil {
		t.Fatal("oversized row accepted")
	}
	if s.kv[stateKey] != string(b) {
		t.Fatal("read wrote state")
	}
	p := &PodRoutingStore{StateStore: s}
	if _, e := NewLedger(p).ExactReservation("pod:p:1:id"); e == nil || !strings.Contains(e.Error(), "fallback refused") {
		t.Fatal(e)
	}
}

type exactOldServer struct {
	*fakeStore
	calls int
}

func (s *exactOldServer) MaatReserve(Reservation, bool) (ReserveResult, error)    { panic("mutation") }
func (s *exactOldServer) MaatMutate(string, string, string) (*Reservation, error) { panic("mutation") }
func (s *exactOldServer) MaatRead(k, id string) ([]Reservation, error) {
	s.calls++
	if k != "reservation" {
		panic("broad fallback")
	}
	return nil, fmt.Errorf("oldserver unknown read")
}
func TestExactReservationOldServerNoFallback(t *testing.T) {
	s := &exactOldServer{fakeStore: newFakeStore()}
	if _, e := NewLedger(s).ExactReservation("805f"); e == nil || s.calls != 1 {
		t.Fatal(e, s.calls)
	}
}
