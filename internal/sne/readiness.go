package sne

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

const modelReadinessSchema = "sne.gemma4-readiness.v1"

const (
	ReadinessIdentity    = "identity"
	ReadinessCorrectness = "correctness"
	ReadinessPerformance = "performance"
	ReadinessRelease     = "release"
)

type ModelReadinessRegistry struct {
	Schema       string               `json:"schema"`
	CatalogID    string               `json:"catalog_id"`
	AsOf         string               `json:"as_of"`
	GateMeanings map[string]string    `json:"gate_meanings"`
	Global       ModelReadinessGlobal `json:"global"`
	Entries      []ModelReadiness     `json:"entries"`
}

type ModelReadinessGlobal struct {
	CatalogEntries                  int  `json:"catalog_entries"`
	ExactTokenAndTerminalLogitPass  int  `json:"exact_token_and_terminal_logit_pass"`
	PantheonExactTupleAdmissionPass int  `json:"pantheon_exact_tuple_admission_pass"`
	BoundedQualityPass              int  `json:"bounded_quality_pass"`
	BoundedQualityFail              int  `json:"bounded_quality_fail"`
	SignedAndNotarized              int  `json:"signed_and_notarized"`
	CleanMacPass                    int  `json:"clean_mac_pass"`
	ExternalPilotPass               int  `json:"external_pilot_pass"`
	DynamicFrameworkFallback        bool `json:"dynamic_framework_fallback"`
	Fresh100ExactPass               int  `json:"fresh100_exact_pass"`
	LowSwapFresh100PerformancePass  int  `json:"low_swap_fresh100_performance_pass"`
}

type ModelReadiness struct {
	ID                  string `json:"id"`
	Exactness           string `json:"exactness"`
	Stability           string `json:"stability"`
	Quality             string `json:"quality"`
	Service             string `json:"service"`
	Package             string `json:"package"`
	SupervisedLifecycle string `json:"supervised_lifecycle"`
	Pantheon            string `json:"pantheon"`
	Nexus               string `json:"nexus"`
	Clean100            string `json:"clean100"`
	ExternalRelease     string `json:"external_release"`
	Disposition         string `json:"disposition"`
	RegistryAsOf        string `json:"-"`
}

func AdmitModelWithReadiness(registryPath, catalogEntry, manifestPath, readinessPath, policy string, allowResearch bool, profile SupervisorProfile) (ModelAdmission, ModelReadiness, error) {
	admission, err := AdmitModel(registryPath, catalogEntry, manifestPath, allowResearch, profile)
	if err != nil {
		return ModelAdmission{}, ModelReadiness{}, err
	}
	registry, err := LoadModelAdmissionRegistry(registryPath)
	if err != nil {
		return ModelAdmission{}, ModelReadiness{}, err
	}
	readiness, err := EvaluateModelReadiness(readinessPath, registry, catalogEntry, policy)
	if err != nil {
		return ModelAdmission{}, ModelReadiness{}, err
	}
	return admission, readiness, nil
}

func EvaluateModelReadiness(path string, admission ModelAdmissionRegistry, catalogEntry, policy string) (ModelReadiness, error) {
	policy = strings.TrimSpace(strings.ToLower(policy))
	if !validReadinessPolicy(policy) {
		return ModelReadiness{}, fmt.Errorf("unsupported SNE readiness policy %q", policy)
	}
	if strings.TrimSpace(path) == "" {
		return ModelReadiness{}, fmt.Errorf("SNE readiness evidence is required for policy %q", policy)
	}
	file, err := os.Open(path)
	if err != nil {
		return ModelReadiness{}, fmt.Errorf("open SNE readiness registry: %w", err)
	}
	defer file.Close()
	var registry ModelReadinessRegistry
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&registry); err != nil {
		return ModelReadiness{}, fmt.Errorf("decode SNE readiness registry: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return ModelReadiness{}, fmt.Errorf("decode SNE readiness registry: %w", err)
	}
	if registry.Schema != modelReadinessSchema || registry.CatalogID != admission.CatalogID || strings.TrimSpace(registry.AsOf) == "" {
		return ModelReadiness{}, fmt.Errorf("SNE readiness registry does not match admission catalog %q", admission.CatalogID)
	}
	for _, meaning := range []string{"pass", "open", "fail", "rejected"} {
		if strings.TrimSpace(registry.GateMeanings[meaning]) == "" {
			return ModelReadiness{}, fmt.Errorf("SNE readiness registry is missing gate meaning %q", meaning)
		}
	}
	if registry.Global.CatalogEntries != len(admission.Entries) {
		return ModelReadiness{}, fmt.Errorf("SNE readiness global catalog count %d does not match admission catalog count %d", registry.Global.CatalogEntries, len(admission.Entries))
	}
	admittedIDs := make(map[string]struct{}, len(admission.Entries))
	for _, entry := range admission.Entries {
		admittedIDs[entry.CatalogEntry] = struct{}{}
	}
	seen := make(map[string]struct{}, len(registry.Entries))
	var selected *ModelReadiness
	for index := range registry.Entries {
		entry := &registry.Entries[index]
		if _, exists := admittedIDs[entry.ID]; !exists {
			return ModelReadiness{}, fmt.Errorf("SNE readiness registry contains unknown catalog entry %q", entry.ID)
		}
		if _, duplicate := seen[entry.ID]; duplicate {
			return ModelReadiness{}, fmt.Errorf("SNE readiness registry contains duplicate catalog entry %q", entry.ID)
		}
		seen[entry.ID] = struct{}{}
		if entry.ID == catalogEntry {
			selected = entry
		}
	}
	if len(seen) != len(admittedIDs) {
		return ModelReadiness{}, fmt.Errorf("SNE readiness registry covers %d of %d admitted catalog entries", len(seen), len(admittedIDs))
	}
	if selected == nil {
		return ModelReadiness{}, fmt.Errorf("SNE readiness registry has no evidence for catalog entry %q", catalogEntry)
	}
	selected.RegistryAsOf = registry.AsOf
	if err := enforceReadinessPolicy(*selected, policy); err != nil {
		return ModelReadiness{}, fmt.Errorf("SNE catalog entry %q: %w", catalogEntry, err)
	}
	return *selected, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func validReadinessPolicy(policy string) bool {
	return policy == ReadinessIdentity || policy == ReadinessCorrectness || policy == ReadinessPerformance || policy == ReadinessRelease
}

func enforceReadinessPolicy(entry ModelReadiness, policy string) error {
	if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.Disposition) == "" {
		return fmt.Errorf("readiness evidence is incomplete")
	}
	if policy == ReadinessIdentity {
		return nil
	}
	if entry.Exactness != "pass" {
		return fmt.Errorf("correctness policy rejected exactness=%q", entry.Exactness)
	}
	if policy == ReadinessCorrectness {
		return nil
	}
	if !strings.HasPrefix(entry.Clean100, "pass-") {
		return fmt.Errorf("performance policy rejected clean100=%q", entry.Clean100)
	}
	disposition := strings.ToLower(entry.Disposition)
	if strings.Contains(disposition, "rejected") || strings.Contains(disposition, "rerun-required") {
		return fmt.Errorf("performance policy rejected disposition=%q", entry.Disposition)
	}
	if policy == ReadinessPerformance {
		return nil
	}
	if entry.ExternalRelease != "pass" {
		return fmt.Errorf("release policy rejected external_release=%q", entry.ExternalRelease)
	}
	return nil
}
