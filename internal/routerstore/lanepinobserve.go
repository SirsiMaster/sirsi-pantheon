package routerstore

// REPORT-ONLY lane-pin observation at a served Wait (owner decision 2026-11-25). The verdict
// (EvaluateLanePin) is computed and counted; NO request changes outcome and a failure to evaluate is counted,
// never returned. Enforcement is deliberately not built: lanePinRefuses is the one named seam.
//
// Cost on the hot path: zero extra queries for the caller's machine id (hostForBearer already read it from
// host_tokens); one indexed read of the lane's active pins on a cache miss, then a 60 s cache (10 s after a
// failed read, so a sick database is not hammered). A pin set or retired by the operator therefore takes up to
// 60 s to be seen by a running service instance. Counters are per instance and reset on restart.

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	lanePinCacheTTL    = 60 * time.Second
	lanePinErrTTL      = 10 * time.Second
	lanePinCacheMax    = 256 // distinct lane names cached; a caller can name any lane, so this is bounded
	lanePinCountMax    = 512 // hard cap on counter keys (pinned lanes x 4 verdicts + a handful of aggregates)
	lanePinLogEvery    = 5 * time.Minute
	lanePinUnpinnedLbl = "_unpinned" // every lane with no active pin shares one label, so a caller cannot mint counter keys
	lanePinOverflowLbl = "_overflow"
)

type lanePinCacheEntry struct {
	pins    []string
	err     bool
	expires time.Time
}

type lanePinCountKey struct {
	lane    string
	verdict LanePinVerdict
}

// lanePinObserver is the in-memory cache and bounded counters of one service instance.
type lanePinObserver struct {
	mu      sync.Mutex
	cache   map[string]lanePinCacheEntry
	counts  map[lanePinCountKey]uint64
	lastLog time.Time
}

// lanePinRefuses is THE ENFORCEMENT SEAM. It is report-only by owner decision (2026-11-25): it returns false for
// every verdict. When the owner turns enforcement on, this is the single function that changes (plus its
// wiring in call), and the mismatch counters are the evidence of what it would have refused.
func (s *server) lanePinRefuses(LanePinVerdict) bool { return false }

// observeLanePin evaluates and counts one served Wait for lane by a token whose adopted machine id is machineID.
// It never fails and never panics outward: whatever happens inside, the caller proceeds exactly as before.
func (s *server) observeLanePin(lane, machineID string) (v LanePinVerdict, refuse bool) {
	defer func() {
		if recover() != nil {
			v, refuse = lanePinError, false
		}
	}()
	if lane == "" || len(lane) > lanePinMaxIdent {
		return "", false // not a lane name the pin table can hold; nothing to evaluate
	}
	o := &s.pins
	now := s.opts.now()
	pins, errored, cached := o.lookup(lane, now)
	if !cached {
		p, err := s.store.ActiveLanePinMachines(lane) // the one hot-path query, outside the lock
		switch {
		case err != nil:
			errored, pins = true, nil
			o.put(lane, nil, true, now)
		default:
			pins = p
			if !o.put(lane, p, false, now) {
				o.count(lanePinOverflowLbl, lanePinSkipped)
				return lanePinSkipped, false
			}
		}
	}
	if errored {
		o.count(lanePinUnpinnedLbl, lanePinError)
		s.maybeLogLanePins(now)
		return lanePinError, false
	}
	v = EvaluateLanePin(machineID, pins)
	label := lane
	if len(pins) == 0 {
		label = lanePinUnpinnedLbl
	}
	o.count(label, v)
	s.maybeLogLanePins(now)
	return v, s.lanePinRefuses(v)
}

func (o *lanePinObserver) lookup(lane string, now time.Time) (pins []string, errored, ok bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	e, hit := o.cache[lane]
	if !hit || !now.Before(e.expires) {
		return nil, false, false
	}
	return e.pins, e.err, true
}

// put caches a lookup result. False means the cache is full of live entries and this lane was not cached.
func (o *lanePinObserver) put(lane string, pins []string, errored bool, now time.Time) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.cache == nil {
		o.cache = map[string]lanePinCacheEntry{}
	}
	if _, exists := o.cache[lane]; !exists && len(o.cache) >= lanePinCacheMax {
		for k, e := range o.cache { // evict expired entries before refusing a new lane
			if !now.Before(e.expires) {
				delete(o.cache, k)
			}
		}
		if len(o.cache) >= lanePinCacheMax {
			return false
		}
	}
	ttl := lanePinCacheTTL
	if errored {
		ttl = lanePinErrTTL
	}
	o.cache[lane] = lanePinCacheEntry{pins: pins, err: errored, expires: now.Add(ttl)}
	return true
}

func (o *lanePinObserver) count(label string, v LanePinVerdict) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.counts == nil {
		o.counts = map[lanePinCountKey]uint64{}
	}
	k := lanePinCountKey{label, v}
	if _, exists := o.counts[k]; !exists && len(o.counts) >= lanePinCountMax {
		return // bounded: a new key past the cap is dropped, never stored
	}
	o.counts[k]++
}

// snapshot is "lane|verdict" -> count, for tests and the periodic log line.
func (o *lanePinObserver) snapshot() map[string]uint64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make(map[string]uint64, len(o.counts))
	for k, n := range o.counts {
		out[k.lane+"|"+string(k.verdict)] = n
	}
	return out
}

// maybeLogLanePins writes one summary line at most every lanePinLogEvery, so the counters are readable from the
// service log without a new endpoint. Nothing secret is in it: lane names, verdict names and counts only.
func (s *server) maybeLogLanePins(now time.Time) {
	o := &s.pins
	o.mu.Lock()
	if now.Sub(o.lastLog) < lanePinLogEvery {
		o.mu.Unlock()
		return
	}
	o.lastLog = now
	o.mu.Unlock()
	snap := o.snapshot()
	keys := make([]string, 0, len(snap))
	for k := range snap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(" " + k + "=" + strconv.FormatUint(snap[k], 10))
	}
	s.logf("lane-pin (report-only, per instance, since start):%s", b.String())
}
