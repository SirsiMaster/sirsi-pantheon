package stacklab

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseNativePayloadPublicModeRecipe(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "contracts", "stacklab", "pantheon-release-artifact-recipe-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var recipe struct {
		Components []struct {
			ID      string   `json:"id"`
			Source  []string `json:"source"`
			Tests   []string `json:"tests"`
			Inputs  []string `json:"inputs"`
			Outputs []string `json:"outputs"`
			Upgrade []string `json:"upgrade_recipe"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &recipe); err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, component := range recipe.Components {
		if component.ID != "release-native-payload-composition" {
			continue
		}
		found++
		for _, path := range []string{"scripts/build-pkg.sh", "scripts/artifact_modes.py", "scripts/pantheon-pkg-components.plist", "scripts/package_info_contract.py", "internal/packageinventory/inventory.go"} {
			if !strings.Contains("\n"+strings.Join(component.Source, "\n")+"\n", "\n"+path+"\n") {
				t.Errorf("mode recipe missing source %s", path)
			}
		}
		for _, path := range []string{"scripts/test_artifact_modes.py", "scripts/test_package_info_contract.py", "cmd/sirsi-package-inventory/main_test.go"} {
			if !strings.Contains("\n"+strings.Join(component.Tests, "\n")+"\n", "\n"+path+"\n") {
				t.Errorf("mode recipe missing test %s", path)
			}
		}
		for _, requirement := range []struct{ text, token string }{
			{strings.Join(component.Inputs, " "), "umask077 or022"},
			{strings.Join(component.Outputs, " "), "0755"},
			{strings.Join(component.Outputs, " "), "0644"},
			{strings.Join(component.Upgrade, " "), "BundleOverwriteAction=upgrade"},
			{strings.Join(component.Upgrade, " "), "higher-version134 over a private133"},
			{strings.Join(component.Upgrade, " "), "fresh BOM/source tests are not proof"},
		} {
			if !strings.Contains(requirement.text, requirement.token) {
				t.Errorf("mode recipe missing scope %q", requirement.token)
			}
		}
	}
	if found != 1 {
		t.Fatalf("native composition component count=%d", found)
	}
}

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
