//go:build darwin && !cgo

package routerstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRelayObservationDarwinACLNoCGORefuses(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "owned-empty-summary"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if relayObservationNoACL(int(f.Fd())) || relayObservationNoACL(-1) {
		t.Fatal("no-cgo build must refuse unverifiable ACLs")
	}
}
