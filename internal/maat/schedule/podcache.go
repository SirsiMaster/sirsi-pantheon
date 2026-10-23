package schedule

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// FiniteGrantCache holds a defensive snapshot fetched through the authenticated
// canonical API. There is no public setter or renewable local expiry. A process
// restart without authenticated canonical access still fails closed.
type FiniteGrantCache struct{ encoded string }

var ErrGrantLookupUnavailable = errors.New("maat: canonical grant lookup unavailable")

func FetchFiniteGrantCache(source AtomicDelegationScheduler, pod string) (*FiniteGrantCache, error) {
	if source == nil {
		return nil, fmt.Errorf("maat: authenticated canonical grant source required")
	}
	d, err := source.MaatGetDelegation(pod)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrGrantLookupUnavailable, err)
	}
	if d == nil || d.Pod != pod || d.Epoch < 1 {
		return nil, fmt.Errorf("maat: exact canonical delegation required")
	}
	if err = validateDelegation(*d); err != nil {
		return nil, err
	}
	issued, _ := time.Parse(time.RFC3339, d.IssuedAt)
	if d.expired(time.Now()) || time.Now().Before(issued) {
		return nil, fmt.Errorf("maat: finite delegation not active")
	}
	copy := *d
	copy.AckedSeq = 0
	b, err := json.Marshal(copy)
	if err != nil {
		return nil, err
	}
	return &FiniteGrantCache{encoded: string(b)}, nil
}

func (c *FiniteGrantCache) Snapshot() (*Delegation, error) {
	if c == nil {
		return nil, fmt.Errorf("maat: no authenticated cached delegation")
	}
	var d Delegation
	if err := json.Unmarshal([]byte(c.encoded), &d); err != nil {
		return nil, err
	}
	if d.expired(time.Now()) {
		return nil, fmt.Errorf("maat: cached delegation expired or revoked")
	}
	return &d, nil
}
