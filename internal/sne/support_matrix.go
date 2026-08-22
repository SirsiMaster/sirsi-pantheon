package sne

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const supportMatrixSchema = "sne.support-matrix.v2"

type SupportMatrix struct {
	Schema          string               `json:"schema"`
	CatalogRoot     string               `json:"catalog_root"`
	AsOf            string               `json:"as_of"`
	SelectionPolicy string               `json:"selection_policy"`
	Fallback        string               `json:"fallback"`
	Entries         []SupportMatrixEntry `json:"entries"`
	ClaimBoundaries struct {
		DeviceNonprojection bool `json:"device_nonprojection"`
		MTPPlainSeparation  bool `json:"mtp_plain_separation"`
		PrecisionSeparation bool `json:"precision_separation"`
		CandidateNotSupport bool `json:"candidate_not_support"`
		ServingPolicyBound  bool `json:"serving_policy_bound"`
	} `json:"claim_boundaries"`
}

type SupportMatrixEntry struct {
	TupleID             string    `json:"tuple_id"`
	CatalogEntryID      string    `json:"catalog_entry_id"`
	ModelID             string    `json:"model_id"`
	ModelRevision       string    `json:"model_revision"`
	ArtifactSetSHA256   string    `json:"artifact_set_sha256"`
	EvidenceSHA256      string    `json:"evidence_manifest_sha256"`
	Family              string    `json:"family"`
	Architecture        string    `json:"architecture"`
	ParameterClass      string    `json:"parameter_class"`
	ArchitectureAdapter string    `json:"architecture_adapter"`
	ExecutionMode       string    `json:"execution_mode"`
	Weight              Precision `json:"weight"`
	Assistant           *struct {
		ModelID          string    `json:"model_id"`
		Revision         string    `json:"revision"`
		CheckpointSHA256 string    `json:"checkpoint_sha256"`
		Precision        Precision `json:"precision"`
	} `json:"assistant,omitempty"`
	DeviceFamilies        []string             `json:"device_families"`
	RequiredMemoryBytes   uint64               `json:"required_memory_bytes"`
	ServingCacheCapacity  int                  `json:"serving_cache_capacity"`
	ServingPolicy         SupportServingPolicy `json:"serving_policy"`
	ManifestQualification string               `json:"manifest_qualification"`
	SupportStatus         string               `json:"support_status"`
	NextGate              string               `json:"next_gate"`
	Fallback              string               `json:"fallback"`
}

type SupportServingPolicy struct {
	Profile               string `json:"profile"`
	MaxConcurrentRequests int    `json:"max_concurrent_requests"`
	MaxQueuedRequests     int    `json:"max_queued_requests"`
	QueueDiscipline       string `json:"queue_discipline"`
	RequestTimeoutMS      int64  `json:"request_timeout_ms"`
}

type Precision struct {
	Format    string `json:"format"`
	Bits      int    `json:"bits"`
	GroupSize int    `json:"group_size,omitempty"`
}

func LoadSupportMatrix(path string) (SupportMatrix, error) {
	f, err := os.Open(path)
	if err != nil {
		return SupportMatrix{}, fmt.Errorf("open SNE support matrix: %w", err)
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	var matrix SupportMatrix
	if err := decoder.Decode(&matrix); err != nil {
		return SupportMatrix{}, fmt.Errorf("decode SNE support matrix: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return SupportMatrix{}, fmt.Errorf("SNE support matrix contains trailing JSON")
	}
	if matrix.Schema != supportMatrixSchema || matrix.CatalogRoot == "" || matrix.SelectionPolicy != "exact-tuple-evidence-only" || matrix.Fallback != "none" || len(matrix.Entries) == 0 {
		return SupportMatrix{}, fmt.Errorf("unsupported SNE support matrix")
	}
	if _, err := time.Parse(time.RFC3339, matrix.AsOf); err != nil {
		return SupportMatrix{}, fmt.Errorf("SNE support matrix as_of is invalid")
	}
	b := matrix.ClaimBoundaries
	if !b.DeviceNonprojection || !b.MTPPlainSeparation || !b.PrecisionSeparation || !b.CandidateNotSupport || !b.ServingPolicyBound {
		return SupportMatrix{}, fmt.Errorf("SNE support matrix claim boundaries are incomplete")
	}
	seen := make(map[string]struct{}, len(matrix.Entries))
	for _, entry := range matrix.Entries {
		if entry.TupleID == "" || entry.CatalogEntryID == "" || entry.ModelID == "" || entry.ModelRevision == "" || entry.ArtifactSetSHA256 == "" || entry.EvidenceSHA256 == "" || entry.Family == "" || entry.Architecture == "" || entry.ParameterClass == "" || entry.ArchitectureAdapter == "" || entry.ExecutionMode == "" || entry.Weight.Format == "" || entry.Weight.Bits < 1 || len(entry.DeviceFamilies) == 0 || entry.RequiredMemoryBytes == 0 || entry.ServingCacheCapacity < 1 || entry.Fallback != "none" {
			return SupportMatrix{}, fmt.Errorf("SNE support tuple %q is incomplete", entry.TupleID)
		}
		policy := entry.ServingPolicy
		if policy != expectedServingPolicy("interactive") {
			return SupportMatrix{}, fmt.Errorf("SNE support tuple %q has invalid serving policy", entry.TupleID)
		}
		switch entry.SupportStatus {
		case "release-supported", "pilot-candidate", "research-only", "unqualified":
		default:
			return SupportMatrix{}, fmt.Errorf("SNE support tuple %q has invalid status", entry.TupleID)
		}
		if entry.SupportStatus == "release-supported" && entry.NextGate != "complete" {
			return SupportMatrix{}, fmt.Errorf("SNE support tuple %q claims release support before completion", entry.TupleID)
		}
		if entry.ExecutionMode == "mtp" && entry.Assistant == nil || entry.ExecutionMode != "mtp" && entry.Assistant != nil {
			return SupportMatrix{}, fmt.Errorf("SNE support tuple %q assistant identity disagrees with execution mode", entry.TupleID)
		}
		key := entry.CatalogEntryID + "|" + entry.ModelID
		if _, exists := seen[key]; exists {
			return SupportMatrix{}, fmt.Errorf("duplicate SNE support tuple %q", key)
		}
		seen[key] = struct{}{}
	}
	return matrix, nil
}

func LoadSignedSupportMatrix(path, signaturePath, publicKeyPath string) (SupportMatrix, error) {
	if err := verifyDetachedEd25519("SNE support matrix", path, signaturePath, publicKeyPath); err != nil {
		return SupportMatrix{}, err
	}
	return LoadSupportMatrix(path)
}

func (matrix SupportMatrix) ByCatalogEntry() map[string]SupportMatrixEntry {
	entries := make(map[string]SupportMatrixEntry, len(matrix.Entries))
	for _, entry := range matrix.Entries {
		entries[strings.TrimSpace(entry.CatalogEntryID)] = entry
	}
	return entries
}
