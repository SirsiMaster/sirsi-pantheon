package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

func TestBuildEntryAcceptsV3ReceiptWithImmutableLineage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "api4096-package")
	paths := map[string][]byte{
		"bin/sned": []byte("service"), "lib/runtime/libsirsi_native_runtime.dylib": []byte("runtime"),
		"lib/libmlx.dylib": []byte("mlx"), "share/mlx.metallib": []byte("metal"),
		"lib/libjaccl.dylib": []byte("jaccl"),
	}
	for relative, data := range paths {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifest := map[string]any{"schema_version": "sne.model-manifest.v0", "model": map[string]any{"id": "model"}, "architecture": map[string]any{"cache_topology": "fixed-4096"}, "qualification": map[string]any{"serving_cache_capacity": 4096}}
	writeCatalogCandidateJSON(t, filepath.Join(root, "manifests/model.json"), manifest)
	identity := map[string]string{}
	preserved := map[string]string{}
	for key, relative := range map[string]string{"service_sha256": "bin/sned", "runtime_sha256": "lib/runtime/libsirsi_native_runtime.dylib", "mlx_sha256": "lib/libmlx.dylib", "metallib_sha256": "share/mlx.metallib", "jaccl_sha256": "lib/libjaccl.dylib", "manifest_sha256": "manifests/model.json"} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		value := hex.EncodeToString(digest[:])
		identity[key] = value
		preserved[relative] = value
	}
	writeCatalogCandidateJSON(t, filepath.Join(root, "manifests/PARENT.json"), map[string]any{"schema": "sirsi.sne.package-lineage.v1", "immutable_parent": "parent", "source_archive": "parent.zip", "source_archive_sha256": "a", "parent_sha256s_sha256": "b", "parent_lineage_sha256": "c", "preserved_payload_sha256": preserved, "change": "metadata", "execution_components_changed": false, "claim_scope": "runtime readiness"})
	seal := ""
	for key, relative := range map[string]string{"service_sha256": "bin/sned", "runtime_sha256": "lib/runtime/libsirsi_native_runtime.dylib", "mlx_sha256": "lib/libmlx.dylib", "metallib_sha256": "share/mlx.metallib", "jaccl_sha256": "lib/libjaccl.dylib", "manifest_sha256": "manifests/model.json"} {
		seal += identity[key] + "  ./" + relative + "\n"
	}
	if err := os.WriteFile(filepath.Join(root, "SHA256SUMS"), []byte(seal), 0o600); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(t.TempDir(), "receipt.json")
	gates := map[string]string{}
	for gate, value := range requiredGates {
		gates[gate] = value
	}
	evidence := map[string]any{}
	for _, name := range requiredV3Evidence {
		path := filepath.Join(t.TempDir(), name+".json")
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(path)
		digest := sha256.Sum256(data)
		evidence[name] = map[string]any{"path": path, "sha256": hex.EncodeToString(digest[:])}
	}
	sealData, _ := os.ReadFile(filepath.Join(root, "SHA256SUMS"))
	sealDigest := sha256.Sum256(sealData)
	identity["package_sha256"] = hex.EncodeToString(sealDigest[:])
	writeCatalogCandidateJSON(t, receiptPath, map[string]any{"schema": "sne.v2.api4096-parent-admission.v3", "status": "accepted", "created_at": "2026-08-22T02:39:55Z", "identity": identity, "execution": map[string]any{"cache_topology": "fixed-4096", "mode": "mtp", "serving_cache_capacity": 4096}, "gates": gates, "evidence": evidence, "claim_boundary": map[string]any{"performance_promoted": false, "runtime_readiness_only": true}})
	receiptData, _ := os.ReadFile(receiptPath)
	receiptDigest := sha256.Sum256(receiptData)
	pointerPath := filepath.Join(t.TempDir(), "pointer.json")
	writeCatalogCandidateJSON(t, pointerPath, map[string]any{"schema": "sne.active-launch-candidate.v1", "package_relative_path": filepath.Base(root), "package_sha256": hex.EncodeToString(sealDigest[:]), "admission_receipt_sha256": hex.EncodeToString(receiptDigest[:])})
	entry, err := buildEntry(root, receiptPath, pointerPath, "runtime-v3", func(sne.RuntimePackage) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if entry.RuntimeID != "runtime-v3" || entry.ServingCacheCapacity != 4096 {
		t.Fatalf("unexpected entry: %+v", entry)
	}
}

func writeCatalogCandidateJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
