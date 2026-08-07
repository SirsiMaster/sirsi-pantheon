package main

// sirsi vitals — the fast memory snapshot every interactive surface polls
// (TUI Pulse gauge, menubar, dashboard; TUI design proof gap V1).
//
// Budget: <100ms. `sirsi diagnose` runs the full 12-check diagnostic (seconds)
// — the wrong tool for a 2s-tick gauge. Vitals therefore takes ONE memory
// sample (guard.SampleMemory: vm_stat + ps, the same probe that powers Hapi
// and `relieve --memory`), resolves pressure through the same chain
// SampleNodeCapacity uses (kernel dispatch → daemon cache → bootstrap), and
// reads swap via the same sysctl the doctor's swap check runs. NO new probing
// logic — this command only exposes what the guard engine already measures.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
	"github.com/SirsiMaster/sirsi-pantheon/internal/seba"
)

var vitalsTop int

// Injectable providers (Rule A16/A21) so vitals is testable without reading
// the real host. Swapped under vitalsProvidersMu by tests — which must NOT
// use t.Parallel() (package-global swap; repo lessons #129/#131).
var (
	vitalsProvidersMu sync.Mutex
	vitalsMemFn       = guard.SampleMemory
	vitalsPressureFn  = guard.EffectivePressure
	vitalsSwapFn      = func() int64 { return guard.SwapUsedBytes(platform.Current()) }
)

// vitalsProc is one memory hog in the snapshot: name, pid, and its best-known
// size. RSSBytes keeps its JSON key ("rss_bytes") for menubar/dashboard
// schema compatibility, but the VALUE is MemProc.Size() — physical footprint
// (broker-truth-corrected for the load-bearing local-model broker via
// guard.BrokerMLXActiveBytes/mlx_active_bytes) when available, raw RSS only
// as the last resort. Raw RSS understated the broker by up to 27 GB (measured
// 2026-08-06, PRD R6, sirsi-pantheon-fabric SNE_HETEROGENEOUS_COMPUTE.md) —
// serializing p.RSS directly here would have shown that lie to every consumer
// of this command (menubar, dashboard, TUI).
type vitalsProc struct {
	Name     string `json:"name"`
	PID      int    `json:"pid"`
	RSSBytes int64  `json:"rss_bytes"`
}

// vitalsReport is the `sirsi vitals --json` contract (TUI design proof V1).
// Free/used follow the governor's definition of free (free + inactive +
// speculative pages — what the broker's RAM gate and Hapi agree on), so every
// surface shows the same numbers the engine acts on.
type vitalsReport struct {
	Command        string       `json:"command"`
	TotalBytes     int64        `json:"total_bytes"`
	UsedBytes      int64        `json:"used_bytes"`
	FreeBytes      int64        `json:"free_bytes"`
	SwapUsedBytes  int64        `json:"swap_used_bytes"`
	Pressure       string       `json:"pressure"`        // normal|warn|critical|unknown (guard.PressureLevel vocabulary)
	PressureSource string       `json:"pressure_source"` // kernel-dispatch|bootstrap-snapshot|unknown
	Top            []vitalsProc `json:"top"`             // top memory hogs, RSS descending
}

// buildVitalsReport assembles the contract from already-taken samples — pure,
// so tests pin the shape without touching the host.
func buildVitalsReport(mem guard.MemSample, pressure guard.PressureLevel, pressureSource string, swapUsed int64, topN int) vitalsReport {
	r := vitalsReport{
		Command:        "sirsi vitals",
		TotalBytes:     mem.TotalRAM,
		FreeBytes:      mem.FreeBytes,
		SwapUsedBytes:  swapUsed,
		Pressure:       pressure.String(),
		PressureSource: pressureSource,
		Top:            []vitalsProc{},
	}
	if used := mem.TotalRAM - mem.FreeBytes; used > 0 {
		r.UsedBytes = used
	}
	for _, p := range mem.Top {
		if topN > 0 && len(r.Top) >= topN {
			break
		}
		name := p.Name
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:] // basename — "codex", not the full bundle path
		}
		r.Top = append(r.Top, vitalsProc{Name: name, PID: p.PID, RSSBytes: p.Size()})
	}
	return r
}

var vitalsCmd = &cobra.Command{
	Use:   "vitals",
	Short: "Fast memory snapshot — RAM, pressure, swap, top memory users",
	Long: `Vitals is the quick read on memory RIGHT NOW: total/used/free RAM, the live
pressure level, swap in use, and the processes holding the most memory.

  sirsi vitals              Snapshot with the top memory users
  sirsi vitals --top 10     Show more memory users
  sirsi vitals --json       Structured output for tools and dashboards

It samples once and returns immediately (built for a 2-second dashboard tick).
For the full 12-check health diagnostic, run sirsi diagnose.`,
	RunE: runVitals,
}

func init() {
	vitalsCmd.Flags().IntVar(&vitalsTop, "top", 5, "Number of top memory users to include")
}

func runVitals(cmd *cobra.Command, args []string) error {
	start := time.Now()

	vitalsProvidersMu.Lock()
	memFn, pressureFn, swapFn := vitalsMemFn, vitalsPressureFn, vitalsSwapFn
	vitalsProvidersMu.Unlock()

	mem, err := memFn()
	if err != nil {
		if JsonOutput {
			return fmt.Errorf("vitals: memory sample failed: %w", err)
		}
		res := &output.CommandResult{Command: "sirsi vitals", Status: "error"}
		res.Errors = []string{err.Error()}
		res.Summary = "Couldn't sample system memory."
		res.Render()
		return nil
	}
	pressure, pressureSource := pressureFn(mem.FreeBytes, mem.TotalRAM)
	report := buildVitalsReport(mem, pressure, pressureSource, swapFn(), vitalsTop)

	if JsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	// Human render — lipgloss dashboard + table (gold theme via internal/output).
	output.Header("Memory Vitals")
	output.Dashboard(map[string]string{
		"Used":     fmt.Sprintf("%s of %s", seba.FormatBytes(report.UsedBytes), seba.FormatBytes(report.TotalBytes)),
		"Free":     seba.FormatBytes(report.FreeBytes),
		"Pressure": report.Pressure,
		"Swap":     seba.FormatBytes(report.SwapUsedBytes),
	})

	if len(report.Top) > 0 {
		var rows [][]string
		for _, p := range report.Top {
			rows = append(rows, []string{p.Name, fmt.Sprintf("%d", p.PID), seba.FormatBytes(p.RSSBytes)})
		}
		output.Table([]string{"TOP MEMORY USERS", "PID", "MEMORY"}, rows)
	}

	output.Dim("Sampled in %s · pressure source: %s", time.Since(start).Truncate(time.Millisecond), report.PressureSource)
	output.NextSteps([][]string{
		{"sirsi relieve --memory", "Flush inactive caches if memory is tight"},
		{"sirsi diagnose", "Full 12-check health diagnostic"},
	})
	return nil
}
