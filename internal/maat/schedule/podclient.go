package schedule

import (
	"context"
	"fmt"
	"time"
)

// PodTransport must authenticate the declared holder and verify the exact
// canonical authority machine/epoch at its endpoint. It is supplied by native
// integration; this API never opens an unauthenticated listener or guesses LAN
// addresses. Transport failure is authoritative, never a canonical fallback.
type PodTransport interface {
	ReservePod(context.Context, Delegation, Reservation, bool) (ReserveResult, error)
}

type PodClient struct {
	Pod       string
	Canonical AtomicDelegationScheduler
	Ordinary  interface {
		Reserve(Reservation, bool) (ReserveResult, error)
	}
	Transport PodTransport
	// Cached is accepted only from prior authenticated canonical lookup. It
	// cannot extend a delegation; an unreachable cloud never mints authority.
	Cached *Delegation
}

func (c PodClient) Reserve(ctx context.Context, req Reservation, queue bool) (ReserveResult, error) {
	if err := ctx.Err(); err != nil {
		return ReserveResult{}, err
	}
	// An unconfigured client retains the existing undelegated path exactly.
	if c.Pod == "" {
		if c.Ordinary == nil {
			return ReserveResult{}, fmt.Errorf("maat: canonical client unavailable")
		}
		return c.Ordinary.Reserve(req, queue)
	}
	if c.Canonical == nil {
		return ReserveResult{}, fmt.Errorf("maat: compatible delegation lookup required")
	}
	d, err := c.Canonical.MaatGetDelegation(c.Pod)
	if err != nil {
		if c.Cached == nil || c.Cached.Pod != c.Pod || !c.Cached.coversResource(req.Resource) || c.Cached.expired(time.Now()) {
			return ReserveResult{}, err
		}
		d = c.Cached
	}
	if d == nil || !d.coversResource(req.Resource) {
		if c.Ordinary == nil {
			return ReserveResult{}, fmt.Errorf("maat: canonical client unavailable")
		}
		return c.Ordinary.Reserve(req, queue)
	}
	if c.Transport == nil {
		return ReserveResult{}, fmt.Errorf("maat: delegated authority unavailable; no fallback")
	}
	if d.expired(time.Now()) {
		return ReserveResult{}, fmt.Errorf("maat: cached authority expired or revoked")
	}
	return c.Transport.ReservePod(ctx, *d, req, queue)
}
