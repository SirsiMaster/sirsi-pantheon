package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

func TestBuildEntryRequiresExactPromotionAdmissionAndAncestry(t *testing.T) {
	root := t.TempDir()
	packageRoot := filepath.Join(root, "api4096-package")
	for _, directory := range []string{"bin", "lib/runtime", "share", "manifests"} {
		mustMkdir(t, filepath.Join(packageRoot, directory))
	}
	files := map[string]string{"bin/sned": "service", "lib/runtime/libsirsi_native_runtime.dylib": "runtime", "lib/libmlx.dylib": "mlx", "lib/libjaccl.dylib": "jaccl", "share/mlx.metallib": "metal"}
	for path, value := range files {
		mustWrite(t, filepath.Join(packageRoot, path), value)
	}
	manifest := map[string]any{"model": map[string]any{"id": "gemma-api4096"}, "architecture": map[string]any{"cache_topology": "grouped-deferred"}, "qualification": map[string]any{"serving_cache_capacity": 4096}}
	mustJSON(t, filepath.Join(packageRoot, "manifests/model.json"), manifest)
	mustWrite(t, filepath.Join(packageRoot, "SHA256SUMS"), "sealed")
	receiptPath := filepath.Join(root, "receipt.json")
	identity := map[string]string{"service_sha256": hashPath(t, filepath.Join(packageRoot, "bin/sned")), "runtime_sha256": hashPath(t, filepath.Join(packageRoot, "lib/runtime/libsirsi_native_runtime.dylib")), "mlx_sha256": hashPath(t, filepath.Join(packageRoot, "lib/libmlx.dylib")), "metallib_sha256": hashPath(t, filepath.Join(packageRoot, "share/mlx.metallib")), "manifest_sha256": hashPath(t, filepath.Join(packageRoot, "manifests/model.json"))}
	gates := map[string]string{}
	for key, value := range requiredGates {
		gates[key] = value
	}
	mustJSON(t, receiptPath, map[string]any{"schema": "sne.v2.api4096-parent-admission.v2", "status": "accepted", "identity": identity, "gates": gates, "claim": map[string]any{"performance_promoted": false, "runtime_readiness_only": true}})
	receiptSHA := hashPath(t, receiptPath)
	mustJSON(t, filepath.Join(packageRoot, "manifests/PARENT.json"), map[string]any{"schema": "sne.api4096-product-parent.v1", "api4096_package_sha256": hashPath(t, filepath.Join(packageRoot, "SHA256SUMS")), "api4096_service_sha256": identity["service_sha256"], "api4096_runtime_sha256": identity["runtime_sha256"], "api4096_mlx_sha256": identity["mlx_sha256"], "api4096_metallib_sha256": identity["metallib_sha256"], "api4096_manifest_sha256": identity["manifest_sha256"], "api4096_admission_receipt_sha256": receiptSHA, "inherited_execution_artifacts_unchanged": true})
	pointerPath := filepath.Join(root, "pointer.json")
	mustJSON(t, pointerPath, map[string]any{"schema": "sne.active-launch-candidate.v1", "package_relative_path": "artifacts/candidates/api4096-package", "package_sha256": hashPath(t, filepath.Join(packageRoot, "SHA256SUMS")), "admission_receipt_sha256": receiptSHA})
	entry, err := buildEntry(packageRoot, receiptPath, pointerPath, "mtp-api4096-m5", func(value sne.RuntimePackage) error {
		if value.PackageRoot != packageRoot || value.NativeRuntimeSHA256 != identity["runtime_sha256"] {
			t.Fatalf("boundary input=%+v", value)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.ModelID != "gemma-api4096" || entry.RuntimeID != "mtp-api4096-m5" || entry.PackageID != "api4096-package" || entry.ServingCacheCapacity != 4096 {
		t.Fatalf("entry=%+v", entry)
	}

	gates["bounded_quality"] = "rejected"
	mustJSON(t, receiptPath, map[string]any{"schema": "sne.v2.api4096-parent-admission.v2", "status": "accepted", "identity": identity, "gates": gates, "claim": map[string]any{"performance_promoted": false, "runtime_readiness_only": true}})
	if _, err := buildEntry(packageRoot, receiptPath, pointerPath, "mtp-api4096-m5", func(sne.RuntimePackage) error { return nil }); err == nil || !strings.Contains(err.Error(), "bounded_quality") {
		t.Fatalf("bad quality gate accepted: %v", err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}
func mustWrite(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}
func mustJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, string(data))
}
func hashPath(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
