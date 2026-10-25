package main

import (
	"bytes"
	"errors"
	"github.com/SirsiMaster/sirsi-pantheon/internal/apollo"
	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/swaphygiene"
	"math"
	"testing"
	"time"
)

func TestFabricHostKnownZeroAndUnavailableAreDistinct(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	dirty := false
	good := fabricHostFrom(guard.NodeCapacity{TotalRAM: 48 << 30, CPUCores: 12}, swaphygiene.Sample{FreeKnown: true, FreePct: 100, SwapKnown: true}, apollo.TelemetryRead{State: "active", Telemetry: &apollo.Telemetry{EngineID: "inert-engine", EmittedAt: now}}, nil, &dirty, now)
	if good.RAMPct == nil || *good.RAMPct != 0 || good.Swap == nil || *good.Swap != 0 || good.GitDirty == nil || *good.GitDirty || good.TotalRAM == nil || good.CPUCores == nil || good.Engine != "inert-engine" {
		t.Fatalf("known facts lost: %+v", good)
	}
	unknown := fabricHostFrom(guard.NodeCapacity{}, swaphygiene.Sample{}, apollo.TelemetryRead{}, errors.New("inert"), nil, now)
	if unknown.RAMPct != nil || unknown.Swap != nil || unknown.GitDirty != nil || unknown.TotalRAM != nil || unknown.CPUCores != nil || unknown.Engine != "unknown" || unknown.EngineState != "unavailable" {
		t.Fatalf("unknown became zero/healthy: %+v", unknown)
	}
}
func TestFabricHostRejectsMalformedAndStaleObservations(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	for _, used := range []float64{-1, math.NaN(), math.Inf(1)} {
		h := fabricHostFrom(guard.NodeCapacity{}, swaphygiene.Sample{FreeKnown: true, FreePct: 101, SwapKnown: true, SwapUsedMiB: used}, apollo.TelemetryRead{State: "active", Telemetry: &apollo.Telemetry{EngineID: "old", EmittedAt: now.Add(-3 * time.Minute)}}, nil, nil, now)
		if h.RAMPct != nil || h.Swap != nil || h.Engine != "unknown" || h.EngineState != "stale_session" {
			t.Fatalf("malformed/stale observation accepted: %+v", h)
		}
	}
	waiting := fabricHostFrom(guard.NodeCapacity{}, swaphygiene.Sample{}, apollo.TelemetryRead{State: "awaiting_session"}, nil, nil, now)
	if waiting.Engine != "unknown" || waiting.EngineState != "awaiting_session" {
		t.Fatal("no session was promoted to ready/down")
	}
}
func TestRouterFabricCommandUsesSelectedWriterAndRefusesUnavailable(t *testing.T) {
	for _, fail := range []bool{false, true} {
		out := &bytes.Buffer{}
		calls := 0
		cmd := newRouterFabricCommand(func() (ledger.FabricBoard, error) {
			calls++
			if fail {
				return ledger.FabricBoard{}, errors.New("inert unavailable")
			}
			return ledger.FabricFrom(ledger.Snapshot{GeneratedAt: "2026-10-25T12:00:00Z"}, nil, "fixture"), nil
		})
		cmd.SetArgs([]string{})
		cmd.SetOut(out)
		cmd.SetErr(&bytes.Buffer{})
		err := cmd.Execute()
		if calls != 1 {
			t.Fatal(calls)
		}
		if fail {
			if err == nil || out.Len() != 0 {
				t.Fatal("unavailable command rendered zero counts")
			}
		} else if err != nil || !bytes.Contains(out.Bytes(), []byte(`"activity_seeded":false`)) {
			t.Fatal("selected writer/one-shot coverage lost", err, out.String())
		}
	}
}
