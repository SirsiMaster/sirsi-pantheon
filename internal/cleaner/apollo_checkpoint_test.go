package cleaner

import (
	"path/filepath"
	"testing"
)

func TestNativeApolloCheckpointProtectionIsNarrow(t *testing.T) {
	f := liveSNEFixture(t)
	args := []string{"/opt/apollo/bin/apollo-serve", "--bundle", f.coldRepo, "--checkpoint", f.servedSnap, "--addr", "127.0.0.1:8477"}
	stubJobs(t, map[string][]string{canonicalSNELabel: args})
	if got, ok := sneModelDir(args); !ok || got != f.servedSnap {
		t.Fatalf("native checkpoint not identified: %q %v", got, ok)
	}
	if err := ValidatePath(f.servedSnap); err == nil {
		t.Fatal("loaded native checkpoint offered for deletion")
	}
	for _, bad := range [][]string{
		{"/opt/other-tool", "--checkpoint", f.servedSnap},
		{"/opt/apollo-serve", "--bundle", f.servedSnap},
		{"/opt/apollo-serve", "--checkpoint"},
		{"/opt/apollo-serve", "--checkpoint", "relative"},
		{"/opt/apollo-serve", "--checkpoint", f.servedSnap, "--checkpoint", f.coldRepo},
		{"/opt/apollo-serve", "--checkpoint", filepath.Join(f.servedSnap, "missing")},
	} {
		if got, ok := sneModelDir(bad); ok {
			t.Fatalf("unrelated or ambiguous model enrolled: %v => %q", bad, got)
		}
	}
	if got, ok := sneModelDir([]string{"/opt/apollo-serve", "--checkpoint=" + f.servedSnap}); !ok || got != f.servedSnap {
		t.Fatal("native inline checkpoint not recognized")
	}
}
