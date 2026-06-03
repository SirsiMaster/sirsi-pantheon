//go:build darwin

package platform

import "testing"

// TestCheckDiskAccess exercises the live probe and logs the resolved level — it
// asserts only invariants (the actual level depends on what the test binary has
// been granted), so it is environment-safe.
func TestCheckDiskAccess(t *testing.T) {
	da := CheckDiskAccess()
	t.Logf("disk access level=%s accessible=%v blocked=%v", da.Level, da.Accessible, da.Blocked)

	switch da.Level {
	case AccessNone, AccessSome, AccessFull:
		// ok — one of the three tiers
	default:
		t.Errorf("unexpected level %d", da.Level)
	}
	if da.Level == AccessFull && !HasFullDiskAccess() {
		t.Error("HasFullDiskAccess must be true when level is full")
	}
	if da.Level == AccessNone && len(da.Accessible) != 0 {
		t.Errorf("level none but %d accessible probes", len(da.Accessible))
	}
}
