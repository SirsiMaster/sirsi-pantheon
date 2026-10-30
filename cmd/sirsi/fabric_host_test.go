package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/apollo"
	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/swaphygiene"
)

func TestFabricHostKnownZeroAndUnavailableAreDistinct(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	dirty := false
	good := fabricHostFrom(guard.NodeCapacity{TotalRAM: 48 << 30, CPUCores: 12}, swaphygiene.Sample{FreeKnown: true, FreePct: 100, SwapKnown: true}, apollo.TelemetryRead{State: "active", Telemetry: &apollo.Telemetry{EngineID: "inert-engine", EmittedAt: now}}, nil, &dirty, now, "this-mac")
	if good.RAMPct == nil || *good.RAMPct != 0 || good.Swap == nil || *good.Swap != 0 || good.GitDirty == nil || *good.GitDirty || good.TotalRAM == nil || good.CPUCores == nil || good.Engine != "inert-engine" {
		t.Fatalf("known facts lost: %+v", good)
	}
	unknown := fabricHostFrom(guard.NodeCapacity{}, swaphygiene.Sample{}, apollo.TelemetryRead{}, errors.New("inert"), nil, now, "this-mac")
	if unknown.RAMPct != nil || unknown.Swap != nil || unknown.GitDirty != nil || unknown.TotalRAM != nil || unknown.CPUCores != nil || unknown.Engine != "unknown" || unknown.EngineState != "unavailable" {
		t.Fatalf("unknown became zero/healthy: %+v", unknown)
	}
}
func TestFabricHostRejectsMalformedAndStaleObservations(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	for _, used := range []float64{-1, math.NaN(), math.Inf(1)} {
		h := fabricHostFrom(guard.NodeCapacity{}, swaphygiene.Sample{FreeKnown: true, FreePct: 101, SwapKnown: true, SwapUsedMiB: used}, apollo.TelemetryRead{State: "active", Telemetry: &apollo.Telemetry{EngineID: "old", EmittedAt: now.Add(-3 * time.Minute)}}, nil, nil, now, "this-mac")
		if h.RAMPct != nil || h.Swap != nil || h.Engine != "unknown" || h.EngineState != "stale_session" {
			t.Fatalf("malformed/stale observation accepted: %+v", h)
		}
	}
	waiting := fabricHostFrom(guard.NodeCapacity{}, swaphygiene.Sample{}, apollo.TelemetryRead{State: "awaiting_session"}, nil, nil, now, "this-mac")
	if waiting.Engine != "unknown" || waiting.EngineState != "awaiting_session" {
		t.Fatal("no session was promoted to ready/down")
	}
}
func TestRouterFabricCommandUsesSelectedWriterAndRefusesUnavailable(t *testing.T) {
	for _, fail := range []bool{false, true} {
		out := &bytes.Buffer{}
		calls := 0
		cause := errors.New("inert unavailable")
		cmd := newRouterFabricCommand(func(context.Context) (ledger.FabricBoard, error) {
			calls++
			if fail {
				return ledger.FabricBoard{}, cause
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
				t.Fatalf("unavailable command must return error with empty stdout: err=%v stdout=%q", err, out.String())
			}
			if !errors.Is(err, cause) {
				t.Fatalf("unavailable command lost reader cause: %v", err)
			}
		} else if err != nil || !bytes.Contains(out.Bytes(), []byte(`"activity_seeded":false`)) {
			t.Fatal("selected writer/one-shot coverage lost", err, out.String())
		}
		if !fail {
			decoder := json.NewDecoder(bytes.NewReader(out.Bytes()))
			var board ledger.FabricBoard
			if err := decoder.Decode(&board); err != nil {
				t.Fatalf("selected writer did not receive a JSON board: %v", err)
			}
			var extra any
			if err := decoder.Decode(&extra); err != io.EOF {
				t.Fatalf("selected writer must contain exactly one JSON value: %v", err)
			}
		}
	}
}

func TestRouterFabricDeadlineKeepsJSONStdoutEmpty(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), -time.Second)
	defer cancel()
	out := &bytes.Buffer{}
	cmd := newRouterFabricCommand(func(got context.Context) (ledger.FabricBoard, error) {
		if got != ctx {
			return ledger.FabricBoard{}, errors.New("command context was not propagated")
		}
		return ledger.FabricBoard{}, &ledger.FabricStageError{Stage: ledger.FabricStageNodeStatus, Elapsed: time.Millisecond, Cause: context.DeadlineExceeded}
	})
	cmd.SetContext(ctx)
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if !errors.Is(err, context.DeadlineExceeded) || out.Len() != 0 {
		t.Fatalf("deadline result must preserve its cause and omit JSON: err=%v stdout=%q", err, out.String())
	}
}

func TestFabricHostEngineMachineBinding(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, machine, sample string
		age                   time.Duration
		readErr               error
		want                  string
	}{
		{name: "matching", machine: "this-mac", sample: "this-mac", want: "observed_session"},
		{name: "legacy local omission", machine: "this-mac", want: "observed_session"},
		{name: "foreign", machine: "this-mac", sample: "peer", want: "binding_mismatch"},
		{name: "peer cannot omit", machine: "peer", want: "binding_mismatch"},
		{name: "missing context", want: "binding_mismatch"},
		{name: "stale", machine: "this-mac", sample: "this-mac", age: 3 * time.Minute, want: "stale_session"},
		{name: "future", machine: "this-mac", sample: "this-mac", age: -time.Second, want: "stale_session"},
		{name: "reader failure", machine: "this-mac", sample: "this-mac", readErr: errors.New("inert"), want: "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := fabricHostFrom(guard.NodeCapacity{}, swaphygiene.Sample{}, apollo.TelemetryRead{State: "active", Telemetry: &apollo.Telemetry{EngineID: "same-engine", MachineID: tc.sample, EmittedAt: now.Add(-tc.age)}}, tc.readErr, nil, now, tc.machine)
			if h.EngineState != tc.want {
				t.Fatalf("engine binding state: got %s want %s", h.EngineState, tc.want)
			}
			if (h.Engine == "same-engine") != (tc.want == "observed_session") {
				t.Fatalf("foreign or unavailable session promoted to local: %+v", h)
			}
		})
	}
}
