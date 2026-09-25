package schedule

import (
	"sort"
	"time"
)

// Coverage is the per-resource usage read-model the dashboard consumes: the live
// holder, the queue, upcoming/active windows, utilization over the horizon, and
// conflicts caught in the last day. Computed on read from the ledger; stored
// nowhere.
type Coverage struct {
	Resource        string        `json:"resource"`
	HorizonHours    int           `json:"horizon_hours"`
	Now             string        `json:"now"`
	Current         *Reservation  `json:"current,omitempty"`    // live holder covering now
	Queued          []Reservation `json:"queued,omitempty"`     // waiting behind the current holder
	Upcoming        []Reservation `json:"upcoming,omitempty"`   // active reservations starting later
	ReservedMinutes int           `json:"reserved_minutes"`     // reserved time inside the horizon
	UtilizationPct  int           `json:"utilization_pct"`      // reserved / horizon
	ConflictsCaught int           `json:"conflicts_caught_24h"` // invalidated blocks in the last 24h
	FreeFrom        string        `json:"free_from,omitempty"`  // when the resource next becomes free
}

// Coverage builds the read-model for one resource over the next horizon hours.
func (l *Ledger) Coverage(resource string, horizonHours int) (Coverage, error) {
	if horizonHours <= 0 {
		horizonHours = 24
	}
	reservations, changed, err := l.load()
	if err != nil {
		return Coverage{}, err
	}
	if changed {
		_ = l.save(reservations)
	}
	now := l.now()
	horizonEnd := now.Add(time.Duration(horizonHours) * time.Hour)
	cov := Coverage{Resource: resource, HorizonHours: horizonHours, Now: now.Format(time.RFC3339)}

	var reservedIntervals [][2]time.Time
	for _, r := range reservations {
		if r.Resource != resource {
			continue
		}
		if r.Status == StatusInvalidated {
			if end, err := time.Parse(time.RFC3339, r.EstEnd); err == nil && now.Sub(end) <= 24*time.Hour {
				cov.ConflictsCaught++
			}
			continue
		}
		if r.Status == StatusQueued {
			cov.Queued = append(cov.Queued, r)
			continue
		}
		if r.Status != StatusActive || r.leaseExpired(now) {
			continue
		}
		s, e := parseWindow(r)
		if !now.Before(s) && now.Before(e) {
			rr := r
			cov.Current = &rr
		} else if s.After(now) {
			cov.Upcoming = append(cov.Upcoming, r)
		}
		// clip to horizon for utilization
		cs, ce := s, e
		if cs.Before(now) {
			cs = now
		}
		if ce.After(horizonEnd) {
			ce = horizonEnd
		}
		if ce.After(cs) {
			reservedIntervals = append(reservedIntervals, [2]time.Time{cs, ce})
		}
	}

	cov.ReservedMinutes = int(mergedMinutes(reservedIntervals))
	total := float64(horizonHours * 60)
	if total > 0 {
		cov.UtilizationPct = int(float64(cov.ReservedMinutes) / total * 100)
	}
	sort.Slice(cov.Upcoming, func(i, j int) bool { return cov.Upcoming[i].Start < cov.Upcoming[j].Start })
	sort.Slice(cov.Queued, func(i, j int) bool { return cov.Queued[i].Priority > cov.Queued[j].Priority })
	if cov.Current != nil {
		_, e := parseWindow(*cov.Current)
		cov.FreeFrom = e.Format(time.RFC3339)
	}
	return cov, nil
}

// mergedMinutes returns the total minutes covered by the union of intervals
// (overlaps counted once).
func mergedMinutes(intervals [][2]time.Time) int64 {
	if len(intervals) == 0 {
		return 0
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i][0].Before(intervals[j][0]) })
	var total time.Duration
	curStart, curEnd := intervals[0][0], intervals[0][1]
	for _, iv := range intervals[1:] {
		if iv[0].After(curEnd) {
			total += curEnd.Sub(curStart)
			curStart, curEnd = iv[0], iv[1]
			continue
		}
		if iv[1].After(curEnd) {
			curEnd = iv[1]
		}
	}
	total += curEnd.Sub(curStart)
	return int64(total.Minutes())
}

// ShouldDefer reports whether a self-hosted CI runner (or any low-priority
// background job) on `machine` must defer right now, because a quiet or loaded
// measurement reservation currently covers that machine. Build-regime
// reservations do not force a deferral (they are the runners themselves).
// A runner calls this before starting a job (P3).
func (l *Ledger) ShouldDefer(machine string) (bool, *Reservation, error) {
	cur, err := l.WhoIsOn(machine)
	if err != nil {
		return false, nil, err
	}
	if cur == nil {
		return false, nil, nil
	}
	if cur.Regime == RegimeQuiet || cur.Regime == RegimeLoaded {
		return true, cur, nil
	}
	return false, cur, nil
}
