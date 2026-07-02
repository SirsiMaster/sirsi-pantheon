package tui

import "testing"

func TestDefaultRegistryBuilds(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry() error = %v", err)
	}
	if len(reg.IDs()) == 0 {
		t.Fatal("DefaultRegistry() registered no commands")
	}
}

// The §7 delta-2 guarantee, made executable: every status-bar hint each of the
// five screens advertises must resolve to a registered, keyed command. A screen
// that surfaces an unwired key fails here — a dead hint cannot ship. This is the
// keymap-completeness test.
func TestNoHintReferencesUnregisteredCommand(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry() error = %v", err)
	}
	screens := []Screen{
		newPulseScreen(), newWasteScreen(), newGhostsScreen(),
		newHealthScreen(), newActivityScreen(),
	}
	for _, s := range screens {
		if _, err := reg.Hints(s.HintIDs()); err != nil {
			t.Errorf("screen %q advertises an invalid hint: %v", s.Name(), err)
		}
	}
}

func TestHintsRejectsUnregisteredID(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(Command{ID: CmdInspect, Key: "enter", Hint: "inspect"}); err != nil {
		t.Fatalf("Register error = %v", err)
	}
	if _, err := reg.Hints([]CommandID{CmdInspect, CmdScan}); err == nil {
		t.Error("Hints() accepted an unregistered id; want error")
	}
}

func TestHintsRejectsPaletteOnlyCommand(t *testing.T) {
	reg := NewRegistry()
	// CmdScan has no key — palette-only. It must not be surfaceable as a hint.
	if err := reg.Register(Command{ID: CmdScan, Hint: "scan"}); err != nil {
		t.Fatalf("Register error = %v", err)
	}
	if _, err := reg.Hints([]CommandID{CmdScan}); err == nil {
		t.Error("Hints() accepted a palette-only (keyless) command; want error")
	}
}

func TestRegisterRejectsDuplicatesAndKeyClashes(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(Command{ID: CmdScan, Key: "s"}); err != nil {
		t.Fatalf("first Register error = %v", err)
	}
	if err := reg.Register(Command{ID: CmdScan, Key: "x"}); err == nil {
		t.Error("duplicate id accepted; want error")
	}
	if err := reg.Register(Command{ID: CmdClean, Key: "s"}); err == nil {
		t.Error("clashing key accepted; want error")
	}
	if err := reg.Register(Command{ID: ""}); err == nil {
		t.Error("empty id accepted; want error")
	}
}

func TestResolveKeyRoundTrips(t *testing.T) {
	reg, _ := DefaultRegistry()
	c, ok := reg.ResolveKey("c")
	if !ok {
		t.Fatal("ResolveKey(c) not found")
	}
	if c.ID != CmdClean {
		t.Errorf("ResolveKey(c) = %q, want %q", c.ID, CmdClean)
	}
	if !c.Destructive {
		t.Error("clean command must be flagged destructive")
	}
	if _, ok := reg.ResolveKey("nope"); ok {
		t.Error("ResolveKey(nope) returned a command; want miss")
	}
}
