package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/packageinventory"
	"github.com/SirsiMaster/sirsi-pantheon/internal/packageinventorycmd"
)

func TestResolveApolloBundleUsesExplicitCapabilityAndProfilePair(t *testing.T) {
	for _, tc := range []struct {
		name       string
		capability string
		profile    bool
		wantErr    string
	}{
		{"disabled absent", packageinventory.ApolloCapabilityDisabled, false, ""},
		{"disabled profile conflict", packageinventory.ApolloCapabilityDisabled, true, "conflicts"},
		{"enabled exact profile", packageinventory.ApolloCapabilityEnabled, true, ""},
		{"enabled missing profile", packageinventory.ApolloCapabilityEnabled, false, "unavailable"},
		{"unknown capability", "automatic", false, "malformed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bundle, executable, profile := makeApolloBundleFixture(t, tc.capability, tc.profile)
			resolved, err := resolveApolloBundle(executable, filepath.Join(t.TempDir(), "managed", "sirsi"), filepath.Join(t.TempDir(), "Pantheon.app"), packageinventorycmd.ReadCanonicalFile, apolloNativeFileSHA256, os.Lstat)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("invalid bundle state accepted or unrelated refusal: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("valid bundle rejected: %v", err)
			}
			if resolved.InfoPlist != filepath.Join(bundle, "Contents", "Info.plist") {
				t.Fatalf("resolved Info.plist=%q", resolved.InfoPlist)
			}
			if tc.profile && (resolved.Profile != profile || string(resolved.ProfileBytes) != `{"fixture":true}`) {
				t.Fatalf("resolved profile differs from the app resource: %+v", resolved)
			}
		})
	}
}

func TestResolveApolloBundleRejectsUnknownAndMalformedAssociations(t *testing.T) {
	managedExecutable := filepath.Join(t.TempDir(), "managed", "sirsi")
	managedApp := filepath.Join(t.TempDir(), "Applications", "Pantheon.app")
	t.Run("unknown executable layout", func(t *testing.T) {
		loose := filepath.Join(t.TempDir(), "loose", "sirsi")
		if _, err := resolveApolloBundle(loose, managedExecutable, managedApp, packageinventorycmd.ReadCanonicalFile, apolloNativeFileSHA256, os.Lstat); err == nil || !strings.Contains(err.Error(), "layout unavailable") {
			t.Fatalf("unknown executable layout accepted: %v", err)
		}
	})
	t.Run("missing Info.plist is unavailable", func(t *testing.T) {
		bundle, executable, _ := makeApolloBundleFixture(t, packageinventory.ApolloCapabilityDisabled, false)
		if err := os.Remove(filepath.Join(bundle, "Contents", "Info.plist")); err != nil {
			t.Fatal(err)
		}
		if _, err := resolveApolloBundle(executable, managedExecutable, managedApp, packageinventorycmd.ReadCanonicalFile, apolloNativeFileSHA256, os.Lstat); err == nil || !strings.Contains(err.Error(), "Info.plist unavailable") {
			t.Fatalf("missing Info.plist accepted as disabled: %v", err)
		}
	})
	t.Run("enabled metadata without profile is unavailable", func(t *testing.T) {
		_, executable, _ := makeApolloBundleFixture(t, packageinventory.ApolloCapabilityEnabled, false)
		if _, err := resolveApolloBundle(executable, managedExecutable, managedApp, packageinventorycmd.ReadCanonicalFile, apolloNativeFileSHA256, os.Lstat); err == nil || !strings.Contains(err.Error(), "profile unavailable") {
			t.Fatalf("enabled metadata without profile accepted: %v", err)
		}
	})
	t.Run("enabled metadata with symlinked profile is unsafe", func(t *testing.T) {
		_, executable, profile := makeApolloBundleFixture(t, packageinventory.ApolloCapabilityEnabled, true)
		target := filepath.Join(t.TempDir(), "profile.json")
		if err := os.Rename(profile, target); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, profile); err != nil {
			t.Fatal(err)
		}
		if _, err := resolveApolloBundle(executable, managedExecutable, managedApp, packageinventorycmd.ReadCanonicalFile, apolloNativeFileSHA256, os.Lstat); err == nil || !strings.Contains(err.Error(), "unsafe") {
			t.Fatalf("symlinked installed profile accepted: %v", err)
		}
	})
	t.Run("malformed Info.plist remains unavailable", func(t *testing.T) {
		bundle, executable, _ := makeApolloBundleFixture(t, packageinventory.ApolloCapabilityDisabled, false)
		if err := os.WriteFile(filepath.Join(bundle, "Contents", "Info.plist"), []byte("not a plist"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := resolveApolloBundle(executable, managedExecutable, managedApp, packageinventorycmd.ReadCanonicalFile, apolloNativeFileSHA256, os.Lstat); err == nil || !strings.Contains(err.Error(), "Info.plist") {
			t.Fatalf("malformed capability metadata accepted: %v", err)
		}
	})
}

func TestResolveManagedApolloCLIRequiresAppEngineIdentity(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	managedExecutable := filepath.Join(root, "usr", "local", "libexec", "sirsi-pantheon", "sirsi")
	managedApp := filepath.Join(root, "Applications", "Pantheon.app")
	if err := os.MkdirAll(filepath.Dir(managedExecutable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(managedApp, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(managedApp, "Contents", "Resources"), 0o755); err != nil {
		t.Fatal(err)
	}
	appEngine := filepath.Join(managedApp, "Contents", "MacOS", "sirsi")
	if err := os.WriteFile(managedExecutable, []byte("same engine bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(appEngine, []byte("same engine bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	info := filepath.Join(managedApp, "Contents", "Info.plist")
	if err := os.WriteFile(info, nativeCapabilityInfo("disabled"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveApolloBundle(managedExecutable, managedExecutable, managedApp, packageinventorycmd.ReadCanonicalFile, apolloNativeFileSHA256, os.Lstat)
	if err != nil || resolved.InfoPlist != info || resolved.Capability != packageinventory.ApolloCapabilityDisabled {
		t.Fatalf("byte-identical managed CLI did not resolve its app metadata: bundle=%+v err=%v", resolved, err)
	}
	if err := os.WriteFile(appEngine, []byte("different engine bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveApolloBundle(managedExecutable, managedExecutable, managedApp, packageinventorycmd.ReadCanonicalFile, apolloNativeFileSHA256, os.Lstat); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("managed CLI accepted an unrelated app bundle: %v", err)
	}
}

func makeApolloBundleFixture(t *testing.T, capability string, withProfile bool) (string, string, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(root, "Applications", "Pantheon.app")
	contents := filepath.Join(bundle, "Contents")
	if err := os.MkdirAll(filepath.Join(contents, "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(contents, "Resources", "Apollo"), 0o755); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(contents, "MacOS", "sirsi")
	if err := os.WriteFile(executable, []byte("fixture engine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), nativeCapabilityInfo(capability), 0o644); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(contents, "Resources", "Apollo", "native-profile.json")
	if withProfile {
		if err := os.WriteFile(profile, []byte(`{"fixture":true}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return bundle, executable, profile
}

func nativeCapabilityInfo(capability string) []byte {
	return []byte(`<?xml version="1.0"?><plist><dict><key>CFBundleIdentifier</key><string>ai.sirsi.pantheon</string><key>SirsiApolloNativeCapability</key><string>` + capability + `</string></dict></plist>`)
}
