package sne

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvaluateModelReadinessPolicies(t *testing.T) {
	admission := readinessAdmissionFixture()
	entry := passingReadinessFixture()
	path := writeReadinessFixture(t, admission.CatalogID, []ModelReadiness{entry})
	for _, policy := range []string{ReadinessIdentity, ReadinessCorrectness, ReadinessPerformance} {
		if _, err := EvaluateModelReadiness(path, admission, entry.ID, policy); err != nil {
			t.Fatalf("policy %s: %v", policy, err)
		}
	}
	if _, err := EvaluateModelReadiness(path, admission, entry.ID, ReadinessRelease); err == nil || !strings.Contains(err.Error(), "external_release") {
		t.Fatalf("release error = %v", err)
	}
}

func TestEvaluateModelReadinessRejectsPerformanceFailures(t *testing.T) {
	admission := readinessAdmissionFixture()
	entry := passingReadinessFixture()
	entry.Clean100 = "fail-swap-growth-post-restart-rerun-required"
	entry.Disposition = "candidate-correctness-pass-performance-rerun-required"
	path := writeReadinessFixture(t, admission.CatalogID, []ModelReadiness{entry})
	if _, err := EvaluateModelReadiness(path, admission, entry.ID, ReadinessCorrectness); err != nil {
		t.Fatalf("correctness policy should pass: %v", err)
	}
	if _, err := EvaluateModelReadiness(path, admission, entry.ID, ReadinessPerformance); err == nil || !strings.Contains(err.Error(), "clean100") {
		t.Fatalf("performance error = %v", err)
	}
}

func TestEvaluateModelReadinessRejectsCatalogDrift(t *testing.T) {
	admission := readinessAdmissionFixture()
	entry := passingReadinessFixture()
	if _, err := EvaluateModelReadiness(writeReadinessFixture(t, "wrong-catalog", []ModelReadiness{entry}), admission, entry.ID, ReadinessIdentity); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("catalog error = %v", err)
	}
	unknown := entry
	unknown.ID = "unknown"
	if _, err := EvaluateModelReadiness(writeReadinessFixture(t, admission.CatalogID, []ModelReadiness{unknown}), admission, entry.ID, ReadinessIdentity); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown-entry error = %v", err)
	}
}

func TestEvaluateModelReadinessRequiresCompleteRegistryMetadata(t *testing.T) {
	admission := readinessAdmissionFixture()
	entry := passingReadinessFixture()
	path := writeReadinessFixture(t, admission.CatalogID, []ModelReadiness{entry})
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var registry ModelReadinessRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatal(err)
	}
	delete(registry.GateMeanings, "rejected")
	data, _ = json.Marshal(registry)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := EvaluateModelReadiness(path, admission, entry.ID, ReadinessIdentity); err == nil || !strings.Contains(err.Error(), "gate meaning") {
		t.Fatalf("metadata error = %v", err)
	}
}

func readinessAdmissionFixture() ModelAdmissionRegistry {
	return ModelAdmissionRegistry{
		SchemaVersion: modelAdmissionSchema,
		CatalogID:     "sne-gemma4-v1",
		Family:        "gemma-4",
		Entries:       []ModelAdmission{{CatalogEntry: "31b-mxfp8"}},
	}
}

func passingReadinessFixture() ModelReadiness {
	return ModelReadiness{
		ID: "31b-mxfp8", Exactness: "pass", Clean100: "pass-low-swap",
		ExternalRelease: "open", Disposition: "candidate",
	}
}

func writeReadinessFixture(t *testing.T, catalogID string, entries []ModelReadiness) string {
	t.Helper()
	registry := ModelReadinessRegistry{
		Schema: modelReadinessSchema, CatalogID: catalogID, AsOf: "2026-08-17",
		GateMeanings: map[string]string{"pass": "proved", "open": "unproved", "fail": "failed", "rejected": "prohibited"},
		Global:       ModelReadinessGlobal{CatalogEntries: len(entries)}, Entries: entries,
	}
	data, err := json.Marshal(registry)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "readiness.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
