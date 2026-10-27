package router

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestCollectNodeStatusScopesLocalOSAndWatcherProbes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_ROUTER_STORE_WAKE", "0")
	installFakeAgentCLIs(t)
	repoRoot := setupNodeTestRouter(t)
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
	now := time.Now().UTC()
	makeThread := func(id, agent, surface, host, machine string) *Thread {
		return &Thread{
			ThreadID: id, AgentID: agent, Surface: surface, Host: host, MachineID: machine,
			PID: 424242, StartTime: "fixture-start", Status: ThreadStatusActive,
			StartedAt: now.Add(-2 * time.Minute), LastSeenAt: now.Add(-time.Minute),
		}
	}
	threads := map[string]*Thread{
		"thr-local-loop":            makeThread("thr-local-loop", "claude-local", "claude", "host-local", "machine-local"),
		"thr-foreign-loop":          makeThread("thr-foreign-loop", "claude-foreign", "claude", "host-foreign", "machine-foreign"),
		"thr-missing-machine":       makeThread("thr-missing-machine", "claude-legacy", "claude", "host-legacy", ""),
		"thr-foreign-app-heartbeat": makeThread("thr-foreign-app-heartbeat", "codex-foreign", "codex", "host-foreign", "machine-foreign"),
	}
	statePath := filepath.Join(routerRoot, "state.json")
	stateBytes, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var state State
	if err := json.Unmarshal(stateBytes, &state); err != nil {
		t.Fatal(err)
	}
	state.Pending["claude-foreign"] = []string{"foreign-pending"}
	state.Pending["claude-legacy"] = []string{"unknown-pending"}
	stateBytes, err = json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, stateBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	registryBytes, err := json.MarshalIndent(&ThreadRegistry{Threads: threads}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(routerRoot, threadsFilename)
	if err := os.WriteFile(registryPath, registryBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	currentMachine := "machine-local"
	oldMachineProbe := getMachineIDFn()
	setMachineIDFn(func() string { return currentMachine })
	t.Cleanup(func() { setMachineIDFn(oldMachineProbe) })
	pidCalls, startCalls := map[int]int{}, map[int]int{}
	oldPIDProbe, oldStartProbe := getPIDStateFn(), getPIDStartFn()
	setPIDStateFn(func(pid int) PIDState {
		pidCalls[pid]++
		return PIDAlive
	})
	setPIDStartFn(func(pid int) string {
		startCalls[pid]++
		return "fixture-start"
	})
	t.Cleanup(func() { setPIDStateFn(oldPIDProbe); setPIDStartFn(oldStartProbe) })
	watcherCalls, watcherAgentCalls := map[string]int{}, map[string]int{}
	oldWatcherProbe, oldWatcherAgentProbe := getWatcherAliveFn(), getWatcherAliveByAgentFn()
	setWatcherAliveFn(func(threadID string) bool {
		watcherCalls[threadID]++
		return threadID == "thr-local-loop"
	})
	setWatcherAliveByAgentFn(func(agentID string) bool {
		watcherAgentCalls[agentID]++
		return false
	})
	t.Cleanup(func() { setWatcherAliveFn(oldWatcherProbe); setWatcherAliveByAgentFn(oldWatcherAgentProbe) })
	launchctlMissing := func(...string) error { return os.ErrNotExist }

	collect := func() *NodeStatus {
		t.Helper()
		ns, err := CollectNodeStatus(repoRoot, launchctlMissing, mockAuthProbe(true, false, ""))
		if err != nil {
			t.Fatalf("CollectNodeStatus: %v", err)
		}
		return ns
	}
	find := func(ns *NodeStatus, id string) *ThreadSummary {
		for _, rows := range [][]ThreadSummary{ns.LiveThreads, ns.StaleThreads} {
			for i := range rows {
				if rows[i].ThreadID == id {
					return &rows[i]
				}
			}
		}
		return nil
	}
	findUnverified := func(ns *NodeStatus, agent string) *UnverifiedInboxAgent {
		for i := range ns.UnverifiedInbox {
			if ns.UnverifiedInbox[i].AgentID == agent {
				return &ns.UnverifiedInbox[i]
			}
		}
		return nil
	}
	inStranded := func(ns *NodeStatus, agent string) bool {
		for _, row := range ns.StrandedInbox {
			if row.AgentID == agent {
				return true
			}
		}
		return false
	}
	assertRegistryUnchanged := func() {
		t.Helper()
		got, err := os.ReadFile(registryPath)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, registryBytes) {
			t.Fatal("CollectNodeStatus changed the canonical thread registry bytes")
		}
	}

	ns := collect()
	local := find(ns, "thr-local-loop")
	if local == nil || local.OSScope != threadOSScopeLocal || local.Host != "host-local" || local.MachineID != "machine-local" || local.OSState != PIDAlive || local.LoopState != "alive" || !local.Armed {
		t.Fatalf("known local thread projection = %+v", local)
	}
	foreign := find(ns, "thr-foreign-loop")
	if foreign == nil || foreign.OSScope != threadOSScopeRemote || foreign.OSState != PIDUnknown || foreign.LoopState != "unknown" || foreign.Armed || foreign.Stale || foreign.ArmedReason != "process-scope-remote" {
		t.Fatalf("fresh foreign loop must remain scope-unknown, not dead: %+v", foreign)
	}
	missing := find(ns, "thr-missing-machine")
	if missing == nil || missing.OSScope != threadOSScopeUnknown || missing.OSState != PIDUnknown || missing.LoopState != "unknown" || missing.Armed || missing.Stale {
		t.Fatalf("missing machine ID must remain unknown without losing heartbeat freshness: %+v", missing)
	}
	foreignHeartbeat := find(ns, "thr-foreign-app-heartbeat")
	if foreignHeartbeat == nil || foreignHeartbeat.OSScope != threadOSScopeRemote || foreignHeartbeat.OSState != PIDUnknown || foreignHeartbeat.LoopState != "na" || !foreignHeartbeat.Armed || foreignHeartbeat.Stale {
		t.Fatalf("foreign fresh app heartbeat was not preserved with explicit scope: %+v", foreignHeartbeat)
	}
	if row := findUnverified(ns, "claude-foreign"); row == nil || row.OpenItems != 1 || row.OSScope != threadOSScopeRemote || inStranded(ns, "claude-foreign") {
		t.Fatalf("foreign pending loop must remain explicitly unverified, not armed/dead-stranded: %+v stranded=%v", row, inStranded(ns, "claude-foreign"))
	}
	if row := findUnverified(ns, "claude-legacy"); row == nil || row.OpenItems != 1 || row.OSScope != threadOSScopeUnknown || inStranded(ns, "claude-legacy") {
		t.Fatalf("unidentified pending loop must remain explicitly unverified, not armed/dead-stranded: %+v stranded=%v", row, inStranded(ns, "claude-legacy"))
	}
	if pidCalls[424242] != 1 || startCalls[424242] != 1 {
		t.Fatalf("same PID probed %d times (start probe %d); want local row only", pidCalls[424242], startCalls[424242])
	}
	if watcherCalls["thr-local-loop"] != 1 || watcherCalls["thr-foreign-loop"] != 0 || watcherCalls["thr-missing-machine"] != 0 || len(watcherAgentCalls) != 0 {
		t.Fatalf("watcher probes escaped local scope: thread=%v agent=%v", watcherCalls, watcherAgentCalls)
	}
	assertRegistryUnchanged()

	// An unavailable local stable ID makes every row unidentifiable, even when
	// the record has a nonempty machine ID and its PID matches a known local one.
	currentMachine = ""
	for pid := range pidCalls {
		delete(pidCalls, pid)
		delete(startCalls, pid)
	}
	for id := range watcherCalls {
		delete(watcherCalls, id)
	}
	for id := range watcherAgentCalls {
		delete(watcherAgentCalls, id)
	}
	unknownNS := collect()
	for _, id := range []string{"thr-local-loop", "thr-foreign-loop", "thr-missing-machine", "thr-foreign-app-heartbeat"} {
		row := find(unknownNS, id)
		if row == nil || row.OSScope != threadOSScopeUnknown || row.OSState != PIDUnknown {
			t.Fatalf("missing current machine ID did not force unknown scope for %s: %+v", id, row)
		}
	}
	for _, agent := range []string{"claude-foreign", "claude-legacy"} {
		row := findUnverified(unknownNS, agent)
		if row == nil || row.OSScope != threadOSScopeUnknown || inStranded(unknownNS, agent) {
			t.Fatalf("pending lane %s lost explicit unknown scope: %+v stranded=%v", agent, row, inStranded(unknownNS, agent))
		}
	}
	if len(pidCalls) != 0 || len(startCalls) != 0 || len(watcherCalls) != 0 || len(watcherAgentCalls) != 0 {
		t.Fatalf("unknown current machine ID invoked local probes: pid=%v start=%v watcher=%v agent=%v", pidCalls, startCalls, watcherCalls, watcherAgentCalls)
	}
	assertRegistryUnchanged()
}
