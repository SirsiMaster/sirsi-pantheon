package schedule

import (
	"context"
	"fmt"
)

// PodRoutingStore wires the existing Ledger entry points to native admission
// and own lifecycle operations. Unconfigured clients never construct it.
type PodRoutingStore struct {
	StateStore
	Client    PodClient
	Canonical AtomicScheduler
}

func (s *PodRoutingStore) MaatReserve(r Reservation, q bool) (ReserveResult, error) {
	return s.Client.Reserve(context.Background(), r, q)
}
func (s *PodRoutingStore) MaatMutate(action, id, value string) (*Reservation, error) {
	return s.Client.Mutate(context.Background(), action, id, value)
}
func (s *PodRoutingStore) MaatRead(kind, resource string) ([]Reservation, error) {
	if s.Canonical == nil {
		return nil, fmt.Errorf("maat: canonical read client unavailable")
	}
	return s.Canonical.MaatRead(kind, resource)
}
