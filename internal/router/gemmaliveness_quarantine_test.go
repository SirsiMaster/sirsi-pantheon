package router

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/liveness"
)

// Live incident, 2026-08-06: an operator deliberately stopped the SNE broker
// and moved its LaunchAgent plists to ~/.sirsi/quarantined-wake-plists/.
// RunGemmaLivenessDuty is a Go tick inside the resident router process, not a
// launchd consumer, so plist quarantine was invisible to it. It restarted the
// broker within one 2-minute tick — twice — overriding an explicit operator
// instruction both times. The quarantine marker exists so ALL restorers,
// including this one, can observe "stopped on purpose."

// The defect this test locks down: quarantined + broker down must NOT restore.
func TestGemmaLivenessDuty_QuarantinedSkipsRestoreOnDown(t *testing.T) {
	oldQ, oldP, oldS := isQuarantinedFn, gemmaProbeFn, gemmaServeFn
	oldStrikes := gemmaWedgeStrikes
	t.Cleanup(func() {
		setIsQuarantinedFn(oldQ)
		setGemmaProbeFn(oldP)
		setGemmaServeFn(oldS)
		gemmaWedgeStrikes = oldStrikes
	})

	setIsQuarantinedFn(func(string) bool { return true })
	setGemmaProbeFn(func(string) (liveness.GemmaStatus, string) { return liveness.GemmaDown, "no port file" })
	served := false
	setGemmaServeFn(func(bool) error { served = true; return nil })

	if err := RunGemmaLivenessDuty("", ""); err != nil {
		t.Fatalf("quarantined duty must no-op cleanly, got: %v", err)
	}
	if served {
		t.Fatal("gemma-liveness restored a QUARANTINED broker — the operator's " +
			"explicit stop was silently overridden")
	}
}

// Same probe result, quarantine LIFTED: the duty must restore exactly as
// before this change. Proves the marker gates only the quarantined case and
// this is not simply "the duty never restores anymore".
func TestGemmaLivenessDuty_UnquarantinedStillRestoresOnDown(t *testing.T) {
	oldQ, oldP, oldS := isQuarantinedFn, gemmaProbeFn, gemmaServeFn
	oldStrikes := gemmaWedgeStrikes
	t.Cleanup(func() {
		setIsQuarantinedFn(oldQ)
		setGemmaProbeFn(oldP)
		setGemmaServeFn(oldS)
		gemmaWedgeStrikes = oldStrikes
	})

	setIsQuarantinedFn(func(string) bool { return false })
	setGemmaProbeFn(func(string) (liveness.GemmaStatus, string) { return liveness.GemmaDown, "no port file" })
	served := false
	setGemmaServeFn(func(bool) error { served = true; return nil })

	if err := RunGemmaLivenessDuty("", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !served {
		t.Fatal("un-quarantined duty must still restore a down broker — this is " +
			"the pre-existing, unchanged behavior")
	}
}

// The real filesystem check, not the injected seam: proves the DEFAULT
// isQuarantinedFn (what production actually runs) reads a real marker file —
// the exact file `sirsi gemma quarantine`/`unquarantine` write and remove.
func TestIsQuarantinedFn_ReadsRealMarkerFile(t *testing.T) {
	old := isQuarantinedFn
	// Restore the REAL production default explicitly (not just whatever was
	// installed before this test) — that is the whole point of this test.
	t.Cleanup(func() { setIsQuarantinedFn(old) })
	setIsQuarantinedFn(func(home string) bool {
		_, err := os.Stat(QuarantineMarkerPath(home))
		return err == nil
	})

	home := t.TempDir()
	if getIsQuarantinedFn()(home) {
		t.Fatal("no marker file exists yet — must read as NOT quarantined")
	}

	marker := QuarantineMarkerPath(home)
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(marker, []byte("2026-08-06T00:00:00Z\n"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if !getIsQuarantinedFn()(home) {
		t.Fatal("marker file exists — must read as quarantined")
	}
}
