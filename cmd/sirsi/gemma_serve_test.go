package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

// TestGemmaNeverAgainInvariants encodes the native SNE cutover so no future edit
// can silently restore the retired Python broker or its unsafe defaults.
func TestGemmaNeverAgainInvariants(t *testing.T) {
	// 1) The default concurrency must be 0 = AUTO-DERIVE from the node (ADR-031-B).
	//    It shipped as 4 (OOM'd the host), was hardened to a fixed 1 (#60), and is now
	//    node-derived: 0 means "use NodeCapacity.MaxConcurrency", which is RAM/VRAM-
	//    gated and floored at 1 — so auto is SAFE (bounded by the box), never the old
	//    aggressive fixed number. The footgun cannot return: a positive default would
	//    fail this, and the derivation itself refuses-or-floors (asserts 4/5 below).
	if dv := gemmaServeCmd.Flags().Lookup("concurrency").DefValue; dv != "0" {
		t.Errorf("default --concurrency must be 0 (auto-derive from the node, ADR-031-B); got %q", dv)
	}

	// 2) Pantheon and SNE share the portfolio-standard port. Python's historical
	//    broker used 8765; accepting that port here would revive a split runtime.
	if gemmaServerDefaultPort != 8477 {
		t.Errorf("native SNE port = %d, want 8477", gemmaServerDefaultPort)
	}

}

func TestGemmaBrokerQuarantineSurvivesSelfHealing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	agents := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(setup.GemmaBrokerPlistPath(), []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldLaunchctl := gemmaLaunchctl
	oldLoaded := gemmaLaunchdLoaded
	var calls [][]string
	gemmaLaunchctl = func(args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		return nil
	}
	gemmaLaunchdLoaded = func(string) bool { return false }
	t.Cleanup(func() {
		gemmaLaunchctl = oldLaunchctl
		gemmaLaunchdLoaded = oldLoaded
	})

	if err := gemmaServerQuarantine(home); err != nil {
		t.Fatal(err)
	}
	if setup.GemmaBrokerInstalled() || !setup.GemmaBrokerQuarantined() {
		t.Fatalf("installed=%v quarantined=%v, want false/true", setup.GemmaBrokerInstalled(), setup.GemmaBrokerQuarantined())
	}
	want := [][]string{
		{"disable", "gui/" + strconv.Itoa(os.Getuid()) + "/" + setup.GemmaBrokerLabel},
		{"bootout", "gui/" + strconv.Itoa(os.Getuid()) + "/" + setup.GemmaBrokerLabel},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("launchctl calls = %#v, want %#v", calls, want)
	}
}

func TestGemmaBrokerRestoreIsExplicitAndVerified(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Dir(setup.GemmaBrokerPlistPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(setup.GemmaBrokerQuarantinePath(), []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldLaunchctl, oldAwait := gemmaLaunchctl, gemmaAwaitWarmFn
	var calls [][]string
	gemmaLaunchctl = func(args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		return nil
	}
	gemmaAwaitWarmFn = func(string) error { return nil }
	t.Cleanup(func() {
		gemmaLaunchctl = oldLaunchctl
		gemmaAwaitWarmFn = oldAwait
	})

	if err := gemmaServerRestore(home); err != nil {
		t.Fatal(err)
	}
	if !setup.GemmaBrokerInstalled() || setup.GemmaBrokerQuarantined() {
		t.Fatalf("installed=%v quarantined=%v, want true/false", setup.GemmaBrokerInstalled(), setup.GemmaBrokerQuarantined())
	}
	want := [][]string{
		{"enable", "gui/" + strconv.Itoa(os.Getuid()) + "/" + setup.GemmaBrokerLabel},
		{"bootstrap", "gui/" + strconv.Itoa(os.Getuid()), setup.GemmaBrokerPlistPath()},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("launchctl calls = %#v, want %#v", calls, want)
	}
}

func TestGemmaBrokerRestoreReadinessFailureRequarantines(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Dir(setup.GemmaBrokerQuarantinePath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(setup.GemmaBrokerQuarantinePath(), []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldLaunchctl, oldLoaded, oldAwait := gemmaLaunchctl, gemmaLaunchdLoaded, gemmaAwaitWarmFn
	var calls [][]string
	gemmaLaunchctl = func(args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		return nil
	}
	gemmaLaunchdLoaded = func(string) bool { return false }
	gemmaAwaitWarmFn = func(string) error { return errors.New("candidate not ready") }
	t.Cleanup(func() {
		gemmaLaunchctl = oldLaunchctl
		gemmaLaunchdLoaded = oldLoaded
		gemmaAwaitWarmFn = oldAwait
	})

	err := gemmaServerRestore(home)
	if err == nil || !strings.Contains(err.Error(), "re-quarantined") {
		t.Fatalf("restore error = %v, want re-quarantined readiness failure", err)
	}
	if setup.GemmaBrokerInstalled() || !setup.GemmaBrokerQuarantined() {
		t.Fatalf("installed=%v quarantined=%v, want false/true", setup.GemmaBrokerInstalled(), setup.GemmaBrokerQuarantined())
	}
	want := [][]string{
		{"enable", "gui/" + strconv.Itoa(os.Getuid()) + "/" + setup.GemmaBrokerLabel},
		{"bootstrap", "gui/" + strconv.Itoa(os.Getuid()), setup.GemmaBrokerPlistPath()},
		{"disable", "gui/" + strconv.Itoa(os.Getuid()) + "/" + setup.GemmaBrokerLabel},
		{"bootout", "gui/" + strconv.Itoa(os.Getuid()) + "/" + setup.GemmaBrokerLabel},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("launchctl calls = %#v, want %#v", calls, want)
	}
}
