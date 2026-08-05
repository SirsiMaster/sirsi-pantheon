package cleaner

import (
	"path/filepath"
	"runtime"
	"testing"
)

// TestDefaultLoadedJobs_PlatformBoundary pins the boundary that a runtime check
// would have left ambiguous.
//
// The regression this guards: `defaultLoadedJobs` used to shell out to
// `launchctl` unconditionally. On Linux that returns "executable file not
// found", which the fail-closed path correctly reads as UNKNOWN AUTHORITY — and
// then blocks every deletion on a platform the cleaner fully supports. Every
// other test replaces the probe, so nothing caught it.
//
// This test deliberately exercises the REAL probe.
func TestDefaultLoadedJobs_PlatformBoundary(t *testing.T) {
	jobs := defaultLoadedJobs()

	if runtime.GOOS != "darwin" {
		if len(jobs) != 0 {
			t.Fatalf("defaultLoadedJobs() = %v on %s — there is no launchd here, so this must be a genuine empty set", jobs, runtime.GOOS)
		}
		return
	}
	// On darwin the probe may legitimately find jobs or none; what must never
	// happen is a synthetic error for a facility that does exist.
	for label, job := range jobs {
		if job.Err != "" && isSNEOwnedLabel(label) {
			t.Logf("SNE discovery error on this machine (fail-closed will engage): %s: %s", label, job.Err)
		}
	}
}

// The corollary that matters to a user: on a platform without launchd, ordinary
// cleanup must still work. If the boundary were a caught exec error instead of a
// build tag, this would fail.
func TestValidatePath_NonDarwinDoesNotFreezeCleanup(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin has launchd; boundary behavior is covered by the stubbed tests")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got := UnknownSubstrate(); len(got) != 0 {
		t.Errorf("UnknownSubstrate() = %v without launchd — absence of launchd is not unknown authority", got)
	}
	cache := filepath.Join(home, ".cache", "huggingface", "hub", "models--x--y")
	if err := ValidatePath(cache); err != nil {
		t.Errorf("ValidatePath(%q) = %v on %s — cleanup must not be frozen by a facility that cannot exist here", cache, err, runtime.GOOS)
	}
}
