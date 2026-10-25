package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/apollo"
	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/swaphygiene"
)

// Reuse the host's measured capacity and the existing Known-bearing swap/free
// sample. RAM is explicitly the complement of kernel available-memory percent;
// it is not a fabricated zero, RSS measurement, or a release timing verdict.
func fabricHostFrom(capacity guard.NodeCapacity, sample swaphygiene.Sample, telemetry apollo.TelemetryRead, telemetryErr error, dirty *bool, now time.Time, machineID string) ledger.FabricHealth {
	h := ledger.FabricHealth{Engine: "unknown", EngineState: "unavailable", Issues: []string{}, ObservedAt: now.UTC().Format(time.RFC3339), GitDirty: dirty}
	if capacity.TotalRAM > 0 {
		n := capacity.TotalRAM
		h.TotalRAM = &n
	} else {
		h.Issues = append(h.Issues, "host capacity unavailable")
	}
	if capacity.CPUCores > 0 {
		n := capacity.CPUCores
		h.CPUCores = &n
	}
	if sample.FreeKnown && sample.FreePct >= 0 && sample.FreePct <= 100 {
		pct := float64(100 - sample.FreePct)
		h.RAMPct = &pct
		h.RAMSource = "memory_pressure available-percent complement"
	} else {
		h.Issues = append(h.Issues, "RAM availability unavailable")
	}
	if sample.SwapKnown && !math.IsNaN(sample.SwapUsedMiB) && !math.IsInf(sample.SwapUsedMiB, 0) && sample.SwapUsedMiB >= 0 && sample.SwapUsedMiB < float64(math.MaxInt64)/(1024*1024) {
		n := int64(sample.SwapUsedMiB * 1024 * 1024)
		h.Swap = &n
	} else {
		h.Issues = append(h.Issues, "swap observation unavailable")
	}
	if dirty == nil {
		h.Issues = append(h.Issues, "repository state unavailable")
	}
	if telemetryErr == nil && telemetry.State == "active" && telemetry.Telemetry != nil {
		t := telemetry.Telemetry
		age := now.Sub(t.EmittedAt)
		if machineID == "" || (t.MachineID != machineID && !(machineID == "this-mac" && t.MachineID == "")) {
			h.EngineState = "binding_mismatch"
			h.Issues = append(h.Issues, "engine session machine binding does not match local host")
		} else if age >= 0 && age <= 2*time.Minute && strings.TrimSpace(t.EngineID) != "" {
			h.Engine = t.EngineID
			h.EngineState = "observed_session"
		} else {
			h.EngineState = "stale_session"
		}
	} else if telemetryErr == nil && telemetry.State == "awaiting_session" {
		h.EngineState = "awaiting_session"
	}
	if h.Engine == "unknown" {
		h.Issues = append(h.Issues, "engine availability not established")
	}
	return h
}

type fabricOutput struct{ bytes.Buffer }

func (b *fabricOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1<<20 {
		return 0, errors.New("fabric read output exceeds bound")
	}
	return b.Buffer.Write(p)
}
func fabricHostCommand(name string, args ...string) (string, error) {
	paths := map[string]string{"sysctl": "/usr/sbin/sysctl", "vm_stat": "/usr/bin/vm_stat", "memory_pressure": "/usr/bin/memory_pressure", "git": "/usr/bin/git"}
	path, ok := paths[name]
	if !ok {
		return "", errors.New("unsupported host read")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	if name == "git" {
		cmd.Env = []string{}
		for _, entry := range os.Environ() {
			key, _, _ := strings.Cut(entry, "=")
			if !strings.HasPrefix(key, "GIT_") {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		cmd.Env = append(cmd.Env, "GIT_OPTIONAL_LOCKS=0")
	}
	out := &fabricOutput{}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}
func collectFabricHostHealth(repo string, now time.Time) ledger.FabricHealth {
	capacity := sampleNodeCapacityFn()                 // existing shared capacity read model, no policy writes
	sample := swaphygiene.Take(fabricHostCommand, now) // read-only; never Record()
	var dirty *bool
	if status, err := fabricHostCommand("git", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-C", repo, "status", "--porcelain", "--untracked-files=normal"); err == nil {
		value := status != ""
		dirty = &value
	}
	var telemetry apollo.TelemetryRead
	home, err := os.UserHomeDir()
	if err == nil {
		telemetry, err = apollo.ReadTelemetry(home)
	}
	// Apollo Collect identifies this local, single-device route as this-mac.
	return fabricHostFrom(capacity, sample, telemetry, err, dirty, now, "this-mac")
}
