package routerstore

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"
)

// spoolOwnerTrusted is the pure ownership decision inside
// CheckSpoolDirTrustingGroup: same uid always passes (the CheckSpoolDir
// baseline); a different uid passes ONLY when a trust group is configured,
// the directory's gid matches it, AND the directory is actually
// group-writable. Table-driven because this is the one piece of real
// security logic in the whole feature — a UID belonging to no configured
// trust group must never slip through, and a trust group whose directory
// isn't group-writable must not silently grant access either.
func TestSpoolOwnerTrusted(t *testing.T) {
	const self = 501
	cases := []struct {
		name          string
		uid, gid      uint32
		trustGID      int
		groupWritable bool
		want          bool
	}{
		{"same uid always passes", self, 20, -1, false, true},
		{"same uid passes even with no group config", self, 999, -1, false, true},
		{"different uid, no trust group configured", 502, 20, -1, false, false},
		{"different uid, trust group configured, gid matches, group-writable", 502, 800, 800, true, true},
		{"different uid, gid matches but NOT group-writable — refused", 502, 800, 800, false, false},
		{"different uid, group-writable but gid does not match — refused", 502, 700, 800, true, false},
		{"different uid, trust group configured, unrelated gid", 502, 20, 800, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := spoolOwnerTrusted(c.uid, c.gid, self, c.trustGID, c.groupWritable)
			if got != c.want {
				t.Errorf("spoolOwnerTrusted(uid=%d,gid=%d,self=%d,trustGID=%d,groupWritable=%v) = %v, want %v",
					c.uid, c.gid, self, c.trustGID, c.groupWritable, got, c.want)
			}
		})
	}
}

// TestCheckSpoolDirTrustingGroupUnresolvableGroupFailsClosed: a typo'd or
// nonexistent trust group name must fail closed, never silently degrade to
// single-UID (CheckSpoolDir) behavior — a misconfiguration must not look like
// a stricter, working configuration.
func TestCheckSpoolDirTrustingGroupUnresolvableGroupFailsClosed(t *testing.T) {
	base := t.TempDir()
	if _, err := CheckSpoolDirTrustingGroup(filepath.Join(base, "spool"), "sirsi-nonexistent-group-xyz"); err == nil {
		t.Fatal("unresolvable trust group must be refused, not silently ignored")
	}
	if _, err := CheckSpoolDirTrustingGroup(filepath.Join(base, "spool"), ""); err == nil {
		t.Fatal("empty trust group must be refused by CheckSpoolDirTrustingGroup (call CheckSpoolDir directly instead)")
	}
}

// TestCheckSpoolDirTrustingGroupTightensToGroupWritable: when the spool
// directory's actual group is the trusted one, the auto-tighten target is
// 0770 (group keeps rwx), not 0700 — otherwise the relay would silently lock
// its lane clients out on every restart, which is the exact bug this feature
// exists to fix. This test can only exercise the SAME-uid ownership branch
// (there is no second real uid available without root/chown in this
// environment) — the different-uid acceptance path is covered by
// TestSpoolOwnerTrusted above and by the live daemon+relay round-trip
// verification recorded in this change's evidence, not by an automated test.
func TestCheckSpoolDirTrustingGroupTightensToGroupWritable(t *testing.T) {
	base := t.TempDir()
	spool := filepath.Join(base, "spool")
	if err := os.Mkdir(spool, 0o700); err != nil {
		t.Fatal(err)
	}
	selfGroup, err := currentPrimaryGroupName()
	if err != nil {
		t.Skipf("cannot resolve current primary group: %v", err)
	}
	if cherr := os.Chmod(spool, 0o770); cherr != nil {
		t.Fatal(cherr)
	}
	got, err := CheckSpoolDirTrustingGroup(spool, selfGroup)
	if err != nil {
		t.Fatalf("CheckSpoolDirTrustingGroup: %v", err)
	}
	st, statErr := os.Stat(got)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if st.Mode().Perm() != 0o770 {
		t.Fatalf("group-trusted spool mode = %o, want 0770 (group-writable preserved, not tightened away)", st.Mode().Perm())
	}
	// Negative control: the SAME directory through plain CheckSpoolDir (no
	// trust group) must still tighten to 0700 — proving the widened target is
	// specific to CheckSpoolDirTrustingGroup, not a change to the base function.
	if cherr := os.Chmod(spool, 0o770); cherr != nil {
		t.Fatal(cherr)
	}
	got2, err := CheckSpoolDir(spool)
	if err != nil {
		t.Fatalf("CheckSpoolDir: %v", err)
	}
	st2, statErr := os.Stat(got2)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if st2.Mode().Perm() != 0o700 {
		t.Fatalf("plain CheckSpoolDir must still tighten to 0700, got %o — the base function must be unaffected", st2.Mode().Perm())
	}
}

func currentPrimaryGroupName() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		return "", err
	}
	return g.Name, nil
}
