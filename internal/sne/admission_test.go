package sne

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdmitModelBindsExactTuple(t *testing.T) {
	registryPath, manifestPath, entry := writeAdmissionFixture(t, "candidate")
	profile := SupervisorProfile{}
	profile.SNE.MemoryCeilingBytes = entry.MemoryBytes
	got, err := AdmitModel(registryPath, entry.CatalogEntry, manifestPath, false, profile)
	if err != nil {
		t.Fatal(err)
	}
	if got.ModelID != entry.ModelID {
		t.Fatalf("model ID = %q, want %q", got.ModelID, entry.ModelID)
	}
}

func TestAdmitModelRejectsResearchWithoutOptIn(t *testing.T) {
	registryPath, manifestPath, entry := writeAdmissionFixture(t, "research")
	if _, err := AdmitModel(registryPath, entry.CatalogEntry, manifestPath, false, SupervisorProfile{}); err == nil || !strings.Contains(err.Error(), "research opt-in") {
		t.Fatalf("research admission error = %v", err)
	}
	if _, err := AdmitModel(registryPath, entry.CatalogEntry, manifestPath, true, SupervisorProfile{}); err != nil {
		t.Fatal(err)
	}
}

func TestAdmitModelRejectsManifestDriftAndMemoryOverrun(t *testing.T) {
	registryPath, manifestPath, entry := writeAdmissionFixture(t, "candidate")
	if err := os.WriteFile(manifestPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := AdmitModel(registryPath, entry.CatalogEntry, manifestPath, false, SupervisorProfile{}); err == nil || !strings.Contains(err.Error(), "hash") {
		t.Fatalf("manifest drift error = %v", err)
	}
	registryPath, manifestPath, entry = writeAdmissionFixture(t, "candidate")
	profile := SupervisorProfile{}
	profile.SNE.MemoryCeilingBytes = entry.MemoryBytes - 1
	if _, err := AdmitModel(registryPath, entry.CatalogEntry, manifestPath, false, profile); err == nil || !strings.Contains(err.Error(), "above profile ceiling") {
		t.Fatalf("memory admission error = %v", err)
	}
}

func TestAdmitModelRejectsUnqualifiedDeviceFamily(t *testing.T) {
	registryPath, manifestPath, entry := writeAdmissionFixture(t, "candidate")
	previous := hostDeviceFamilyFn
	hostDeviceFamilyFn = func() (string, error) { return "Apple M1 Pro", nil }
	defer func() { hostDeviceFamilyFn = previous }()

	if _, err := AdmitModel(registryPath, entry.CatalogEntry, manifestPath, false, SupervisorProfile{}); err == nil || !strings.Contains(err.Error(), "not qualified for host device family") {
		t.Fatalf("device-family admission error = %v", err)
	}
}

func TestRegistryRejectsSemanticDuplicates(t *testing.T) {
	registryPath, _, _ := writeAdmissionFixture(t, "candidate")
	data, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	var registry ModelAdmissionRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatal(err)
	}
	duplicate := registry.Entries[0]
	duplicate.CatalogEntry = "duplicate-entry"
	registry.Entries = append(registry.Entries, duplicate)
	data, _ = json.Marshal(registry)
	if err := os.WriteFile(registryPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadModelAdmissionRegistry(registryPath); err == nil || !strings.Contains(err.Error(), "duplicate model ID") {
		t.Fatalf("duplicate registry error = %v", err)
	}
}

func writeAdmissionFixture(t *testing.T, qualification string) (string, string, ModelAdmission) {
	t.Helper()
	previous := hostDeviceFamilyFn
	hostDeviceFamilyFn = func() (string, error) { return "Test GPU", nil }
	t.Cleanup(func() { hostDeviceFamilyFn = previous })
	dir := t.TempDir()
	manifest := admissionManifest{SchemaVersion: "sne.model-manifest.v0"}
	manifest.Model.ID = "gemma-4-test"
	manifest.Model.Family = "gemma-4"
	manifest.Model.Architecture = "gemma4-dense"
	manifest.Model.ParameterClass = "12B"
	manifest.Artifacts.CheckpointSHA256 = strings.Repeat("b", 64)
	manifest.Artifacts.ArtifactSetSHA256 = strings.Repeat("c", 64)
	manifest.Artifacts.CompleteSnapshot = true
	manifest.Architecture.Adapter = "gemma4-dense-v0"
	manifest.Execution.Mode = "plain"
	manifest.Execution.Weight.Format = "affine"
	manifest.Execution.Weight.Bits = 8
	manifest.Execution.Weight.GroupSize = 64
	manifest.Requirements.MemoryBytes = 1024
	manifest.Requirements.DeviceFamilies = []string{"Test GPU"}
	manifest.Qualification.Status = qualification
	manifestData, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(dir, "model.json")
	if err := os.WriteFile(manifestPath, manifestData, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(manifestData)
	entry := ModelAdmission{
		CatalogEntry: "12b-affine8-plain", ManifestSHA256: hex.EncodeToString(digest[:]), ModelID: manifest.Model.ID,
		Architecture: manifest.Model.Architecture, ParameterClass: manifest.Model.ParameterClass, Adapter: manifest.Architecture.Adapter,
		ExecutionMode: manifest.Execution.Mode, WeightFormat: manifest.Execution.Weight.Format, WeightBits: 8, WeightGroupSize: 64,
		MemoryBytes: 1024, Qualification: qualification, CheckpointSHA256: manifest.Artifacts.CheckpointSHA256, ArtifactSetSHA256: manifest.Artifacts.ArtifactSetSHA256,
	}
	registry := ModelAdmissionRegistry{SchemaVersion: modelAdmissionSchema, CatalogID: "test", Family: "gemma-4", LifecyclePolicy: "supervised-restart", Entries: []ModelAdmission{entry}}
	registryData, _ := json.Marshal(registry)
	registryPath := filepath.Join(dir, "registry.json")
	if err := os.WriteFile(registryPath, registryData, 0o600); err != nil {
		t.Fatal(err)
	}
	return registryPath, manifestPath, entry
}
