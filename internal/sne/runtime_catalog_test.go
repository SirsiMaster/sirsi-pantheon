package sne

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimePackageCatalogIsStrictAndResolvable(t *testing.T) {
	root := t.TempDir()
	catalog := RuntimePackageCatalog{SchemaVersion: RuntimePackageCatalogSchema, CatalogID: "test", Entries: []RuntimePackage{{
		ModelID: "gemma-test", PackageRoot: filepath.Join(root, "package"), RuntimeSHA256: strings.Repeat("a", 64),
		NativeRuntimeSHA256: strings.Repeat("b", 64), MLXDylibSHA256: strings.Repeat("c", 64),
		MetallibSHA256: strings.Repeat("d", 64), JACCLSHA256: strings.Repeat("e", 64),
	}}}
	data, _ := json.Marshal(catalog)
	path := filepath.Join(root, "runtime-packages.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRuntimePackageCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := loaded.Resolve("gemma-test")
	if err != nil || entry.PackageRoot != catalog.Entries[0].PackageRoot {
		t.Fatalf("resolved entry=%+v err=%v", entry, err)
	}
	data = append(data[:len(data)-1], []byte(`,"unknown":true}`)...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRuntimePackageCatalog(path); err == nil {
		t.Fatal("catalog accepted an unknown field")
	}
}

func TestRuntimePackageCatalogRequiresCompleteCapacityContract(t *testing.T) {
	root := t.TempDir()
	entry := RuntimePackage{
		ModelID: "gemma-test", PackageRoot: filepath.Join(root, "package"), RuntimeSHA256: strings.Repeat("a", 64),
		NativeRuntimeSHA256: strings.Repeat("b", 64), MLXDylibSHA256: strings.Repeat("c", 64),
		MetallibSHA256: strings.Repeat("d", 64), JACCLSHA256: strings.Repeat("e", 64),
		CacheTopology: "paged-ring-4096",
	}
	catalog := RuntimePackageCatalog{SchemaVersion: RuntimePackageCatalogSchema, CatalogID: "test", Entries: []RuntimePackage{entry}}
	data, _ := json.Marshal(catalog)
	path := filepath.Join(root, "runtime-packages.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRuntimePackageCatalog(path); err == nil {
		t.Fatal("catalog accepted cache topology without serving capacity")
	}
	entry.ServingCacheCapacity = 4096
	entry.PrefixSessionsMaximum = 2
	catalog.Entries[0] = entry
	data, _ = json.Marshal(catalog)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRuntimePackageCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Entries[0].ServingCacheCapacity != 4096 || loaded.Entries[0].PrefixSessionsMaximum != 2 {
		t.Fatalf("capacity contract=%+v", loaded.Entries[0])
	}
}

func TestRuntimePackageCatalogCarriesTruthfulStreamingMode(t *testing.T) {
	root := t.TempDir()
	entry := RuntimePackage{
		ModelID: "gemma-test", PackageRoot: filepath.Join(root, "package"), RuntimeSHA256: strings.Repeat("a", 64),
		NativeRuntimeSHA256: strings.Repeat("b", 64), MLXDylibSHA256: strings.Repeat("c", 64),
		MetallibSHA256: strings.Repeat("d", 64), JACCLSHA256: strings.Repeat("e", 64), StreamingMode: "buffered-compatibility-sse",
	}
	catalog := RuntimePackageCatalog{SchemaVersion: RuntimePackageCatalogSchema, CatalogID: "streaming", Entries: []RuntimePackage{entry}}
	path := filepath.Join(root, "runtime-packages.json")
	data, _ := json.Marshal(catalog)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRuntimePackageCatalog(path)
	if err != nil || loaded.Entries[0].StreamingMode != "buffered-compatibility-sse" {
		t.Fatalf("streaming contract=%+v err=%v", loaded.Entries, err)
	}
	catalog.Entries[0].StreamingMode = "pretend-live"
	data, _ = json.Marshal(catalog)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRuntimePackageCatalog(path); err == nil {
		t.Fatal("catalog accepted an unsupported streaming mode")
	}
}

func TestRuntimePackageCatalogRequiresExplicitVariantSelection(t *testing.T) {
	root := t.TempDir()
	entry := func(runtimeID, packageName, digest string) RuntimePackage {
		return RuntimePackage{
			ModelID: "gemma-test", RuntimeID: runtimeID, PackageRoot: filepath.Join(root, packageName),
			RuntimeSHA256: strings.Repeat(digest, 64), NativeRuntimeSHA256: strings.Repeat("b", 64),
			MLXDylibSHA256: strings.Repeat("c", 64), MetallibSHA256: strings.Repeat("d", 64),
			JACCLSHA256: strings.Repeat("e", 64),
		}
	}
	catalog := RuntimePackageCatalog{
		SchemaVersion: RuntimePackageCatalogSchema, CatalogID: "variants",
		Entries: []RuntimePackage{entry("stable", "stable", "a"), entry("candidate-v26", "candidate", "f")},
	}
	data, _ := json.Marshal(catalog)
	path := filepath.Join(root, "runtime-packages.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRuntimePackageCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loaded.Resolve("gemma-test"); err == nil || !strings.Contains(err.Error(), "runtime_id is required") {
		t.Fatalf("implicit ambiguous selection did not fail closed: %v", err)
	}
	selected, err := loaded.ResolveRuntime("gemma-test", "candidate-v26")
	if err != nil || selected.RuntimeID != "candidate-v26" {
		t.Fatalf("explicit runtime selection=%+v err=%v", selected, err)
	}

	catalog.Entries[1].RuntimeID = ""
	data, _ = json.Marshal(catalog)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRuntimePackageCatalog(path); err == nil || !strings.Contains(err.Error(), "lacks runtime_id") {
		t.Fatalf("mixed legacy/explicit variants were admitted: %v", err)
	}
}

func TestPortableRuntimeCatalogMaterializesOnlyBeneathPackageRoot(t *testing.T) {
	root := t.TempDir()
	entry := RuntimePackage{
		ModelID: "gemma-test", RuntimeID: "stable", PackageID: "gemma-test-stable-v1",
		RuntimeSHA256: strings.Repeat("a", 64), NativeRuntimeSHA256: strings.Repeat("b", 64),
		MLXDylibSHA256: strings.Repeat("c", 64), MetallibSHA256: strings.Repeat("d", 64),
		JACCLSHA256: strings.Repeat("e", 64),
	}
	catalog := RuntimePackageCatalog{SchemaVersion: RuntimePackageCatalogSchema, CatalogID: "portable", Entries: []RuntimePackage{entry}}
	data, _ := json.Marshal(catalog)
	path := filepath.Join(root, "catalog.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRuntimePackageCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	packagesRoot := filepath.Join(root, "installed-packages")
	materialized, err := loaded.MaterializePackageRoots(packagesRoot)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(packagesRoot, entry.PackageID)
	if materialized.Entries[0].PackageRoot != want || materialized.Entries[0].PackageID != entry.PackageID {
		t.Fatalf("materialized entry=%+v want_root=%s", materialized.Entries[0], want)
	}

	for _, unsafe := range []string{"../escape", "nested/package", ".", "..", "package id"} {
		catalog.Entries[0].PackageID = unsafe
		data, _ = json.Marshal(catalog)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadRuntimePackageCatalog(path); err == nil {
			t.Fatalf("unsafe package_id %q was admitted", unsafe)
		}
	}

	catalog.Entries[0] = entry
	catalog.Entries[0].PackageRoot = filepath.Join(root, "also-set")
	data, _ = json.Marshal(catalog)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRuntimePackageCatalog(path); err == nil {
		t.Fatal("catalog admitted both package_id and package_root")
	}
}

func TestRuntimeCatalogPortableRoundTripPreservesPackageSelection(t *testing.T) {
	root := t.TempDir()
	packagesRoot := filepath.Join(root, "packages")
	packageID := "gemma-test-stable-v1"
	original := RuntimePackageCatalog{
		SchemaVersion: RuntimePackageCatalogSchema, CatalogID: "round-trip",
		Entries: []RuntimePackage{{
			ModelID: "gemma-test", RuntimeID: "stable", PackageRoot: filepath.Join(packagesRoot, packageID),
			RuntimeSHA256: strings.Repeat("a", 64), NativeRuntimeSHA256: strings.Repeat("b", 64),
			MLXDylibSHA256: strings.Repeat("c", 64), MetallibSHA256: strings.Repeat("d", 64),
			JACCLSHA256: strings.Repeat("e", 64),
		}},
	}
	portable, err := original.PortablePackageCatalog(packagesRoot)
	if err != nil {
		t.Fatal(err)
	}
	if portable.Entries[0].PackageID != packageID || portable.Entries[0].PackageRoot != "" {
		t.Fatalf("portable entry=%+v", portable.Entries[0])
	}
	materialized, err := portable.MaterializePackageRoots(packagesRoot)
	if err != nil {
		t.Fatal(err)
	}
	if materialized.Entries[0].PackageRoot != original.Entries[0].PackageRoot {
		t.Fatalf("round trip root=%q want=%q", materialized.Entries[0].PackageRoot, original.Entries[0].PackageRoot)
	}

	original.Entries[0].PackageRoot = filepath.Join(packagesRoot, "nested", packageID)
	if _, err := original.PortablePackageCatalog(packagesRoot); err == nil {
		t.Fatal("nested package root was made portable")
	}
}

func TestRuntimePackageBoundaryAllowsContainedFileSymlinkAndRejectsEscape(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "contained")); err != nil {
		t.Fatal(err)
	}
	if err := verifyPackageTreeSymlinkBoundary(root); err != nil {
		t.Fatalf("contained file symlink rejected: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escaped")); err != nil {
		t.Fatal(err)
	}
	if err := verifyPackageTreeSymlinkBoundary(root); err == nil {
		t.Fatal("runtime package accepted an escaping symlink")
	}
}

func TestMachOLoadPathRejectsBuildTreeAndAllowsPackageBindings(t *testing.T) {
	root := t.TempDir()
	for _, relative := range []string{"bin/sned", "lib/libmlx.dylib", "lib/libjaccl.dylib", "lib/runtime/libsirsi_native_runtime.dylib"} {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	binary := filepath.Join(root, "bin", "sned")
	for _, allowed := range []string{
		"/System/Library/Frameworks/Metal.framework/Versions/A/Metal",
		"/usr/lib/libSystem.B.dylib",
		"@rpath/libsirsi_native_runtime.dylib",
		"@executable_path/../lib/runtime/libsirsi_native_runtime.dylib",
	} {
		if err := validateMachOLoadPath(root, binary, allowed, false); err != nil {
			t.Fatalf("allowed path %q rejected: %v", allowed, err)
		}
	}
	for _, allowedRPath := range []string{"@loader_path", "@loader_path/..", "@executable_path/../lib/runtime"} {
		if err := validateMachOLoadPath(root, filepath.Join(root, "lib", "libmlx.dylib"), allowedRPath, true); err != nil {
			t.Fatalf("allowed rpath %q rejected: %v", allowedRPath, err)
		}
	}
	for _, rejected := range []string{
		"/private/tmp/build/libmlx.dylib",
		"/Users/developer/build/libmlx.dylib",
		"@rpath/untracked.dylib",
		"@loader_path/../../../outside.dylib",
	} {
		if err := validateMachOLoadPath(root, binary, rejected, false); err == nil {
			t.Fatalf("unsafe path %q accepted", rejected)
		}
	}
}

func TestRealRuntimePackageBoundary(t *testing.T) {
	root := os.Getenv("SNE_REAL_PACKAGE_BOUNDARY")
	if root == "" {
		t.Skip("real SNE package boundary input not supplied")
	}
	hash := func(relative string) string {
		digest, err := runtimeSHA256File(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		return digest
	}
	entry := RuntimePackage{
		ModelID: "real-package-boundary", PackageRoot: root,
		RuntimeSHA256: hash("bin/sned"), NativeRuntimeSHA256: hash("lib/runtime/libsirsi_native_runtime.dylib"),
		MLXDylibSHA256: hash("lib/libmlx.dylib"), MetallibSHA256: hash("share/mlx.metallib"),
		JACCLSHA256: hash("lib/libjaccl.dylib"),
	}
	if err := VerifyRuntimePackageBoundary(entry); err != nil {
		t.Fatal(err)
	}
	t.Logf("pantheon_sne_package_boundary accepted=true root=%s runtime_sha256=%s", root, entry.RuntimeSHA256)
}
