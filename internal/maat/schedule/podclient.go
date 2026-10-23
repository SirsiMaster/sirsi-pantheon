package schedule

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
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
	Cache *FiniteGrantCache
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
		cached, e := c.Cache.Snapshot()
		if e != nil || cached.Pod != c.Pod || !cached.coversResource(req.Resource) {
			return ReserveResult{}, err
		}
		d = cached
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
	prefix := podReservationPrefix(*d)
	if !strings.HasPrefix(req.ID, prefix) {
		if req.ID == "" {
			b := make([]byte, 16)
			if _, err := rand.Read(b); err != nil {
				return ReserveResult{}, err
			}
			req.ID = hex.EncodeToString(b)
		}
		req.ID = prefix + req.ID
	}
	return c.Transport.ReservePod(ctx, *d, req, queue)
}

func podReservationPrefix(d Delegation) string {
	return "pod:" + d.Pod + ":" + strconv.Itoa(d.Epoch) + ":"
}

// Mutate routes only IDs issued by this pod client to its native authority.
// Ordinary reservation IDs retain the old canonical lifecycle path.
func (c PodClient) Mutate(ctx context.Context, action, id, value string) (*Reservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.Pod == "" || !strings.HasPrefix(id, "pod:"+c.Pod+":") {
		switch action {
		case "heartbeat":
			if l, ok := c.Ordinary.(interface {
				Heartbeat(string) (*Reservation, error)
			}); ok {
				return l.Heartbeat(id)
			}
		case "extend":
			if l, ok := c.Ordinary.(interface {
				Extend(string, string) (*Reservation, error)
			}); ok {
				return l.Extend(id, value)
			}
		case "release":
			if l, ok := c.Ordinary.(interface {
				Release(string) (*Reservation, error)
			}); ok {
				return l.Release(id)
			}
		case "invalidate":
			if l, ok := c.Ordinary.(interface {
				Invalidate(string, string) (*Reservation, error)
			}); ok {
				return l.Invalidate(id, value)
			}
		}
		return nil, fmt.Errorf("maat: ordinary lifecycle client unavailable")
	}
	if c.Canonical == nil {
		return nil, fmt.Errorf("maat: canonical delegation source required")
	}
	d, err := c.Canonical.MaatGetDelegation(c.Pod)
	if err != nil {
		d, err = c.Cache.Snapshot()
	}
	if err != nil {
		return nil, err
	}
	if d == nil || d.Pod != c.Pod || d.expired(time.Now()) || !strings.HasPrefix(id, podReservationPrefix(*d)) {
		return nil, fmt.Errorf("maat: reservation epoch not current")
	}
	t, ok := c.Transport.(interface {
		MutatePod(context.Context, Delegation, string, string, string) (*Reservation, error)
	})
	if !ok {
		return nil, fmt.Errorf("maat: native lifecycle authority unavailable; no fallback")
	}
	return t.MutatePod(ctx, *d, action, id, value)
}

// ReadOwnStatus supplies native admission verification without depending on
// canonical catch-up. It returns handled=false only for undelegated resources.
func (c PodClient) ReadOwnStatus(ctx context.Context, resource string) ([]Reservation, bool, error) {
	if c.Pod == "" || resource == "" {
		return nil, false, nil
	}
	if c.Canonical == nil {
		return nil, true, fmt.Errorf("maat: canonical delegation source required")
	}
	var d *Delegation
	var err error
	if bounded, ok := c.Canonical.(contextDelegationScheduler); ok {
		d, err = bounded.MaatGetDelegationContext(ctx, c.Pod)
	} else {
		d, err = c.Canonical.MaatGetDelegation(c.Pod)
	}
	if err != nil {
		d, err = c.Cache.Snapshot()
		if err != nil {
			return nil, true, err
		}
	}
	if d == nil || !d.coversResource(resource) {
		return nil, false, nil
	}
	if d.Pod != c.Pod || d.expired(time.Now()) {
		return nil, true, fmt.Errorf("maat: finite own guard delegation unavailable")
	}
	reader, ok := c.Transport.(interface {
		ReadOwnPod(context.Context, Delegation, string) ([]Reservation, error)
	})
	if !ok {
		return nil, true, fmt.Errorf("maat: authenticated own guard transport unavailable")
	}
	rows, err := reader.ReadOwnPod(ctx, *d, resource)
	return rows, true, err
}
