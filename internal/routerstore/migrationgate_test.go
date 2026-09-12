package routerstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A dirty build must not migrate. This is the sequence that took the fleet down
// on 2026-08-05: source in a working tree only, migration applied to shared
// state, every peer binary fail-closed with no commit to rebuild from.
func TestDirtyBuildIsRefused(t *testing.T) {
	restore := stubBuild(buildStamp{Revision: "abc123def456789", Dirty: true, Known: true})
	defer restore()
	t.Setenv("SIRSI_ALLOW_DIRTY_MIGRATION", "")

	err := checkMigrationAllowed(7, 8, sharedPathForTest(t))
	if err == nil {
		t.Fatal("checkMigrationAllowed() = nil for a dirty build; the migration that broke the fleet would be allowed again")
	}
	for _, want := range []string{"uncommitted changes", "commit and push", "SIRSI_ALLOW_DIRTY_MIGRATION"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must tell the operator how to proceed; missing %q in:\n%s", want, err)
		}
	}
}

// Clean builds are the normal path and must not be impeded.
func TestCleanBuildIsAllowed(t *testing.T) {
	restore := stubBuild(buildStamp{Revision: "abc123def456789", Dirty: false, Known: true})
	defer restore()

	if err := checkMigrationAllowed(7, 8, sharedPathForTest(t)); err != nil {
		t.Errorf("checkMigrationAllowed() = %v for a clean build, want nil", err)
	}
}

// `go test` and `go run` produce no VCS stamp. A gate that blocked them would
// fail every test that opens a store, and a gate nobody can run gets deleted.
// Refuse on PROVEN dirty, not on unproven clean.
func TestUnstampedBuildIsAllowed(t *testing.T) {
	restore := stubBuild(buildStamp{Known: false})
	defer restore()

	if err := checkMigrationAllowed(7, 8, sharedPathForTest(t)); err != nil {
		t.Errorf("checkMigrationAllowed() = %v for an unstamped build, want nil — this is every test binary", err)
	}
}

// The override exists so a deliberate operator is not stuck, but it must be
// explicit and loud rather than a silent default.
func TestOverrideAllowsDirtyMigration(t *testing.T) {
	restore := stubBuild(buildStamp{Revision: "deadbeefcafe", Dirty: true, Known: true})
	defer restore()
	t.Setenv("SIRSI_ALLOW_DIRTY_MIGRATION", "1")

	if err := checkMigrationAllowed(7, 8, sharedPathForTest(t)); err != nil {
		t.Errorf("explicit override should permit the migration, got %v", err)
	}
}

// The whole point of provenance: the refusal a peer sees must name the build,
// not pose a forensic puzzle. Finding it by hand cost an hour.
func TestTooNewErrorNamesTheCulprit(t *testing.T) {
	err := tooNewError(14, 7, "v14 by 2f88a673aa11 (DIRTY — uncommitted changes) at 2026-08-06T03:32:00Z")
	for _, want := range []string{"schema version 14", "max 7", "Migrated by", "2f88a673aa11", "DIRTY"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in refusal:\n%s", want, err)
		}
	}
}

func TestTooNewErrorSaysUnknownWhenUnrecorded(t *testing.T) {
	err := tooNewError(14, 7, "")
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("an unrecorded migration must say so plainly, got:\n%s", err)
	}
}

func TestBuildStampRendersDirtyVisibly(t *testing.T) {
	if got := (buildStamp{Revision: "abcdef0123456789", Dirty: true, Known: true}).String(); !strings.Contains(got, "DIRTY") {
		t.Errorf("dirty must be visible in the stamp, got %q", got)
	}
	if got := (buildStamp{Known: false}).String(); !strings.Contains(got, "unstamped") {
		t.Errorf("unstamped must be visible, got %q", got)
	}
}

// stubBuild swaps the build-stamp source so the gate is testable without
// rebuilding this binary under different VCS conditions.
func stubBuild(b buildStamp) func() {
	prev := currentBuildFn
	currentBuildFn = func() buildStamp { return b }
	return func() { currentBuildFn = prev }
}

var _ = os.Getenv

func sharedPathForTest(t *testing.T) string {
	t.Helper()
	p, err := LocalPath()
	if err != nil {
		t.Skipf("cannot resolve the canonical store path here: %v", err)
	}
	return p
}

// THE fix. A dirty build migrating a PRIVATE store strands nobody, because no
// peer binary will ever open it. Refusing here made every test that opens a
// fresh store fail whenever the tree was dirty, which is nearly always.
func TestDirtyBuildMayMigrateAPrivateStore(t *testing.T) {
	restore := stubBuild(buildStamp{Revision: "abc123def456789", Dirty: true, Known: true})
	defer restore()
	t.Setenv("SIRSI_ALLOW_DIRTY_MIGRATION", "")

	if err := checkMigrationAllowed(0, 1, filepath.Join(t.TempDir(), "router.db")); err != nil {
		t.Errorf("refused a private test store: %v", err)
	}
}

// Unknown is not evidence of safety, and the write being gated is one-way.
func TestEmptyStorePathIsTreatedAsShared(t *testing.T) {
	restore := stubBuild(buildStamp{Revision: "abc123def456789", Dirty: true, Known: true})
	defer restore()
	t.Setenv("SIRSI_ALLOW_DIRTY_MIGRATION", "")

	if err := checkMigrationAllowed(7, 8, ""); err == nil {
		t.Error("an unknown store path was allowed — unknown must fail toward refusing for a one-way write")
	}
}

// The gate must not be walkable with a different spelling of the same path.
func TestUnnormalisedCanonicalPathIsStillRefused(t *testing.T) {
	restore := stubBuild(buildStamp{Revision: "abc123def456789", Dirty: true, Known: true})
	defer restore()
	t.Setenv("SIRSI_ALLOW_DIRTY_MIGRATION", "")

	p := sharedPathForTest(t)
	if err := checkMigrationAllowed(7, 8, filepath.Join(filepath.Dir(p), ".", filepath.Base(p))); err == nil {
		t.Error("an unnormalised spelling of the canonical store was allowed — the gate is walkable")
	}
}

// TestDirtyFreshInitPointsAtEnvNotMigration (rs-29): a dirty-build refusal on a
// FRESH store init (from==0) must tell the operator they are almost certainly
// not pointed at the router service and to fix the environment — NOT chase a
// phantom migration to commit/push. A real migration (from>0) must NOT carry
// that fresh-init note.
func TestDirtyFreshInitPointsAtEnvNotMigration(t *testing.T) {
	restore := stubBuild(buildStamp{Revision: "abc123def456789", Dirty: true, Known: true})
	defer restore()
	t.Setenv("SIRSI_ALLOW_DIRTY_MIGRATION", "")

	freshErr := checkMigrationAllowed(0, 1, "") // "" = shared (deterministic, no host dependency)
	if freshErr == nil {
		t.Fatal("dirty fresh-init on a shared/unknown store must be refused")
	}
	for _, want := range []string{"FRESH local store", "SIRSI_ROUTER_URL", "SIRSI_ROUTER_DB", "canonical store"} {
		if !strings.Contains(freshErr.Error(), want) {
			t.Errorf("fresh-init refusal must point at the env fix; missing %q in:\n%s", want, freshErr)
		}
	}

	// Fresh-init must NOT carry the migration remediation — that contradictory
	// "commit and push" advice is exactly what rs-29 removes (SSA review).
	if strings.Contains(freshErr.Error(), "commit and push") {
		t.Errorf("fresh-init refusal must NOT tell the operator to commit and push a migration:\n%s", freshErr)
	}

	migErr := checkMigrationAllowed(19, 20, "")
	if migErr == nil {
		t.Fatal("dirty migration must be refused")
	}
	if strings.Contains(migErr.Error(), "FRESH local store") {
		t.Errorf("a real migration (from>0) must NOT carry the fresh-init note:\n%s", migErr)
	}
	if !strings.Contains(migErr.Error(), "commit and push") {
		t.Errorf("a real migration (from>0) must keep the commit-and-push remediation:\n%s", migErr)
	}
}
