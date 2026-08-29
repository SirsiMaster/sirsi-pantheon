package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClassifyHostReachabilityDoesNotRequireActivePeerMetadata(t *testing.T) {
	got := classifyHostReachability(hostProbeResult{TailscalePing: true, GUIState: "locked", SNEState: "ready"})
	if !got.TransportReachable || got.Classification != "reachable-sne-ready" {
		t.Fatalf("successful bounded probe was not authoritative: %+v", got)
	}
	if got.GUIState != "locked" {
		t.Fatalf("GUI lock must remain independent: %+v", got)
	}
}

func TestHostReachabilityCommandSeparatesTransportGUIAndSNE(t *testing.T) {
	oldTCP, oldRun := hostReachabilityTCP, hostReachabilityRun
	t.Cleanup(func() { hostReachabilityTCP, hostReachabilityRun = oldTCP, oldRun })
	hostReachabilityTCP = func(_ context.Context, address string) bool {
		return strings.HasSuffix(address, ":22") || strings.HasSuffix(address, ":5900")
	}
	hostReachabilityRun = func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if strings.Contains(name, "Tailscale") {
			return []byte("pong from m1 in 7ms"), nil
		}
		return []byte("LOCK=\"IOConsoleLocked\" = Yes\nREADY={\"status\":\"ready\"}\n"), nil
	}

	command := newHostReachabilityCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"--host", "100.88.242.95", "--ssh-user", "sirsimasterdev"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	var got hostProbeResult
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.TransportReachable || !got.TailscalePing || !got.SSHReachable || !got.ScreenSharing {
		t.Fatalf("transport probes were not preserved: %+v", got)
	}
	if got.GUIState != "locked" || got.SNEState != "ready" || got.Classification != "reachable-sne-ready" {
		t.Fatalf("independent states were conflated: %+v", got)
	}
}

func TestHostReachabilityRequiresBoundedInput(t *testing.T) {
	command := newHostReachabilityCommand()
	command.SetArgs(nil)
	if err := command.Execute(); err == nil {
		t.Fatal("missing host must fail closed")
	}
	command = newHostReachabilityCommand()
	command.SetArgs([]string{"--host", "m1", "--timeout", "31s"})
	if err := command.Execute(); err == nil {
		t.Fatal("unbounded timeout must fail closed")
	}
}

func TestHostReachabilitySlowTailscaleDoesNotStarveTCP(t *testing.T) {
	oldTCP, oldRun := hostReachabilityTCP, hostReachabilityRun
	t.Cleanup(func() { hostReachabilityTCP, hostReachabilityRun = oldTCP, oldRun })
	var tcpCalls atomic.Int32
	hostReachabilityTCP = func(_ context.Context, _ string) bool {
		tcpCalls.Add(1)
		return true
	}
	hostReachabilityRun = func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	command := newHostReachabilityCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"--host", "m1", "--timeout", "10ms"})
	started := time.Now()
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("bounded concurrent probes exceeded deadline")
	}
	var got hostProbeResult
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if tcpCalls.Load() != 2 || !got.TransportReachable || got.TailscalePing {
		t.Fatalf("slow Tailscale starved independent TCP evidence: %+v calls=%d", got, tcpCalls.Load())
	}
}
