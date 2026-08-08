package guard

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
	if err := os.WriteFile(filepath.Join(cache, MarkerFile), nil, 0o644); err != nil {
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

	plan := PlanIndexMarkers(home, dev)

	byPath := map[string]MarkerState{}
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

	applied, err := ApplyIndexMarkers(plan)
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
	if _, statErr := os.Stat(filepath.Join(mod, MarkerFile)); statErr != nil {
		t.Errorf("module cache not marked after apply: %v", statErr)
	}

	// Idempotence: a second apply must write nothing at all.
	again, err := ApplyIndexMarkers(PlanIndexMarkers(home, dev))
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	for _, st := range again {
		if st.WroteNow {
			t.Errorf("second apply rewrote %s — not idempotent", st.Path)
		}
	}
}

// The defect this pins: discovery matched on basename alone, so a source
// repository that happens to be named `dist` or `target` was planned for
// marking — silently deleting a real source tree from Spotlight, the exact
// outcome the feature promises never to cause.
func TestDiscoveryRequiresBuildOutputEvidence(t *testing.T) {
	dev := t.TempDir()

	mk := func(parts ...string) string {
		p := filepath.Join(append([]string{dev}, parts...)...)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	touch := func(p string) {
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// A source repository that is simply NAMED dist. No manifest, has its own
	// .git. This is the case that regressed.
	repoDist := mk("checkouts", "dist")
	mk("checkouts", "dist", ".git")

	// A source repository named target, also its own checkout.
	repoTarget := mk("checkouts", "target")
	mk("checkouts", "target", ".git")

	// Genuine build output: dist beside a package.json, target beside Cargo.toml.
	realDist := mk("webapp", "dist")
	touch(filepath.Join(dev, "webapp", "package.json"))
	realTarget := mk("rustapp", "target")
	touch(filepath.Join(dev, "rustapp", "Cargo.toml"))

	// A dist with neither manifest nor .git — no evidence at all, so refused.
	bareDist := mk("mystery", "dist")

	// node_modules stays unambiguous and needs no manifest.
	nm := mk("anything", "node_modules")

	found := map[string]bool{}
	for _, p := range DiscoverBuildDirs(dev, 4) {
		found[p] = true
	}

	for _, p := range []string{realDist, realTarget, nm} {
		if !found[p] {
			t.Errorf("real build output not discovered: %s", p)
		}
	}
	for _, p := range []string{repoDist, repoTarget, bareDist} {
		if found[p] {
			t.Errorf("marked a source tree with no build evidence: %s", p)
		}
	}
}

// A name match that fails the evidence gate must still stop the walk, so a
// vendored checkout named dist does not become a full recursive descent.
func TestUnprovenBuildNameIsNotDescended(t *testing.T) {
	dev := t.TempDir()
	inner := filepath.Join(dev, "checkouts", "dist", "vendor", "node_modules")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dev, "checkouts", "dist", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range DiscoverBuildDirs(dev, 5) {
		if p == inner {
			t.Fatalf("descended into a refused build-name dir: %s", p)
		}
	}
}

// The preview→apply window. A candidate that is replaced between planning and
// writing must be refused, not written through: the plan records (dev, ino) and
// the write path re-reads it, so a path that now resolves elsewhere is skipped
// with a reason instead of silently marking the attacker's target.
func TestApplyRefusesPathChangedAfterPreview(t *testing.T) {
	home := t.TempDir()
	dev := t.TempDir()

	mod := filepath.Join(home, "go", "pkg", "mod")
	if err := os.MkdirAll(mod, 0o755); err != nil {
		t.Fatal(err)
	}

	plan := PlanIndexMarkers(home, dev)

	// Swap the previewed directory for a different one at the same path.
	elsewhere := filepath.Join(home, "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(mod); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, mod); err != nil {
		t.Fatal(err)
	}

	applied, err := ApplyIndexMarkers(plan)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	var st MarkerState
	for _, s := range applied {
		if s.Path == mod {
			st = s
		}
	}
	if st.WroteNow {
		t.Error("wrote through a path that changed after preview")
	}
	if st.Skipped == "" {
		t.Error("redirected path was skipped without saying why")
	}
	if _, statErr := os.Stat(filepath.Join(elsewhere, MarkerFile)); statErr == nil {
		t.Error("marker landed in the redirect target — the recheck did not hold")
	}
}

// Removal is the reverse gesture and must be as conservative as the write:
// every marker removed, nothing else touched, and idempotent on a second pass.
func TestRemoveIndexMarkers(t *testing.T) {
	home := t.TempDir()
	dev := t.TempDir()

	mod := filepath.Join(home, "go", "pkg", "mod")
	if err := os.MkdirAll(mod, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyIndexMarkers(PlanIndexMarkers(home, dev)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(mod, MarkerFile)); err != nil {
		t.Fatalf("setup did not mark: %v", err)
	}

	removed, err := RemoveIndexMarkers(PlanIndexMarkers(home, dev))
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	n := 0
	for _, st := range removed {
		if st.Removed {
			n++
		}
	}
	if n == 0 {
		t.Fatal("remove unlinked nothing")
	}
	if _, statErr := os.Stat(filepath.Join(mod, MarkerFile)); !os.IsNotExist(statErr) {
		t.Errorf("marker survived removal: %v", statErr)
	}

	again, err := RemoveIndexMarkers(PlanIndexMarkers(home, dev))
	if err != nil {
		t.Fatalf("second remove: %v", err)
	}
	for _, st := range again {
		if st.Removed {
			t.Errorf("second remove re-unlinked %s — not idempotent", st.Path)
		}
	}
}
