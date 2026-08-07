package guard

// brokertrend.go — distinguishes a STABLE broker footprint from a GROWING one,
// so the memory-hog check (doctor.go checkTopMemoryProcesses) can stop
// alarming on the broker's ordinary steady-state size while still catching
// the failure mode that burned this repo before.
//
// This is deliberately NOT the same shape as the exemption removed on
// 2026-07-27 (see doctor.go's comment on that check, and PANTHEON_RULES.md
// Rule A35's "the broker is capped at 20.8 GiB" incident). That exemption
// trusted the broker's OWN self-reported cap (mlx_memory_limit_bytes) —
// which the broker's own /health response labels
// "scheduler_backpressure_not_allocation_cap", i.e. a scheduling hint, not an
// enforced ceiling. Trusting it was why three real OOM/Jetsam incidents (31,
// 43.9, 38.1 GB) went unflagged.
//
// This check trusts nothing the broker claims about itself. It only trusts
// what THIS process independently measured over time: a rolling history of
// footprint samples on disk. A footprint that has been flat for a while is
// treated as normal; a footprint that is actively growing is a hog
// regardless of its absolute size relative to any claimed cap. A broker with
// no observed history yet (fresh install, history file missing/corrupt) is
// NOT assumed stable — insufficient data fails toward alarming, matching the
// "never invent a number" posture the rest of this package already uses for
// broker reads.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// brokerFootprintHistoryFile is where independently-measured samples live.
// Distinct from anything the broker itself writes — this file is only ever
// appended to by this check.
const brokerFootprintHistoryFile = "broker-footprint-history.jsonl"

// brokerTrendMinHistory is the minimum elapsed time before a footprint can be
// called "stable". Below this, there simply hasn't been enough observation to
// say the broker ISN'T mid-spike — the 2026-07-27 incidents each took several
// minutes to run away.
const brokerTrendMinHistory = 10 * time.Minute

// brokerTrendMaxAge bounds how long a sample stays relevant. A sample from 6
// hours ago says nothing about whether the broker is stable right now.
const brokerTrendMaxAge = 2 * time.Hour

// brokerTrendGrowthRatio and brokerTrendGrowthAbs are the two ways a
// footprint counts as "growing" — ratio catches proportional runaway growth,
// absolute catches a small-base percentage that would otherwise look calm
// (e.g. 4.1 GB -> 8 GB is +95% but also a real +3.9 GB jump either check
// should catch).
const brokerTrendGrowthRatio = 0.25                        // +25% since the oldest in-window sample
const brokerTrendGrowthAbs = int64(4) * 1024 * 1024 * 1024 // +4 GB

// footprintSample is one independently-measured broker footprint reading.
type footprintSample struct {
	Unix  int64 `json:"t"`
	Bytes int64 `json:"b"`
}

func brokerFootprintHistoryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sirsi", brokerFootprintHistoryFile), nil
}

// loadBrokerFootprintHistory reads existing samples. A missing or corrupt
// file is treated as empty history, never an error — the caller's job is to
// start observing from now, not to fail the whole doctor pass.
func loadBrokerFootprintHistory() []footprintSample {
	path, err := brokerFootprintHistoryPath()
	if err != nil {
		return nil
	}
	f, err := os.Open(path) //nolint:gosec // fixed path under ~/.sirsi
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var samples []footprintSample
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var s footprintSample
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			continue // one bad line never invalidates the rest of the history
		}
		samples = append(samples, s)
	}
	return samples
}

// pruneAndAppendFootprint drops samples older than brokerTrendMaxAge relative
// to now, then appends the current reading. Pure function — no I/O — so the
// trend math is unit-testable without a filesystem.
func pruneAndAppendFootprint(history []footprintSample, now time.Time, bytes int64) []footprintSample {
	cutoff := now.Add(-brokerTrendMaxAge).Unix()
	kept := make([]footprintSample, 0, len(history)+1)
	for _, s := range history {
		if s.Unix >= cutoff {
			kept = append(kept, s)
		}
	}
	kept = append(kept, footprintSample{Unix: now.Unix(), Bytes: bytes})
	return kept
}

func saveBrokerFootprintHistory(history []footprintSample) error {
	path, err := brokerFootprintHistoryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	for _, s := range history {
		line, err := json.Marshal(s)
		if err != nil {
			continue
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0o644) //nolint:gosec // local diagnostic history, not sensitive
}

// evaluateBrokerTrend decides stability from history ALONE (not including the
// current sample being folded in by the caller) — pure, so it's testable
// without touching disk or a clock.
//
// Returns stable=false whenever there isn't enough qualifying history to
// support a "yes, this is normal" claim — fail toward alarming, never toward
// silence, matching the never-invent-a-number posture used elsewhere in this
// package for broker reads.
func evaluateBrokerTrend(history []footprintSample, now time.Time, currentBytes int64) (stable bool, detail string) {
	cutoff := now.Add(-brokerTrendMaxAge).Unix()
	var oldest *footprintSample
	for i := range history {
		s := history[i]
		if s.Unix < cutoff {
			continue
		}
		if oldest == nil || s.Unix < oldest.Unix {
			oldest = &history[i]
		}
	}

	if oldest == nil {
		return false, "no observation history yet"
	}

	age := now.Sub(time.Unix(oldest.Unix, 0))
	if age < brokerTrendMinHistory {
		return false, fmt.Sprintf("only %s of observation so far (need %s)", age.Round(time.Second), brokerTrendMinHistory)
	}

	if oldest.Bytes <= 0 {
		return false, "invalid baseline sample"
	}

	growth := currentBytes - oldest.Bytes
	ratio := float64(growth) / float64(oldest.Bytes)

	if growth > brokerTrendGrowthAbs || ratio > brokerTrendGrowthRatio {
		return false, fmt.Sprintf("grew %s (%.0f%%) over %s", FormatBytes(growth), ratio*100, age.Round(time.Minute))
	}

	return true, fmt.Sprintf("flat for %s (%+.0f%% since oldest sample)", age.Round(time.Minute), ratio*100)
}

// BrokerTrendStable records the current footprint into the on-disk history
// (best-effort — a write failure never blocks the doctor pass) and reports
// whether the broker's measured behavior over time counts as stable.
func BrokerTrendStable(currentBytes int64) (stable bool, detail string) {
	now := time.Now()
	history := loadBrokerFootprintHistory()

	stable, detail = evaluateBrokerTrend(history, now, currentBytes)

	updated := pruneAndAppendFootprint(history, now, currentBytes)
	_ = saveBrokerFootprintHistory(updated) // best-effort; see doc comment

	return stable, detail
}
