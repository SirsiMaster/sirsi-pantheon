package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
	"github.com/SirsiMaster/sirsi-pantheon/internal/snemodels"
)

const sneReadModelSchema = "pantheon.sne-read-model.v1"

type SNECatalogItem struct {
	CatalogEntry            string `json:"catalog_entry"`
	ModelID                 string `json:"model_id"`
	RuntimeID               string `json:"runtime_id,omitempty"`
	ParameterClass          string `json:"parameter_class"`
	ExecutionMode           string `json:"execution_mode"`
	WeightFormat            string `json:"weight_format"`
	WeightBits              int    `json:"weight_bits"`
	WeightGroupSize         int    `json:"weight_group_size"`
	MemoryBytes             uint64 `json:"memory_bytes"`
	Qualification           string `json:"qualification"`
	SupportStatus           string `json:"support_status"`
	NextGate                string `json:"next_gate,omitempty"`
	Installed               bool   `json:"installed"`
	Active                  bool   `json:"active"`
	State                   string `json:"state"`
	ActionLabel             string `json:"action_label"`
	ActionKind              string `json:"action_kind,omitempty"`
	ActionEnabled           bool   `json:"action_enabled"`
	RemovalEnabled          bool   `json:"removal_enabled"`
	RemovalReason           string `json:"removal_reason,omitempty"`
	Reason                  string `json:"reason,omitempty"`
	LicenseID               string `json:"license_id,omitempty"`
	LicenseURL              string `json:"license_url,omitempty"`
	LicenseRequired         bool   `json:"license_acceptance_required"`
	CacheTopology           string `json:"cache_topology,omitempty"`
	ServingCacheCapacity    int    `json:"serving_cache_capacity,omitempty"`
	PrefixSessionsMaximum   int    `json:"prefix_sessions_maximum,omitempty"`
	PrefixSessionsSupported bool   `json:"prefix_sessions_supported"`
	StreamingMode           string `json:"streaming_mode,omitempty"`
}

type SNEReadModel struct {
	SchemaVersion           string                  `json:"schema_version"`
	GeneratedAt             time.Time               `json:"generated_at"`
	Configured              bool                    `json:"configured"`
	Ready                   bool                    `json:"ready"`
	ServiceState            string                  `json:"service_state"`
	ServiceURL              string                  `json:"service_url,omitempty"`
	ActiveModel             string                  `json:"active_model,omitempty"`
	DeviceFamily            string                  `json:"device_family,omitempty"`
	UnifiedMemoryBytes      uint64                  `json:"unified_memory_bytes,omitempty"`
	CatalogID               string                  `json:"catalog_id,omitempty"`
	Catalog                 []SNECatalogItem        `json:"catalog"`
	RuntimeCatalog          SNERuntimeCatalogStatus `json:"runtime_catalog"`
	Lifecycle               SNELifecycleState       `json:"lifecycle"`
	Recovery                string                  `json:"recovery,omitempty"`
	LifecycleToolsReady     bool                    `json:"lifecycle_tools_ready"`
	LifecycleToolsStatus    string                  `json:"lifecycle_tools_status,omitempty"`
	QueueTelemetryAvailable bool                    `json:"queue_telemetry_available"`
	RequestsActive          int64                   `json:"requests_active,omitempty"`
	RequestsQueued          int                     `json:"requests_queued,omitempty"`
	MaxConcurrentRequests   int                     `json:"max_concurrent_requests,omitempty"`
	MaxQueuedRequests       int                     `json:"max_queued_requests,omitempty"`
	QueueDiscipline         string                  `json:"queue_discipline,omitempty"`
	RequestTimeoutMS        int64                   `json:"request_timeout_ms,omitempty"`
}

func supportsPrefixSessions(executionMode string, maximum int) bool {
	return executionMode == "plain" && maximum > 0
}

func (s *Server) apiSNE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	allowNexusOrigin(w, r)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	model, err := s.sneReadModel(ctx)
	if err != nil {
		writeError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, model)
}

func (s *Server) sneReadModel(ctx context.Context) (SNEReadModel, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return SNEReadModel{}, fmt.Errorf("resolve home directory: %w", err)
	}
	registryPath := filepath.Join(home, "Library", "Application Support", "Sirsi", "Pantheon", "model-admission.json")
	supportMatrixPath := filepath.Join(home, "Library", "Application Support", "Sirsi", "Pantheon", "support-matrix-current", "support-matrix.json")
	supportMatrixSignature := supportMatrixPath + ".sig"
	supportMatrixPublicKey := filepath.Join(home, "Library", "Application Support", "Sirsi", "Pantheon", "runtime-catalog-ed25519.pub")
	runtimeRoot := filepath.Join(home, "Library", "Application Support", "Sirsi", "SNE-Runtimes")
	storeRoot := ""
	available := map[string]bool{}
	sources := map[string]snemodels.SourceEntry{}
	runtimeAvailable := map[string]bool{}
	runtimeSelections := map[string][]string{}
	runtimeCapabilities := map[string]SNERuntimeCapabilities{}
	if s.sneJobs != nil {
		registryPath = s.sneJobs.cfg.AdmissionRegistry
		storeRoot = s.sneJobs.cfg.StoreRoot
		available = s.sneJobs.Available()
		if catalog, sourceErr := snemodels.LoadSourceCatalog(s.sneJobs.cfg.SourceCatalog); sourceErr == nil {
			for _, source := range catalog.Entries {
				sources[source.CatalogEntry] = source
			}
		}
	}
	if s.sneLifecycle != nil {
		runtimeAvailable = s.sneLifecycle.Available()
		runtimeSelections = s.sneLifecycle.RuntimeSelections()
		runtimeCapabilities = s.sneLifecycle.RuntimeCapabilities()
	}
	view := SNEReadModel{
		SchemaVersion:      sneReadModelSchema,
		GeneratedAt:        time.Now().UTC(),
		DeviceFamily:       sysctlString("machdep.cpu.brand_string"),
		UnifiedMemoryBytes: sysctlUint64("hw.memsize"),
		ServiceState:       "not-configured",
		Catalog:            []SNECatalogItem{},
		RuntimeCatalog:     SNERuntimeCatalogStatus{State: "not-configured"},
		Lifecycle:          SNELifecycleState{State: "not-configured"},
	}
	if s.sneJobs != nil {
		view.LifecycleToolsReady, view.LifecycleToolsStatus = s.sneJobs.LifecycleToolsStatus()
	} else {
		view.LifecycleToolsStatus = "SNE lifecycle tools are not configured."
	}
	if s.sneLifecycle != nil {
		view.RuntimeCatalog = s.sneLifecycle.CatalogStatus()
		view.Lifecycle = s.sneLifecycle.Snapshot()
		if view.Lifecycle.State == "starting" || view.Lifecycle.State == "stopping" || view.Lifecycle.State == "failed" {
			view.ServiceState = view.Lifecycle.State
		}
		applySNELifecycleRecovery(&view)
	}
	registry, err := sne.LoadModelAdmissionRegistry(registryPath)
	if err != nil {
		if os.IsNotExist(rootCause(err)) {
			view.Recovery = "Install Pantheon SNE model admission data."
			return view, nil
		}
		return SNEReadModel{}, err
	}
	view.Configured = true
	view.CatalogID = registry.CatalogID
	view.ServiceState = "stopped"
	view.ServiceURL = "http://127.0.0.1:8477"
	supportMatrix, supportErr := sne.LoadSignedSupportMatrix(supportMatrixPath, supportMatrixSignature, supportMatrixPublicKey)
	supportByEntry := map[string]sne.SupportMatrixEntry{}
	if supportErr == nil {
		supportByEntry = supportMatrix.ByCatalogEntry()
	} else {
		view.Recovery = "Install or repair the verified SNE support matrix before installing or starting a model."
	}

	installed := installedSNEModels(runtimeRoot)
	for model := range installedSNEModels(storeRoot) {
		installed[model] = true
	}
	active := map[string]bool{}
	client, clientErr := s.newSNEReadClient("http://127.0.0.1:8477")
	identity, identityErr := sne.ServiceReadinessIdentity{}, clientErr
	if clientErr == nil {
		identity, identityErr = client.ReadinessIdentity(ctx)
	}
	if identityErr == nil && sneReadinessMatchesLifecycle(identity, view.Lifecycle) {
		view.Ready = true
		view.ServiceState = "ready"
		view.MaxConcurrentRequests = identity.MaxConcurrentRequests
		view.MaxQueuedRequests = identity.MaxQueuedRequests
		view.QueueDiscipline = identity.QueueDiscipline
		view.RequestTimeoutMS = identity.RequestTimeoutMS
		if metrics, metricsErr := client.Metrics(ctx); metricsErr == nil &&
			metrics.MaxConcurrentRequests == identity.MaxConcurrentRequests &&
			metrics.MaxQueuedRequests == identity.MaxQueuedRequests &&
			metrics.QueueDiscipline == identity.QueueDiscipline &&
			metrics.RequestTimeoutMS == identity.RequestTimeoutMS {
			view.QueueTelemetryAvailable = true
			view.RequestsActive = metrics.RequestsActive
			view.RequestsQueued = metrics.RequestsQueued
		}
		for _, model := range identity.Models {
			active[model.ID] = true
			if view.ActiveModel == "" {
				view.ActiveModel = model.ID
			}
		}
	} else if identityErr == nil {
		if recovery := sneReadinessMismatchRecovery(identity, view.Lifecycle); recovery != "" {
			view.ServiceState = "identity-mismatch"
			view.Recovery = recovery
		}
	}
	if !view.Ready && view.Recovery == "" {
		view.Recovery = "Start an installed, compatible model through Pantheon."
	}

	for _, entry := range registry.Entries {
		selections := runtimeSelections[entry.ModelID]
		if len(selections) == 0 {
			selections = []string{""}
		}
		for _, runtimeID := range selections {
			capability := runtimeCapabilities[entry.ModelID+"\x00"+runtimeID]
			support, supportFound := supportByEntry[entry.CatalogEntry]
			source, sourceFound := sources[entry.CatalogEntry]
			item := SNECatalogItem{
				CatalogEntry: entry.CatalogEntry, ModelID: entry.ModelID,
				RuntimeID:      runtimeID,
				ParameterClass: entry.ParameterClass, ExecutionMode: entry.ExecutionMode,
				WeightFormat: entry.WeightFormat, WeightBits: entry.WeightBits,
				WeightGroupSize: entry.WeightGroupSize, MemoryBytes: entry.MemoryBytes,
				Qualification: entry.Qualification, Installed: installed[entry.ModelID], SupportStatus: "unqualified",
				Active: active[entry.ModelID] && (view.Lifecycle.RuntimeID == "" || view.Lifecycle.RuntimeID == runtimeID), ActionEnabled: false,
				CacheTopology: capability.CacheTopology, ServingCacheCapacity: capability.ServingCacheCapacity,
				PrefixSessionsMaximum: capability.PrefixSessionsMaximum, StreamingMode: capability.StreamingMode,
			}
			item.PrefixSessionsSupported = supportsPrefixSessions(item.ExecutionMode, item.PrefixSessionsMaximum)
			if sourceFound {
				item.LicenseID = source.LicenseID
				item.LicenseURL = sneLicenseTermsURL(source.LicenseID)
				item.LicenseRequired = source.LicenseID != ""
			}
			if supportFound && support.ModelID == entry.ModelID && support.Architecture == entry.Architecture && support.ParameterClass == entry.ParameterClass && support.ArchitectureAdapter == entry.Adapter && support.ExecutionMode == entry.ExecutionMode && support.Weight.Format == entry.WeightFormat && support.Weight.Bits == entry.WeightBits && support.Weight.GroupSize == entry.WeightGroupSize && support.RequiredMemoryBytes == entry.MemoryBytes && support.ArtifactSetSHA256 == entry.ArtifactSetSHA256 {
				item.SupportStatus, item.NextGate = support.SupportStatus, support.NextGate
			} else if supportFound {
				item.NextGate = "support-identity-repair"
			}
			switch {
			case item.Active:
				item.State, item.ActionLabel, item.ActionKind = "ready", "Stop", "stop"
				item.ActionEnabled = s.sneLifecycle != nil
			case item.SupportStatus != "release-supported":
				item.State, item.ActionLabel = item.SupportStatus, "Qualification pending"
				item.Reason = "This exact tuple is not release-supported; next gate: " + item.NextGate + "."
			case view.UnifiedMemoryBytes > 0 && item.MemoryBytes > view.UnifiedMemoryBytes:
				item.State, item.ActionLabel = "incompatible-memory", "Unavailable"
				item.Reason = "Declared model memory exceeds this Mac's unified memory."
			case entry.Qualification == "research":
				item.State, item.ActionLabel = "research", "Research opt-in required"
			case item.Installed:
				item.State, item.ActionLabel, item.ActionKind = "installed", "Start", "start"
				item.ActionEnabled = runtimeAvailable[item.ModelID]
				item.RemovalEnabled = view.LifecycleToolsReady && s.sneLifecycle != nil && view.Lifecycle.State == "stopped"
				if item.RemovalEnabled {
					item.RemovalReason = "Remove this installed model transactionally; shared objects remain available to other models."
				} else {
					item.RemovalReason = "Stop SNE before removing this installed model."
				}
				if item.ActionEnabled {
					item.Reason = "Start this exact installed tuple with its verified packaged runtime."
				} else {
					item.Reason = "No verified runtime package is installed for this exact tuple."
				}
			case available[entry.CatalogEntry] && (item.LicenseID == "" || item.LicenseURL == ""):
				item.State, item.ActionLabel = "license-unavailable", "Unavailable"
				item.Reason = "Pantheon cannot present verified license terms for this source, so installation is disabled."
			case available[entry.CatalogEntry]:
				item.State, item.ActionLabel, item.ActionKind, item.ActionEnabled = "available", "Install", "install", true
				item.Reason = "Download, verify, and promote this exact tuple transactionally."
			default:
				item.State, item.ActionLabel = "source-unavailable", "Unavailable"
				item.Reason = "No signed acquisition source is installed for this exact tuple."
			}
			view.Catalog = append(view.Catalog, item)
		}
	}
	enforceSNEReadModelSupportInvariant(&view)
	return view, nil
}

// enforceSNEReadModelSupportInvariant prevents a live but unsupported tuple
// from being projected as product-ready. The runtime remains stoppable so a
// user can recover without killing processes manually, but Nexus/OpenAI and
// readiness consumers must fail closed until the exact signed support identity
// is release-supported.
func enforceSNEReadModelSupportInvariant(view *SNEReadModel) {
	if view == nil {
		return
	}
	releaseSupportedActive := false
	for index := range view.Catalog {
		item := &view.Catalog[index]
		if !item.Active || item.ModelID != view.ActiveModel {
			continue
		}
		if item.SupportStatus == "release-supported" {
			releaseSupportedActive = true
			continue
		}
		item.State = "support-mismatch"
		item.ActionLabel = "Stop"
		item.ActionKind = "stop"
		item.Reason = "This running tuple is not release-supported by the installed signed support matrix. Stop it, repair the catalog, and restart the exact admitted tuple."
	}
	if view.Ready && !releaseSupportedActive {
		view.Ready = false
		view.ServiceState = "support-mismatch"
		view.Recovery = "Stop the unsupported local SNE tuple, repair the signed support matrix, and restart the exact release-supported model and runtime through Pantheon."
	}
}

func applySNELifecycleRecovery(view *SNEReadModel) {
	if view == nil || view.Ready || view.Lifecycle.ErrorCode != sneMetalSessionLockedCode {
		return
	}
	view.ServiceState = "waiting-for-unlock"
	view.Recovery = view.Lifecycle.Recovery
}

func (s *Server) newSNEReadClient(baseURL string) (*sne.Client, error) {
	token := ""
	if s != nil && s.sneAccess != nil {
		token = s.sneAccess.snapshot()
	}
	return sne.NewAuthenticatedClient(baseURL, token)
}

func sneReadinessMatchesLifecycle(identity sne.ServiceReadinessIdentity, lifecycle SNELifecycleState) bool {
	if lifecycle.State != "ready" || lifecycle.ModelID == "" || lifecycle.Profile == "" ||
		len(lifecycle.RuntimeSHA256) != 64 || len(lifecycle.NativeRuntimeSHA256) != 64 || len(lifecycle.ModelManifestSHA256) != 64 {
		return false
	}
	if identity.Status != "ready" || identity.APIVersion != "v0" ||
		identity.APIContract != "sne.openai-chat.v2" || identity.ReadyAPIContract != identity.APIContract {
		return false
	}
	if identity.Profile != lifecycle.Profile || identity.ReadyProfile != lifecycle.Profile ||
		identity.RuntimeSHA256 != lifecycle.RuntimeSHA256 || identity.ReadyRuntimeSHA256 != lifecycle.RuntimeSHA256 ||
		identity.NativeRuntimeSHA256 != lifecycle.NativeRuntimeSHA256 || identity.ReadyNativeRuntimeSHA256 != lifecycle.NativeRuntimeSHA256 ||
		identity.LoadedModel != lifecycle.ModelID || identity.ReadyModelID != lifecycle.ModelID ||
		identity.ReadyManifestSHA256 != lifecycle.ModelManifestSHA256 {
		return false
	}
	if len(identity.Models) != 1 || identity.Models[0].ID != lifecycle.ModelID ||
		identity.Models[0].ManifestSHA256 != lifecycle.ModelManifestSHA256 {
		return false
	}
	return identity.MaxConcurrentRequests > 0 && identity.MaxConcurrentRequests == identity.ReadyMaxConcurrentRequests &&
		identity.MaxQueuedRequests >= 0 && identity.MaxQueuedRequests == identity.ReadyMaxQueuedRequests &&
		identity.QueueDiscipline != "" && identity.QueueDiscipline == identity.ReadyQueueDiscipline &&
		identity.RequestTimeoutMS > 0 && identity.RequestTimeoutMS == identity.ReadyRequestTimeoutMS
}

func sneReadinessMismatchRecovery(identity sne.ServiceReadinessIdentity, lifecycle SNELifecycleState) string {
	if identity.Status != "ready" || sneReadinessMatchesLifecycle(identity, lifecycle) {
		return ""
	}
	return "Stop the unverified local SNE service, then start the installed model through Pantheon so its exact runtime and model identity can be supervised."
}

func sneLicenseTermsURL(identifier string) string {
	switch identifier {
	case "gemma-terms":
		return "https://ai.google.dev/gemma/terms"
	case "apache-2.0":
		return "https://www.apache.org/licenses/LICENSE-2.0"
	default:
		return ""
	}
}

func installedSNEModels(root string) map[string]bool {
	installed := map[string]bool{}
	if strings.TrimSpace(root) == "" {
		return installed
	}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Name() != "model.json" && entry.Name() != ".sne-checkout.json" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		switch entry.Name() {
		case "model.json":
			if filepath.Base(filepath.Dir(path)) != "manifests" {
				return nil
			}
			var manifest struct {
				Model struct {
					ID string `json:"id"`
				} `json:"model"`
			}
			if json.Unmarshal(data, &manifest) == nil && manifest.Model.ID != "" {
				installed[manifest.Model.ID] = true
			}
		case ".sne-checkout.json":
			var receipt struct {
				ModelID string `json:"model_id"`
			}
			if json.Unmarshal(data, &receipt) == nil && receipt.ModelID != "" {
				installed[receipt.ModelID] = true
			}
		}
		return nil
	})
	return installed
}

func sysctlString(name string) string {
	output, err := exec.Command("sysctl", "-n", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func sysctlUint64(name string) uint64 {
	value, err := strconv.ParseUint(sysctlString(name), 10, 64)
	if err != nil {
		return 0
	}
	return value
}

func rootCause(err error) error {
	for {
		type unwrapper interface{ Unwrap() error }
		wrapped, ok := err.(unwrapper)
		if !ok || wrapped.Unwrap() == nil {
			return err
		}
		err = wrapped.Unwrap()
	}
}
