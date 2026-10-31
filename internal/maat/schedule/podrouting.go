package schedule

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// PodRoutingStore wires the existing Ledger entry points to native admission
// and own lifecycle operations. Unconfigured clients never construct it.
type PodRoutingStore struct {
	StateStore
	Client    PodClient
	Canonical AtomicScheduler
}

func (s *PodRoutingStore) MaatReserve(r Reservation, q bool) (ReserveResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.Client.Reserve(ctx, r, q)
}
func (s *PodRoutingStore) MaatMutate(action, id, value string) (*Reservation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.Client.Mutate(ctx, action, id, value)
}
func (s *PodRoutingStore) MaatRead(kind, resource string) ([]Reservation, error) {
	if kind == "reservation" && strings.HasPrefix(resource, "pod:") {
		return nil, fmt.Errorf("maat: exact delegated reservation read not supported by pod transport; canonical fallback refused")
	}
	if kind == "status" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		rows, handled, err := s.Client.ReadOwnStatus(ctx, resource)
		if handled {
			return rows, err
		}
	}
	if s.Canonical == nil {
		return nil, fmt.Errorf("maat: canonical read client unavailable")
	}
	return s.Canonical.MaatRead(kind, resource)
}
