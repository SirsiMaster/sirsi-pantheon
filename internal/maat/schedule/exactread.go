package schedule

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// ExactReservation reads at most one canonical reservation, including terminal
// history. No status dump crosses the wire and no admission/mutation is performed.
// Old servers reject the new MaatRead kind; never fall back to a history dump.
func (l *Ledger) ExactReservation(id string) ([]Reservation, error) {
	if id == "" || len(id) > 256 || strings.TrimSpace(id) != id || strings.IndexFunc(id, unicode.IsControl) >= 0 {
		return nil, fmt.Errorf("maat: exact reservation ID required (1..256 bytes, no surrounding whitespace/control characters)")
	}
	// A canonical projection is not evidence of the delegated pod authority's
	// current state. This must precede both remote dispatch and local reads so
	// direct server callers cannot bypass the pod-routing wrapper.
	if strings.HasPrefix(id, "pod:") {
		return nil, fmt.Errorf("maat: exact delegated reservation read not supported; canonical fallback refused")
	}
	if a, ok := l.store.(AtomicScheduler); ok {
		return a.MaatRead("reservation", id)
	}
	rows, _, err := l.load()
	if err != nil {
		return nil, err
	}
	out := []Reservation{}
	for _, row := range rows {
		if row.ID == id {
			out = append(out, row)
			if len(out) > 1 {
				return nil, fmt.Errorf("maat: duplicate exact reservation ID")
			}
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if len(b) > 64<<10 {
		return nil, fmt.Errorf("maat: exact reservation exceeds 64KiB read bound; record not truncated")
	}
	return out, nil
}
