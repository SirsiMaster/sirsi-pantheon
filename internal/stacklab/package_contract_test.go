package stacklab

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPackageBuildersUseOneCanonicalInventory keeps release packaging from
// degrading into ad-hoc shell checks. Both the assembled app and the expanded
// installer payload are checked through the same descriptor-rooted Go engine.
func TestPackageBuildersUseOneCanonicalInventory(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, script := range []string{"build-dmg.sh", "build-pkg.sh"} {
		bytes, err := os.ReadFile(filepath.Join(root, "scripts", script))
		if err != nil {
			t.Fatal(err)
		}
		for _, required := range []string{
			"package-inventory",
			"--require-code-signature",
			"cmd/sirsi-menubar/bundle/PkgInfo",
			"cmd/sirsi-menubar/bundle/ai.sirsi.pantheon.plist",
		} {
			if !strings.Contains(string(bytes), required) {
				t.Fatalf("%s does not bind canonical package identity through %q", script, required)
			}
		}
	}

	dmg, err := os.ReadFile(filepath.Join(root, "scripts", "build-dmg.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"cp -R \"${PROJECT_ROOT}/contracts/stacklab\"",
		"${BUILD_DIR}/sirsi\" package-inventory",
		"macapp/Package.swift",
		"swift build -c release",
	} {
		if !strings.Contains(string(dmg), required) {
			t.Fatalf("DMG builder does not retain the canonical payload route through %q", required)
		}
	}
	if strings.Contains(string(dmg), "go build -ldflags=\"${GO_LDFLAGS}\" -o \"${BUILD_DIR}/sirsi-menubar\"") {
		t.Fatal("DMG builder retains a Go menubar fallback instead of failing closed on the canonical Swift payload")
	}

	pkg, err := os.ReadFile(filepath.Join(root, "scripts", "build-pkg.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"pkgutil --expand-full",
		"${PROJECT_ROOT}/bin/sirsi\" package-inventory",
		"Payload/Applications/Pantheon.app",
		"COPYFILE_DISABLE=1 /usr/bin/ditto",
		"AppleDouble metadata",
	} {
		if !strings.Contains(string(pkg), required) {
			t.Fatalf("PKG builder does not verify its exact archive payload through %q", required)
		}
	}
	if strings.Contains(string(pkg), "/usr/bin/xattr -cr") {
		t.Fatal("PKG builder must not broadly erase protected macOS metadata; AppleDouble scans are the transport-metadata authority")
	}
}
