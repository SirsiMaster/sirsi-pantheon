//go:build darwin

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/SirsiMaster/sirsi-pantheon/internal/updater"
)

func maintenanceReleaseFixture(data []byte) *updater.Release {
	digest := sha256.Sum256(data)
	r := &updater.Release{TagName: "v0.24.200"}
	for _, ext := range []string{"dmg", "pkg"} {
		name := "SirsiPantheon-0.24.200-arm64." + ext
		r.Assets = append(r.Assets, updater.Asset{Name: name, Size: int64(len(data)), Digest: "sha256:" + hex.EncodeToString(digest[:]),
			BrowserDownloadURL: "https://github.com/SirsiMaster/sirsi-pantheon/releases/download/" + r.TagName + "/" + name})
	}
	return r
}

func TestMaintenanceInstallerClosedAssetSelection(t *testing.T) {
	for _, name := range []string{"same-release", "missing-pkg", "old-release", "foreign-url", "missing-hash", "malformed-hash", "oversize"} {
		t.Run(name, func(t *testing.T) {
			r := maintenanceReleaseFixture([]byte("inert package, never executed"))
			switch name {
			case "missing-pkg":
				r.Assets = r.Assets[:1]
			case "old-release":
				r.TagName = "v0.24.136"
			case "foreign-url":
				r.Assets[1].BrowserDownloadURL = "https://untrusted.invalid/installer.pkg"
			case "missing-hash":
				r.Assets[1].Digest = ""
			case "malformed-hash":
				r.Assets[1].Digest = "sha256:" + strings.Repeat("A", 64)
			case "oversize":
				r.Assets[1].Size = 1<<30 + 1
			}
			asset, err := maintenancePackageAsset("0.24.200", r)
			if name == "same-release" {
				if err != nil || asset == nil || asset.Name != "SirsiPantheon-0.24.200-arm64.pkg" {
					t.Fatal(asset, err)
				}
			} else if err == nil {
				t.Fatal("untrusted/incompatible selection accepted", asset)
			}
		})
	}
}

func TestMaintenanceRollbackAssetRequiresExactOlderCompletePair(t *testing.T) {
	selected := maintenanceReleaseFixture([]byte("inert rollback package bytes"))
	selected.TagName = "v0.24.137"
	for i := range selected.Assets {
		selected.Assets[i].Name = strings.ReplaceAll(selected.Assets[i].Name, "0.24.200", "0.24.137")
		selected.Assets[i].BrowserDownloadURL = strings.ReplaceAll(selected.Assets[i].BrowserDownloadURL, "v0.24.200", "v0.24.137")
	}
	asset, err := maintenanceRollbackPackageAsset("0.24.138", selected)
	if err != nil || asset == nil || asset.Name != "SirsiPantheon-0.24.137-arm64.pkg" {
		t.Fatalf("exact complete older commercial pair refused: asset=%+v err=%v", asset, err)
	}
	if _, err := maintenanceRollbackPackageAsset("0.24.138", maintenanceReleaseFixture([]byte("inert package"))); err == nil {
		t.Fatal("newer package passed rollback selection")
	}
	selected.Assets = selected.Assets[:1]
	if _, err := maintenanceRollbackPackageAsset("0.24.138", selected); err == nil {
		t.Fatal("partial older package pair passed rollback selection")
	}
}

func TestMaintenanceInstallerStagesAndRechecksCapturedBytes(t *testing.T) {
	// Only the transport and trust probes are injected. The actual command,
	// private staging, checksum, retained-file and captured-selection checks run.
	data := []byte("inert test package; not an executable")
	release := maintenanceReleaseFixture(data)
	inspections := 0
	ops := maintenanceInstallerOps{
		release: func() (*updater.Release, error) { return release, nil },
		download: func(_ context.Context, _ string, path string) error {
			return os.WriteFile(path, data, 0600)
		},
		inspect: func(context.Context, string, string) (maintenancePackageInspection, error) {
			inspections++
			return maintenancePackageInspection{Capability: setup.LegacyAppCapability{Schema: 1, Native: true, Protocol: "root-relay-local-app-v1", Release: "0.24.200", Implementation: strings.Repeat("a", 64)},
				EngineSHA256: strings.Repeat("b", 64), InstallerIdentity: "Developer ID Installer: Sirsi Technologies Inc. (9D382WV988)", Notarization: "Notarized Developer ID",
				TargetBundleID: "ai.sirsi.pantheon", TargetAppPath: "/Applications/Pantheon.app", TargetHelperPath: "/usr/local/libexec/sirsi-pantheon/sirsi",
				Dependencies: []string{"/Applications/Pantheon.app", "/usr/local/libexec/sirsi-pantheon/sirsi"}}, nil
		},
	}
	plan, err := stageMaintenanceInstaller(context.Background(), "0.24.200", ops)
	if err != nil {
		t.Fatal(err)
	}
	// Cleanup only this fixture-created private staging directory.
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(plan.Package)) })
	if inspections != 1 || plan.Release != "0.24.200" || plan.Bytes != int64(len(data)) {
		t.Fatal(plan, inspections)
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "matching", true: "changed-before-open"}[changed], func(t *testing.T) {
			if changed {
				if err := os.WriteFile(plan.Package, []byte("substituted package"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before := inspections
			cmd := newMaintenanceInstallerCmd(ops)
			cmd.SilenceUsage = true
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"--verify-selection", hex.EncodeToString(raw)})
			err := cmd.Execute()
			if changed {
				if err == nil || !strings.Contains(err.Error(), "changed") || inspections != before || out.Len() != 0 {
					t.Fatal("changed selection reached trust/open boundary", err, inspections, out.String())
				}
			} else if err != nil || !bytes.Equal(out.Bytes(), raw) {
				t.Fatal("captured selection was not preserved", err, out.String())
			}
		})
	}
}

func TestMaintenanceInstallerTrustRefusalNeverProducesOpenSelection(t *testing.T) {
	data := []byte("inert")
	for _, failure := range []string{"checksum", "signature", "notary", "capability"} {
		t.Run(failure, func(t *testing.T) {
			var path string
			calls := 0
			ops := maintenanceInstallerOps{
				release: func() (*updater.Release, error) { return maintenanceReleaseFixture(data), nil },
				download: func(_ context.Context, _ string, dest string) error {
					path = dest
					if failure == "checksum" {
						return os.WriteFile(dest, []byte("wrong"), 0600)
					}
					return os.WriteFile(dest, data, 0600)
				},
				inspect: func(context.Context, string, string) (maintenancePackageInspection, error) {
					calls++
					return maintenancePackageInspection{}, errors.New("inert " + failure + " refused")
				},
			}
			plan, err := stageMaintenanceInstaller(context.Background(), "0.24.200", ops)
			if path != "" {
				t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(path)) })
			}
			if err == nil || plan.Schema != 0 || failure == "checksum" && calls != 0 || failure != "checksum" && calls != 1 {
				t.Fatal("unverified selection offered", plan, err, calls)
			}
		})
	}
}

func TestMaintenanceInstallerObservedPackageInfoContract(t *testing.T) {
	// Native pkgbuild marker shape, with inert version/payload sizes. Upgrade is
	// an element, not an attribute. No pkgbuild/Installer/tool is run here.
	const good = `<pkg-info overwrite-permissions="true" relocatable="false" identifier="ai.sirsi.pantheon" version="0.24.200" install-location="/" postinstall-action="none"><payload numberOfFiles="1" installKBytes="1"/><bundle path="./Applications/Pantheon.app" id="ai.sirsi.pantheon" CFBundleShortVersionString="0.24.200" CFBundleVersion="0.24.200"/><bundle-version><bundle id="ai.sirsi.pantheon"/></bundle-version><upgrade-bundle><bundle id="ai.sirsi.pantheon"/></upgrade-bundle><update-bundle/><atomic-update-bundle/><strict-identifier><bundle id="ai.sirsi.pantheon"/></strict-identifier><relocate/></pkg-info>`
	for _, name := range []string{"observed-shape", "missing-upgrade", "wrong-app", "empty-alias", "scripts", "entity"} {
		t.Run(name, func(t *testing.T) {
			body := good
			switch name {
			case "missing-upgrade":
				body = strings.Replace(body, "<upgrade-bundle><bundle id=\"ai.sirsi.pantheon\"/></upgrade-bundle>", "<upgrade-bundle/>", 1)
			case "wrong-app":
				body = strings.ReplaceAll(body, "Pantheon.app", "Other.app")
			case "empty-alias":
				body = strings.Replace(body, `<bundle id="ai.sirsi.pantheon"/>`, `<bundle wrong=""/>`, 1)
			case "scripts":
				body = strings.Replace(body, "<relocate/>", "<scripts/>", 1)
			case "entity":
				body = "<!DOCTYPE pkg-info>" + body
			}
			path := filepath.Join(t.TempDir(), "PackageInfo")
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			err := maintenancePackageInfo(path, "0.24.200")
			if (err == nil) != (name == "observed-shape") {
				t.Fatal(name, err)
			}
		})
	}
}
