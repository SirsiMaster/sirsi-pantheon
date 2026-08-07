package guard

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
)

// cutoverRoot builds a router root inside a temp HOME with the store-wake marker
// present, so the check runs its post-cutover branch without touching the real
// host state.
func cutoverRoot(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Env wins over the marker, so set it explicitly rather than relying on a
	// marker file the helper would also have to create.
	t.Setenv(routercfg.StoreWakeEnv, "1")

	root := filepath.Join(home, "Development", "sirsi-pantheon", ".agents", "idea-router")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// Run from a neutral cwd so the walk-up cannot find some ancestor router and
	// the HOME fallback is what resolves.
	t.Chdir(home)
	return root
}

func findingFor(r DoctorReport, check string) (DiagnosticFinding, bool) {
	for _, f := range r.Findings {
		if f.Check == check {
			return f, true
		}
	}
	return DiagnosticFinding{}, false
}

// The healthy post-cutover shape: store is authority, no legacy file at all.
func TestThreadRegistrySplit_NoLegacyFileIsOK(t *testing.T) {
	cutoverRoot(t)
	var r DoctorReport
	checkThreadRegistrySplit(&platform.Mock{}, &r)

	f, ok := findingFor(r, "Thread Registry Split")
	if !ok {
		t.Fatal("check produced no finding")
	}
	if f.Severity != SeverityOK {
		t.Errorf("severity = %v, want OK when no legacy registry exists: %s", f.Severity, f.Message)
	}
}

// THE case this check exists for: a legacy registry written RECENTLY while the
// cutover is active means something is still writing it — two live registries.
func TestThreadRegistrySplit_RecentlyWrittenLegacyIsCritical(t *testing.T) {
	root := cutoverRoot(t)
	if err := os.WriteFile(filepath.Join(root, "threads.json"),
		[]byte(`{"threads":{"thr-a":{},"thr-b":{},"thr-c":{}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var r DoctorReport
	checkThreadRegistrySplit(&platform.Mock{}, &r)

	f, _ := findingFor(r, "Thread Registry Split")
	if f.Severity != SeverityCritical {
		t.Fatalf("severity = %v, want CRITICAL for a live second registry: %s", f.Severity, f.Message)
	}
	if !contains(f.Message, "3") {
		t.Errorf("message omits the record count, so an operator cannot gauge the split: %s", f.Message)
	}
}

// A long-untouched legacy file is a leftover, not an active second writer.
// Reporting it CRITICAL would cry wolf on every host that predates the cutover.
func TestThreadRegistrySplit_StaleLegacyIsWarnNotCritical(t *testing.T) {
	root := cutoverRoot(t)
	p := filepath.Join(root, "threads.json")
	if err := os.WriteFile(p, []byte(`{"threads":{"thr-a":{}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}

	var r DoctorReport
	checkThreadRegistrySplit(&platform.Mock{}, &r)

	f, _ := findingFor(r, "Thread Registry Split")
	if f.Severity != SeverityWarn {
		t.Errorf("severity = %v, want WARN for a stale leftover: %s", f.Severity, f.Message)
	}
}

// Before the cutover the legacy file IS the registry. Flagging it would be a
// false alarm on every pre-cutover host.
func TestThreadRegistrySplit_PreCutoverIsOK(t *testing.T) {
	root := cutoverRoot(t)
	t.Setenv(routercfg.StoreWakeEnv, "0")
	if err := os.WriteFile(filepath.Join(root, "threads.json"),
		[]byte(`{"threads":{"thr-a":{}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var r DoctorReport
	checkThreadRegistrySplit(&platform.Mock{}, &r)

	f, _ := findingFor(r, "Thread Registry Split")
	if f.Severity != SeverityOK {
		t.Errorf("severity = %v, want OK pre-cutover: %s", f.Severity, f.Message)
	}
}

// An unreadable legacy file is UNKNOWN, never healthy — a parse failure must not
// be reported as "no second registry".
func TestThreadRegistrySplit_UnreadableLegacyIsWarnNotOK(t *testing.T) {
	root := cutoverRoot(t)
	if err := os.WriteFile(filepath.Join(root, "threads.json"), []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}

	var r DoctorReport
	checkThreadRegistrySplit(&platform.Mock{}, &r)

	f, _ := findingFor(r, "Thread Registry Split")
	if f.Severity == SeverityOK {
		t.Errorf("an unreadable legacy registry reported OK — absence of evidence rendered as evidence of absence: %s", f.Message)
	}
}
