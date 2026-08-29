package sne

import (
	"strings"
	"testing"
)

func TestLegacySingleRuntimeUsesSignedPackageIdentity(t *testing.T) {
	entry := RuntimePackage{ModelID: "model", PackageID: "signed-package-v1", RuntimeSHA256: strings.Repeat("a", 64)}
	catalog := RuntimePackageCatalog{Entries: []RuntimePackage{entry}}
	if got := entry.EffectiveRuntimeID(); got != "signed-package-v1" {
		t.Fatalf("effective runtime ID=%q", got)
	}
	resolved, err := catalog.ResolveRuntime("model", "signed-package-v1")
	if err != nil || resolved.RuntimeID != "signed-package-v1" {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
}

func TestLegacyRootRuntimeUsesFullSignedRuntimeHash(t *testing.T) {
	hash := strings.Repeat("b", 64)
	entry := RuntimePackage{ModelID: "model", PackageRoot: "/legacy", RuntimeSHA256: hash}
	if got := entry.EffectiveRuntimeID(); got != "runtime-sha256-"+hash {
		t.Fatalf("effective runtime ID=%q", got)
	}
}
