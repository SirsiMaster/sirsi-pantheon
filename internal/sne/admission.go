package sne

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const modelAdmissionSchema = "pantheon.sne-model-admission.v1"

type ModelAdmissionRegistry struct {
	SchemaVersion   string           `json:"schema_version"`
	CatalogID       string           `json:"catalog_id"`
	Family          string           `json:"family"`
	LifecyclePolicy string           `json:"lifecycle_policy"`
	Entries         []ModelAdmission `json:"entries"`
}

type ModelAdmission struct {
	CatalogEntry      string `json:"catalog_entry"`
	ManifestSHA256    string `json:"manifest_sha256"`
	ModelID           string `json:"model_id"`
	Architecture      string `json:"architecture"`
	ParameterClass    string `json:"parameter_class"`
	Adapter           string `json:"adapter"`
	ExecutionMode     string `json:"execution_mode"`
	WeightFormat      string `json:"weight_format"`
	WeightBits        int    `json:"weight_bits"`
	WeightGroupSize   int    `json:"weight_group_size"`
	MemoryBytes       uint64 `json:"memory_bytes"`
	Qualification     string `json:"qualification"`
	CheckpointSHA256  string `json:"checkpoint_sha256"`
	ArtifactSetSHA256 string `json:"artifact_set_sha256"`
}

type admissionManifest struct {
	SchemaVersion string `json:"schema_version"`
	Model         struct {
		ID             string `json:"id"`
		Family         string `json:"family"`
		Architecture   string `json:"architecture"`
		ParameterClass string `json:"parameter_class"`
	} `json:"model"`
	Artifacts struct {
		CheckpointSHA256  string `json:"checkpoint_sha256"`
		ArtifactSetSHA256 string `json:"artifact_set_sha256"`
		CompleteSnapshot  bool   `json:"complete_snapshot"`
	} `json:"artifacts"`
	Architecture struct {
		Adapter string `json:"adapter"`
	} `json:"architecture"`
	Execution struct {
		Mode   string `json:"mode"`
		Weight struct {
			Format    string `json:"format"`
			Bits      int    `json:"bits"`
			GroupSize int    `json:"group_size"`
		} `json:"weight"`
	} `json:"execution"`
	Requirements struct {
		MemoryBytes    uint64   `json:"memory_bytes"`
		DeviceFamilies []string `json:"device_families"`
	} `json:"requirements"`
	Qualification struct {
		Status string `json:"status"`
	} `json:"qualification"`
}

func LoadModelAdmissionRegistry(path string) (ModelAdmissionRegistry, error) {
	file, err := os.Open(path)
	if err != nil {
		return ModelAdmissionRegistry{}, fmt.Errorf("open SNE model admission registry: %w", err)
	}
	defer file.Close()
	var registry ModelAdmissionRegistry
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&registry); err != nil {
		return ModelAdmissionRegistry{}, fmt.Errorf("decode SNE model admission registry: %w", err)
	}
	if registry.SchemaVersion != modelAdmissionSchema || registry.CatalogID == "" || registry.Family != "gemma-4" || registry.LifecyclePolicy != "supervised-restart" || len(registry.Entries) == 0 {
		return ModelAdmissionRegistry{}, fmt.Errorf("unsupported SNE model admission registry")
	}
	entryIDs := make(map[string]struct{}, len(registry.Entries))
	modelIDs := make(map[string]struct{}, len(registry.Entries))
	manifestHashes := make(map[string]struct{}, len(registry.Entries))
	tuples := make(map[string]struct{}, len(registry.Entries))
	for _, entry := range registry.Entries {
		if err := validateAdmissionEntry(entry); err != nil {
			return ModelAdmissionRegistry{}, fmt.Errorf("catalog entry %q: %w", entry.CatalogEntry, err)
		}
		if _, exists := entryIDs[entry.CatalogEntry]; exists {
			return ModelAdmissionRegistry{}, fmt.Errorf("duplicate catalog entry %q", entry.CatalogEntry)
		}
		if _, exists := modelIDs[entry.ModelID]; exists {
			return ModelAdmissionRegistry{}, fmt.Errorf("duplicate model ID %q", entry.ModelID)
		}
		if _, exists := manifestHashes[entry.ManifestSHA256]; exists {
			return ModelAdmissionRegistry{}, fmt.Errorf("duplicate manifest SHA-256 %q", entry.ManifestSHA256)
		}
		tuple := fmt.Sprintf("%s|%s|%s|%s|%s|%d|%d", entry.Architecture, entry.ParameterClass, entry.Adapter, entry.ExecutionMode, entry.WeightFormat, entry.WeightBits, entry.WeightGroupSize)
		if _, exists := tuples[tuple]; exists {
			return ModelAdmissionRegistry{}, fmt.Errorf("duplicate execution tuple %q", tuple)
		}
		entryIDs[entry.CatalogEntry] = struct{}{}
		modelIDs[entry.ModelID] = struct{}{}
		manifestHashes[entry.ManifestSHA256] = struct{}{}
		tuples[tuple] = struct{}{}
	}
	return registry, nil
}

func AdmitModel(registryPath, catalogEntry, manifestPath string, allowResearch bool, profile SupervisorProfile) (ModelAdmission, error) {
	registry, err := LoadModelAdmissionRegistry(registryPath)
	if err != nil {
		return ModelAdmission{}, err
	}
	var admitted *ModelAdmission
	for index := range registry.Entries {
		if registry.Entries[index].CatalogEntry == catalogEntry {
			admitted = &registry.Entries[index]
			break
		}
	}
	if admitted == nil {
		return ModelAdmission{}, fmt.Errorf("SNE catalog entry %q is not admitted", catalogEntry)
	}
	if admitted.Qualification == "research" && !allowResearch {
		return ModelAdmission{}, fmt.Errorf("SNE catalog entry %q requires explicit research opt-in", catalogEntry)
	}
	if profile.SNE.MemoryCeilingBytes > 0 && admitted.MemoryBytes > profile.SNE.MemoryCeilingBytes {
		return ModelAdmission{}, fmt.Errorf("SNE catalog entry %q requires %d bytes above profile ceiling %d", catalogEntry, admitted.MemoryBytes, profile.SNE.MemoryCeilingBytes)
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return ModelAdmission{}, fmt.Errorf("read admitted SNE model manifest: %w", err)
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != admitted.ManifestSHA256 {
		return ModelAdmission{}, fmt.Errorf("SNE model manifest hash does not match catalog entry %q", catalogEntry)
	}
	var manifest admissionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ModelAdmission{}, fmt.Errorf("decode admitted SNE model manifest: %w", err)
	}
	if manifest.SchemaVersion != "sne.model-manifest.v0" || manifest.Model.Family != registry.Family || !manifest.Artifacts.CompleteSnapshot {
		return ModelAdmission{}, fmt.Errorf("SNE model manifest is not a complete %s v0 snapshot", registry.Family)
	}
	if manifest.Model.ID != admitted.ModelID || manifest.Model.Architecture != admitted.Architecture || manifest.Model.ParameterClass != admitted.ParameterClass || manifest.Architecture.Adapter != admitted.Adapter || manifest.Execution.Mode != admitted.ExecutionMode || manifest.Execution.Weight.Format != admitted.WeightFormat || manifest.Execution.Weight.Bits != admitted.WeightBits || manifest.Execution.Weight.GroupSize != admitted.WeightGroupSize || manifest.Requirements.MemoryBytes != admitted.MemoryBytes || manifest.Qualification.Status != admitted.Qualification || manifest.Artifacts.CheckpointSHA256 != admitted.CheckpointSHA256 || manifest.Artifacts.ArtifactSetSHA256 != admitted.ArtifactSetSHA256 {
		return ModelAdmission{}, fmt.Errorf("SNE model manifest tuple does not match catalog entry %q", catalogEntry)
	}
	if len(manifest.Requirements.DeviceFamilies) == 0 {
		return ModelAdmission{}, fmt.Errorf("SNE model manifest declares no qualified device families")
	}
	hostFamily, err := hostDeviceFamilyFn()
	if err != nil {
		return ModelAdmission{}, fmt.Errorf("determine host device family: %w", err)
	}
	hostFamily = strings.TrimSpace(hostFamily)
	if hostFamily == "" {
		return ModelAdmission{}, fmt.Errorf("determine host device family: empty hardware identity")
	}
	qualified := false
	for _, family := range manifest.Requirements.DeviceFamilies {
		if strings.TrimSpace(family) == hostFamily {
			qualified = true
			break
		}
	}
	if !qualified {
		return ModelAdmission{}, fmt.Errorf("SNE catalog entry %q is not qualified for host device family %q", catalogEntry, hostFamily)
	}
	return *admitted, nil
}

var hostDeviceFamilyFn = detectHostDeviceFamily

func detectHostDeviceFamily() (string, error) {
	output, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
	if err != nil {
		return "", err
	}
	family := strings.TrimSpace(string(output))
	if family == "" {
		return "", fmt.Errorf("empty machdep.cpu.brand_string")
	}
	return family, nil
}

func validateAdmissionEntry(entry ModelAdmission) error {
	if strings.TrimSpace(entry.CatalogEntry) == "" || strings.TrimSpace(entry.ModelID) == "" || strings.TrimSpace(entry.Architecture) == "" || strings.TrimSpace(entry.ParameterClass) == "" || strings.TrimSpace(entry.Adapter) == "" || strings.TrimSpace(entry.ExecutionMode) == "" || strings.TrimSpace(entry.WeightFormat) == "" || entry.WeightBits <= 0 || entry.WeightGroupSize <= 0 || entry.MemoryBytes == 0 {
		return fmt.Errorf("incomplete execution tuple")
	}
	if entry.Qualification != "candidate" && entry.Qualification != "admitted" && entry.Qualification != "research" {
		return fmt.Errorf("unsupported qualification %q", entry.Qualification)
	}
	for label, value := range map[string]string{"manifest": entry.ManifestSHA256, "checkpoint": entry.CheckpointSHA256, "artifact set": entry.ArtifactSetSHA256} {
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("invalid %s SHA-256", label)
		}
	}
	return nil
}
