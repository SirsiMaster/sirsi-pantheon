package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

const sneSupportDiagnosticsSchema = "pantheon.sne-support-diagnostics.v1"

type sneSupportCatalog struct {
	State                string   `json:"state"`
	SignedRequired       bool     `json:"signed_required"`
	CatalogID            string   `json:"catalog_id,omitempty"`
	VersionSHA256        string   `json:"version_sha256,omitempty"`
	Entries              int      `json:"entries"`
	Versions             int      `json:"versions"`
	RetainedVersions     []string `json:"retained_versions"`
	RollbackAvailable    bool     `json:"rollback_available"`
	UpdateFeedConfigured bool     `json:"update_feed_configured"`
}

type sneSupportLifecycle struct {
	State               string `json:"state"`
	ModelID             string `json:"model_id,omitempty"`
	RuntimeID           string `json:"runtime_id,omitempty"`
	RuntimeSHA256       string `json:"runtime_sha256,omitempty"`
	ModelManifestSHA256 string `json:"model_manifest_sha256,omitempty"`
	Profile             string `json:"profile,omitempty"`
	ErrorCode           string `json:"error_code,omitempty"`
	Recovery            string `json:"recovery,omitempty"`
}

type sneSupportModel struct {
	CatalogEntry    string `json:"catalog_entry"`
	ModelID         string `json:"model_id"`
	RuntimeID       string `json:"runtime_id,omitempty"`
	ParameterClass  string `json:"parameter_class"`
	ExecutionMode   string `json:"execution_mode"`
	WeightFormat    string `json:"weight_format"`
	WeightBits      int    `json:"weight_bits"`
	MemoryBytes     uint64 `json:"memory_bytes"`
	Qualification   string `json:"qualification"`
	SupportStatus   string `json:"support_status"`
	NextGate        string `json:"next_gate,omitempty"`
	Installed       bool   `json:"installed"`
	Active          bool   `json:"active"`
	State           string `json:"state"`
	CacheTopology   string `json:"cache_topology,omitempty"`
	ServingCapacity int    `json:"serving_cache_capacity,omitempty"`
	PrefixMaximum   int    `json:"prefix_sessions_maximum,omitempty"`
	PrefixSupported bool   `json:"prefix_sessions_supported"`
	StreamingMode   string `json:"streaming_mode,omitempty"`
}

type sneSupportDiagnostics struct {
	SchemaVersion        string                `json:"schema_version"`
	GeneratedAt          time.Time             `json:"generated_at"`
	Platform             string                `json:"platform"`
	Architecture         string                `json:"architecture"`
	DeviceFamily         string                `json:"device_family,omitempty"`
	UnifiedMemory        uint64                `json:"unified_memory_bytes,omitempty"`
	ServiceState         string                `json:"service_state"`
	ActiveModel          string                `json:"active_model,omitempty"`
	Catalog              sneSupportCatalog     `json:"runtime_catalog"`
	Lifecycle            sneSupportLifecycle   `json:"lifecycle"`
	Models               []sneSupportModel     `json:"models"`
	Resources            sne.ResourceAdmission `json:"resources"`
	LifecycleToolsReady  bool                  `json:"lifecycle_tools_ready"`
	LifecycleToolsStatus string                `json:"lifecycle_tools_status,omitempty"`
	ApplicationRecovery  []recoveryTargetView  `json:"application_recovery"`
}

func buildSNESupportDiagnostics(model SNEReadModel, resources sne.ResourceAdmission, recovery []recoveryTargetView, now time.Time) sneSupportDiagnostics {
	if model.Lifecycle.ResourceAdmission != nil {
		resources = *model.Lifecycle.ResourceAdmission
	}
	diagnostics := sneSupportDiagnostics{
		SchemaVersion: sneSupportDiagnosticsSchema,
		GeneratedAt:   now.UTC(),
		Platform:      runtime.GOOS,
		Architecture:  runtime.GOARCH,
		DeviceFamily:  model.DeviceFamily,
		UnifiedMemory: model.UnifiedMemoryBytes,
		ServiceState:  model.ServiceState,
		ActiveModel:   model.ActiveModel,
		Catalog: sneSupportCatalog{
			State: model.RuntimeCatalog.State, SignedRequired: model.RuntimeCatalog.SignedRequired,
			CatalogID: model.RuntimeCatalog.CatalogID, VersionSHA256: model.RuntimeCatalog.VersionSHA256,
			Entries: model.RuntimeCatalog.Entries, Versions: model.RuntimeCatalog.Versions,
			RetainedVersions:     append([]string(nil), model.RuntimeCatalog.RetainedVersions...),
			RollbackAvailable:    model.RuntimeCatalog.RollbackAvailable,
			UpdateFeedConfigured: model.RuntimeCatalog.UpdateFeedConfigured,
		},
		Lifecycle: sneSupportLifecycle{
			State: model.Lifecycle.State, ModelID: model.Lifecycle.ModelID, RuntimeID: model.Lifecycle.RuntimeID,
			RuntimeSHA256: model.Lifecycle.RuntimeSHA256, ModelManifestSHA256: model.Lifecycle.ModelManifestSHA256,
			Profile: model.Lifecycle.Profile, ErrorCode: model.Lifecycle.ErrorCode, Recovery: model.Lifecycle.Recovery,
		},
		Models:               make([]sneSupportModel, 0, len(model.Catalog)),
		Resources:            resources,
		LifecycleToolsReady:  model.LifecycleToolsReady,
		LifecycleToolsStatus: model.LifecycleToolsStatus,
		ApplicationRecovery:  append([]recoveryTargetView(nil), recovery...),
	}
	for _, item := range model.Catalog {
		diagnostics.Models = append(diagnostics.Models, sneSupportModel{
			CatalogEntry: item.CatalogEntry, ModelID: item.ModelID, RuntimeID: item.RuntimeID,
			ParameterClass: item.ParameterClass, ExecutionMode: item.ExecutionMode,
			WeightFormat: item.WeightFormat, WeightBits: item.WeightBits, MemoryBytes: item.MemoryBytes,
			Qualification: item.Qualification, SupportStatus: item.SupportStatus, NextGate: item.NextGate, Installed: item.Installed, Active: item.Active, State: item.State,
			CacheTopology: item.CacheTopology, ServingCapacity: item.ServingCacheCapacity,
			PrefixMaximum: item.PrefixSessionsMaximum, PrefixSupported: item.PrefixSessionsSupported, StreamingMode: item.StreamingMode,
		})
	}
	return diagnostics
}

func (s *Server) apiSNEDiagnostics(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	model, err := s.sneReadModel(ctx)
	if err != nil {
		writeError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	now := time.Now().UTC()
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="sirsi-sne-diagnostics-%s.json"`, now.Format("20060102T150405Z")))
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, buildSNESupportDiagnostics(model, sne.SampleResourceState(), s.recoveryViews(), now))
}

func (s *Server) apiSNESupportBundle(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if !sameOriginRequest(request) {
		writeError(w, "cross-origin support export rejected", http.StatusForbidden)
		return
	}
	if s.sneJobs == nil {
		writeError(w, "packaged SNE support export is unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 15*time.Second)
	defer cancel()
	archive, err := s.sneJobs.SupportBundle(ctx)
	if err != nil {
		writeError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	now := time.Now().UTC()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="sirsi-sne-support-%s.zip"`, now.Format("20060102T150405Z")))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Sirsi-Support-Privacy-Verified", "true")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(archive)
}
