package packageinventory

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// Both app builders copy this entire source directory. A hand-built bundle
// fixture alone cannot prove the real release payload fits the strict policy.
func TestSourceStackLabContractsMatchExactPayloadPolicy(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "stacklab")
	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		payload := "Contents/Resources/StackLab"
		if rel != "." {
			payload += "/" + filepath.ToSlash(rel)
		}
		want := "regular"
		if entry.IsDir() {
			want = "directory"
		} else if !entry.Type().IsRegular() {
			t.Errorf("source contract is not a regular file: %s", payload)
		}
		if allowed[payload] != want {
			t.Errorf("source contract payload %s has policy %q, want exact %q", payload, allowed[payload], want)
		}
		seen[payload] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for payload := range allowed {
		if (payload == "Contents/Resources/StackLab" || strings.HasPrefix(payload, "Contents/Resources/StackLab/")) && !seen[payload] {
			t.Errorf("payload policy has no corresponding source contract: %s", payload)
		}
	}
}
