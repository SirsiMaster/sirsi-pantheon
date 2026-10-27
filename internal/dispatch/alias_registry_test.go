package dispatch

import (
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// The shipped registry's aliases must pass the same validation the live router
// applies; a bad alias otherwise surfaces only after it is pinned on every host.
func TestShippedRegistryAliasesResolve(t *testing.T) {
	t.Setenv("SIRSI_ROUTER_URL", "")
	store, err := routerstore.OpenPath(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatal(err)
	}
	f := New(filepath.Join("..", "..", ".agents", "idea-router"), store)
	t.Cleanup(func() { _ = f.Close() })
	aliases, err := f.Aliases()
	if err != nil {
		t.Fatalf("shipped registry aliases invalid: %v", err)
	}
	for _, name := range []string{"rpa", "RPA"} {
		if got := aliases[name]; got != "codex-sirsinexusapp" {
			t.Fatalf("alias %q → %q, want codex-sirsinexusapp", name, got)
		}
	}
}
