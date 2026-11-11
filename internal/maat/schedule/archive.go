package schedule

import "encoding/json"

// ArchiveStore is implemented by a StateStore that moves finished reservations
// out of the hot ledger document (the router service does). Every lookup, replay
// or mutation BY ID falls back to it, so a row that left the hot document
// behaves exactly as it did while it was there. Rows that are not found hot and
// not found archived do not exist.
type ArchiveStore interface {
	// FindArchived returns the archived row with this id as raw JSON.
	FindArchived(id string) (json.RawMessage, bool, error)
	// ReplaceArchived rewrites the archived row id in place; nil removes it.
	ReplaceArchived(id string, row json.RawMessage) error
}

// findArchived decodes the archived row for id. A store with no archive has none.
func (l *Ledger) findArchived(id string) (Reservation, json.RawMessage, bool, error) {
	a, ok := l.store.(ArchiveStore)
	if !ok {
		return Reservation{}, nil, false, nil
	}
	raw, found, err := a.FindArchived(id)
	if err != nil || !found {
		return Reservation{}, nil, false, err
	}
	var r Reservation
	if err := json.Unmarshal(raw, &r); err != nil {
		return Reservation{}, nil, false, err
	}
	return r, raw, true, nil
}

// putArchived writes r over the archived row, keeping fields this build does
// not know.
func (l *Ledger) putArchived(id string, prev json.RawMessage, r *Reservation) error {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(prev, &fields)
	merged, err := mergeRow(fields, *r)
	if err != nil {
		return err
	}
	b, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	return l.store.(ArchiveStore).ReplaceArchived(id, b)
}
