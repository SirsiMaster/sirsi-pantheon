// Fleet capacity and the floor share (owner directive, 2026-09-26): "Maat
// should always grant some space to every lane. Not totally block lanes from
// operation. You, SNE, sha and FinalWishes have 28 cores to share there's
// room for all of you to never be locked out." The fleet is provisioned for
// FairShareLanes lanes sharing a machine at once; a resource's floor share is
// its configured capacity divided by that count, never below 1 core — the
// minimum every lane is guaranteed on Reserve/Extend even when a live
// conflict or an unanswered cede would otherwise apply (reservation.go).
package schedule

import (
	"encoding/json"
	"fmt"
	"strings"
)

// capacityStateKey holds the fleet's per-resource core capacity as one JSON
// map, e.g. {"m1":10,"m5":18}. A resource missing from the stored map falls
// back to defaultCapacity, then to 0 (which still floors to 1 core below).
const capacityStateKey = "maat:capacity"

// defaultCapacity applies when the state key is absent or a resource has no
// explicit entry in it.
var defaultCapacity = map[string]int{"m1": 10, "m5": 18}

// FairShareLanes is the number of lanes the fleet is provisioned to share
// simultaneously on any one machine (Hermes, SNE/Apollo, SHA, FinalWishes —
// owner directive 2026-09-26). FloorShare divides a resource's capacity by
// this count.
const FairShareLanes = 4

func (l *Ledger) loadCapacity() (map[string]int, error) {
	capacities := make(map[string]int, len(defaultCapacity))
	for k, v := range defaultCapacity {
		capacities[k] = v
	}
	raw, ok, err := l.store.GetState(capacityStateKey)
	if err != nil {
		return nil, fmt.Errorf("maat: read capacity: %w", err)
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return capacities, nil
	}
	var stored map[string]int
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, fmt.Errorf("maat: parse capacity: %w", err)
	}
	for k, v := range stored {
		capacities[k] = v
	}
	return capacities, nil
}

// Capacity returns the configured core count for resource: the stored
// override if set, else the default (m1=10, m5=18), else 0 for an unlisted
// resource (FloorShare still floors that to 1).
func (l *Ledger) Capacity(resource string) (int, error) {
	capacities, err := l.loadCapacity()
	if err != nil {
		return 0, err
	}
	return capacities[resource], nil
}

// SetCapacity records resource's core count, overriding the default.
func (l *Ledger) SetCapacity(resource string, cores int) error {
	if !resourceRe.MatchString(resource) {
		return fmt.Errorf("maat: resource %q invalid (want a lowercase slug like m1, rail-a, ci-runners@m5)", resource)
	}
	if cores < 1 {
		return fmt.Errorf("maat: capacity cores must be >= 1, got %d", cores)
	}
	capacities, err := l.loadCapacity()
	if err != nil {
		return err
	}
	capacities[resource] = cores
	b, err := json.Marshal(capacities)
	if err != nil {
		return fmt.Errorf("maat: marshal capacity: %w", err)
	}
	return l.store.SetState(capacityStateKey, string(b))
}

// FloorShare returns the guaranteed minimum core count for resource: its
// capacity divided by FairShareLanes, never below 1 — the grant a lane always
// gets, even when a live conflict or an unanswered cede would otherwise apply.
func (l *Ledger) FloorShare(resource string) (int, error) {
	cores, err := l.Capacity(resource)
	if err != nil {
		return 0, err
	}
	floor := cores / FairShareLanes
	if floor < 1 {
		floor = 1
	}
	return floor, nil
}
