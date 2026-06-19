package guard

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMostSevere(t *testing.T) {
	cases := []struct {
		name                   string
		normal, warn, critical bool
		want                   PressureLevel
	}{
		{"critical wins over all (coalesced)", true, true, true, PressureCritical},
		{"warn over normal", true, true, false, PressureWarn},
		{"normal only", true, false, false, PressureNormal},
		{"none set → unknown (ignored)", false, false, false, PressureUnknown},
		{"critical alone", false, false, true, PressureCritical},
	}
	for _, c := range cases {
		if got := mostSevere(c.normal, c.warn, c.critical); got != c.want {
			t.Errorf("%s: mostSevere=%v, want %v", c.name, got, c.want)
		}
	}
}

func TestMemTierFromPressure(t *testing.T) {
	// The kernel reports NORMAL/WARN/CRITICAL only; it can RAISE the tier to
	// suspend-governed but never decides a kill (TierEmergency stays free-%-driven).
	if memTierFromPressure(PressureCritical) != TierCritical ||
		memTierFromPressure(PressureWarn) != TierWarn ||
		memTierFromPressure(PressureNormal) != TierOK ||
		memTierFromPressure(PressureUnknown) != TierOK {
		t.Error("memTierFromPressure mapping wrong")
	}
}

func TestPressureCacheRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, _, ok := readPressureCache(); ok {
		t.Fatal("no cache file yet → must not be ok")
	}
	writePressureCache(PressureWarn)
	lvl, src, ok := readPressureCache()
	if !ok || lvl != PressureWarn || src != "kernel-dispatch" {
		t.Fatalf("round-trip: got (%v,%q,%v), want (warn,kernel-dispatch,true)", lvl, src, ok)
	}
}

func TestPressureCacheStaleIgnored(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Hand-write an entry older than pressureCacheStale: dispatch pressure is
	// transition-based, so a stale stamp means "no live daemon", not "still valid".
	old := pressureCacheEntry{Level: PressureCritical, Source: "kernel-dispatch", Unix: time.Now().Add(-2 * pressureCacheStale).Unix()}
	b, _ := json.Marshal(old)
	_ = os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755)
	if err := os.WriteFile(filepath.Join(home, ".sirsi", "pressure.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := readPressureCache(); ok {
		t.Error("a stale stamp must be ignored (no live watcher)")
	}
}

func TestEffectivePressurePrefersCacheOverBootstrap(t *testing.T) {
	lastPressureAuth.Store(false) // ensure the in-process branch is inactive
	t.Setenv("HOME", t.TempDir())
	// 50% free → bootstrap would say Normal, with no cache present.
	if lvl, src := effectivePressure(50, 100); lvl != PressureNormal || src != "bootstrap-snapshot" {
		t.Fatalf("no cache: got (%v,%q), want (normal,bootstrap-snapshot)", lvl, src)
	}
	// A fresh kernel cache at Critical must win even though free-% says Normal —
	// the kernel level is authoritative, the free-% seed is only the fallback.
	writePressureCache(PressureCritical)
	if lvl, src := effectivePressure(50, 100); lvl != PressureCritical || src != "kernel-dispatch" {
		t.Fatalf("fresh cache: got (%v,%q), want (critical,kernel-dispatch)", lvl, src)
	}
}

func TestEffectivePressureUnknownWithoutTotal(t *testing.T) {
	lastPressureAuth.Store(false)
	t.Setenv("HOME", t.TempDir())
	if lvl, src := effectivePressure(0, 0); lvl != PressureUnknown || src != "unknown" {
		t.Errorf("no total RAM, no cache: got (%v,%q), want (unknown,unknown)", lvl, src)
	}
}

// TestNewPressureWatcherSubscribeIsSafe runs LAST: on darwin+cgo it starts the
// real libdispatch source (which then stays resident), so keeping it after the
// cache tests guarantees a stray real pressure event can't flip the in-process
// atomic mid-test. Either way Subscribe must return nil and not block.
func TestNewPressureWatcherSubscribeIsSafe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan PressureEvent, 1)
	if err := NewPressureWatcher().Subscribe(ctx, ch); err != nil {
		t.Errorf("Subscribe returned error: %v", err)
	}
}
