package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// `router token list` must show each token's lane scope: a host-wide token is the dangerous kind to have in a
// cloud sandbox, so it has to be visible at a glance and never read as scoped.
func TestRouterTokenListShowsScope(t *testing.T) {
	db := filepath.Join(t.TempDir(), "router.db")
	s, err := routerstore.OpenPath(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.MintHostToken("mac-host", "a Mac"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.MintScopedHostToken("cloud-host", "cloud env", []string{"lane-b", "lane-a"}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	old := routerTokenStore
	routerTokenStore = db
	defer func() { routerTokenStore = old }()
	var out bytes.Buffer
	routerTokenListCmd.SetOut(&out)
	if err := routerTokenListCmd.RunE(routerTokenListCmd, nil); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"mac-host", "scope=unscoped(host-wide)", "cloud-host", "scope=lane-a,lane-b"} {
		if !strings.Contains(got, want) {
			t.Fatalf("token list missing %q:\n%s", want, got)
		}
	}
}
