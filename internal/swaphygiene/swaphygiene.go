// Package swaphygiene is the owner's recurring swap and memory hygiene duty
// (owner directive 2026-10-04: "Cleaning stale swap regularly should be a pantheon
// priority"). It measures, classifies and records; it never deletes swap files,
// kills processes, or restarts a host. A nonzero swap ALLOCATION is not paging:
// the verdict comes from swap-in/swap-out deltas between samples, and a restart is
// only ever PROPOSED for the owner and the workload owners, never triggered.
package swaphygiene

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Verdicts, in increasing severity.
const (
	VerdictUnknown        = "unknown"         // telemetry missing: nothing is claimed
	VerdictClean          = "clean"           // no swap in use
	VerdictIdleAllocation = "idle-allocation" // swap is allocated but nothing is paging
	VerdictActivePaging   = "active-paging"   // swap-ins/outs moved since the last sample
	VerdictPressure       = "pressure"        // paging AND free memory below the floor
)

const (
	// CleanSwapMiB: below this much swap in use the host counts as clean for
	// release-performance qualification. Apollo's clean-host rule owns the exact
	// number; this is the published default so the distinction stays visible.
	CleanSwapMiB = 1.0
	// CorrectnessFreePct is the free-memory floor for correctness-only diagnosis.
	CorrectnessFreePct = 50
	// PressureFreePct: paging below this much free memory is real pressure.
	PressureFreePct = 20
	// PagingPagesPerSample: swap-in+out page movement between two samples that counts as paging.
	PagingPagesPerSample = 64
)

// Sample is one reading of host swap and memory.
type Sample struct {
	At           time.Time `json:"at"`
	SwapUsedMiB  float64   `json:"swap_used_mib"`
	SwapTotalMiB float64   `json:"swap_total_mib"`
	SwapKnown    bool      `json:"swap_known"`
	SwapIns      int64     `json:"swap_ins"`  // cumulative pages (vm_stat)
	SwapOuts     int64     `json:"swap_outs"` // cumulative pages (vm_stat)
	PagingKnown  bool      `json:"paging_known"`
	FreePct      int       `json:"free_pct"`
	FreeKnown    bool      `json:"free_known"`
}

// Receipt is a Sample plus what it means. It is appended to the receipts log.
type Receipt struct {
	Sample
	Verdict           string `json:"verdict"`
	DeltaSwapPages    int64  `json:"delta_swap_pages"`    // swap-in+out movement since the previous sample; -1 = unknown
	CorrectnessOnlyOK bool   `json:"correctness_only_ok"` // known swap telemetry and >= 50% free memory
	ReleaseTimingOK   bool   `json:"release_timing_ok"`   // correctness_only_ok, no paging, swap below CleanSwapMiB
	RestartProposed   bool   `json:"restart_proposed"`    // a proposal for owners; nothing is restarted
	Note              string `json:"note,omitempty"`
}

// Runner runs a command and returns stdout; injectable (Rule A16).
type Runner func(name string, args ...string) (string, error)

func execRunner(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return string(out), err
}

var (
	swapRe  = regexp.MustCompile(`total = ([0-9.]+)M\s+used = ([0-9.]+)M`)
	pctRe   = regexp.MustCompile(`free percentage: (\d+)%`)
	countRe = regexp.MustCompile(`^(Swapins|Swapouts):\s+(\d+)\.`)
)

// Take reads the host. Any source that cannot be read leaves its *Known flag false;
// a missing number is never defaulted to a healthy-looking one.
func Take(run Runner, now time.Time) Sample {
	if run == nil {
		run = execRunner
	}
	s := Sample{At: now.UTC()}
	if out, err := run("sysctl", "-n", "vm.swapusage"); err == nil {
		if m := swapRe.FindStringSubmatch(out); m != nil {
			s.SwapTotalMiB, _ = strconv.ParseFloat(m[1], 64)
			s.SwapUsedMiB, _ = strconv.ParseFloat(m[2], 64)
			s.SwapKnown = true
		}
	}
	if out, err := run("vm_stat"); err == nil {
		var gotIn, gotOut bool
		sc := bufio.NewScanner(strings.NewReader(out))
		for sc.Scan() {
			line := strings.Trim(sc.Text(), `" `)
			if m := countRe.FindStringSubmatch(line); m != nil {
				n, _ := strconv.ParseInt(m[2], 10, 64)
				if m[1] == "Swapins" {
					s.SwapIns, gotIn = n, true
				} else {
					s.SwapOuts, gotOut = n, true
				}
			}
		}
		s.PagingKnown = gotIn && gotOut
	}
	if out, err := run("memory_pressure"); err == nil {
		if m := pctRe.FindStringSubmatch(out); m != nil {
			s.FreePct, _ = strconv.Atoi(m[1])
			s.FreeKnown = true
		}
	}
	return s
}

// Assess turns a sample (and the previous one, if any) into a receipt.
func Assess(prev *Sample, cur Sample) Receipt {
	r := Receipt{Sample: cur, DeltaSwapPages: -1}
	if prev != nil && prev.PagingKnown && cur.PagingKnown {
		r.DeltaSwapPages = (cur.SwapIns - prev.SwapIns) + (cur.SwapOuts - prev.SwapOuts)
		if r.DeltaSwapPages < 0 { // counter reset (reboot): no movement can be claimed
			r.DeltaSwapPages = -1
		}
	}
	switch {
	case !cur.SwapKnown:
		r.Verdict = VerdictUnknown
		r.Note = "swap telemetry unavailable; no claim made"
	case cur.SwapUsedMiB < CleanSwapMiB:
		r.Verdict = VerdictClean
	case r.DeltaSwapPages < 0:
		r.Verdict = VerdictIdleAllocation
		r.Note = "swap is allocated; paging not measurable yet (needs two samples), not treated as active"
	case r.DeltaSwapPages >= PagingPagesPerSample:
		r.Verdict = VerdictActivePaging
		if cur.FreeKnown && cur.FreePct < PressureFreePct {
			r.Verdict = VerdictPressure
			r.RestartProposed = true
			r.Note = "paging under memory pressure: PROPOSE a coordinated restart to the owner and active workload owners (not triggered)"
		}
	default:
		r.Verdict = VerdictIdleAllocation
	}
	r.CorrectnessOnlyOK = cur.SwapKnown && cur.FreeKnown && cur.FreePct >= CorrectnessFreePct
	r.ReleaseTimingOK = r.CorrectnessOnlyOK && r.Verdict == VerdictClean
	return r
}

// Dir is where state and receipts live.
func Dir(home string) string { return filepath.Join(home, ".sirsi", "swap-hygiene") }

func lastPath(home string) string     { return filepath.Join(Dir(home), "last.json") }
func receiptsPath(home string) string { return filepath.Join(Dir(home), "receipts.jsonl") }

// Last returns the most recent receipt, if one exists.
func Last(home string) (*Receipt, error) {
	b, err := os.ReadFile(lastPath(home))
	if err != nil {
		return nil, err
	}
	var r Receipt
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// maxReceiptsBytes bounds the log; at the cap the oldest half is dropped.
const maxReceiptsBytes = 1 << 20

// Record samples the host, assesses it against the previous sample, and persists
// last.json and one receipts.jsonl line. It reclaims nothing.
func Record(home string, run Runner, now time.Time) (Receipt, error) {
	var prev *Sample
	if last, err := Last(home); err == nil {
		prev = &last.Sample
	}
	r := Assess(prev, Take(run, now))
	if err := os.MkdirAll(Dir(home), 0o755); err != nil {
		return r, err
	}
	b, _ := json.Marshal(r)
	if err := os.WriteFile(lastPath(home), b, 0o644); err != nil {
		return r, err
	}
	if st, err := os.Stat(receiptsPath(home)); err == nil && st.Size() > maxReceiptsBytes {
		if data, rerr := os.ReadFile(receiptsPath(home)); rerr == nil {
			_ = os.WriteFile(receiptsPath(home), data[len(data)/2:], 0o644)
		}
	}
	f, err := os.OpenFile(receiptsPath(home), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return r, err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\n", b)
	return r, err
}
