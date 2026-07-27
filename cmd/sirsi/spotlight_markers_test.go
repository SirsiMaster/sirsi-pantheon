package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The value this guards is idempotence and blast radius: the plan must be
// identical whether or not a marker already exists, and it must never reach
// into a source tree.
func TestPlanAndApplyIndexMarkers(t *testing.T) {
	home := t.TempDir()
	dev := t.TempDir()

	// One churn path that exists, pre-marked; one that exists, unmarked.
	cache := filepath.Join(home, "Library", "Caches")
	mod := filepath.Join(home, "go", "pkg", "mod")
	for _, d := range []string{cache, mod} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(cache, markerFile), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	// A build tree that SHOULD be found, and source beside it that must not be.
	nm := filepath.Join(dev, "proj", "node_modules")
	src := filepath.Join(dev, "proj", "src")
	nested := filepath.Join(nm, "pkg", "node_modules") // must NOT be descended into
	for _, d := range []string{nested, src} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	plan := planIndexMarkers(home, dev)

	byPath := map[string]markerState{}
	for _, st := range plan {
		byPath[st.Path] = st
	}

	if !byPath[cache].Marked {
		t.Error("pre-marked cache dir should report Marked")
	}
	if byPath[mod].Marked {
		t.Error("unmarked module cache should not report Marked")
	}
	if _, ok := byPath[nm]; !ok {
		t.Errorf("node_modules not discovered under %s", dev)
	}
	if _, ok := byPath[src]; ok {
		t.Error("source tree was planned for marking — code search must stay intact")
	}
	if _, ok := byPath[nested]; ok {
		t.Error("descended into a matched build dir; marking the parent already covers it")
	}

	applied, err := applyIndexMarkers(plan)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	wrote := 0
	for _, st := range applied {
		if st.WroteNow {
			wrote++
		}
	}
	if wrote == 0 {
		t.Fatal("apply wrote nothing")
	}
	if _, statErr := os.Stat(filepath.Join(mod, markerFile)); statErr != nil {
		t.Errorf("module cache not marked after apply: %v", statErr)
	}

	// Idempotence: a second apply must write nothing at all.
	again, err := applyIndexMarkers(planIndexMarkers(home, dev))
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	for _, st := range again {
		if st.WroteNow {
			t.Errorf("second apply rewrote %s — not idempotent", st.Path)
		}
	}
}
