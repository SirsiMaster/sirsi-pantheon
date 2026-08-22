package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type sneOpenAIErrorBody struct {
	Error struct {
		Message string  `json:"message"`
		Type    string  `json:"type"`
		Param   *string `json:"param"`
		Code    string  `json:"code"`
	} `json:"error"`
	SNE *sneOpenAIRecovery `json:"sne,omitempty"`
}

type sneOpenAIRecovery struct {
	NoFallback        bool   `json:"no_fallback"`
	Recovery          string `json:"recovery,omitempty"`
	SupportStatus     string `json:"support_status,omitempty"`
	NextGate          string `json:"next_gate,omitempty"`
	RequiredBytes     uint64 `json:"required_bytes,omitempty"`
	AvailableRAMBytes uint64 `json:"available_ram_bytes,omitempty"`
	SwapUsedBytes     uint64 `json:"swap_used_bytes,omitempty"`
	SwapLimitBytes    uint64 `json:"swap_limit_bytes,omitempty"`
}

func writeSNEOpenAIAdmissionError(w http.ResponseWriter, item SNECatalogItem) {
	response := sneOpenAIErrorBody{}
	response.Error.Message = "the active SNE model and runtime tuple is not release-supported"
	response.Error.Type = "sne_local_error"
	response.Error.Code = "sne_tuple_not_release_supported"
	response.SNE = &sneOpenAIRecovery{
		NoFallback:    true,
		Recovery:      "Complete the listed qualification gate or select an exact release-supported local tuple.",
		SupportStatus: item.SupportStatus,
		NextGate:      item.NextGate,
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(response)
}

func writeSNEOpenAIError(w http.ResponseWriter, status int, code, message string, lifecycle *SNELifecycleState) {
	response := sneOpenAIErrorBody{}
	response.Error.Message = message
	response.Error.Type = "sne_local_error"
	response.Error.Code = code
	response.SNE = &sneOpenAIRecovery{NoFallback: true}
	if lifecycle != nil {
		response.SNE.Recovery = lifecycle.Recovery
		if lifecycle.ResourceAdmission != nil {
			response.SNE.RequiredBytes = lifecycle.ResourceAdmission.RequiredBytes
			response.SNE.AvailableRAMBytes = lifecycle.ResourceAdmission.AvailableRAMBytes
			response.SNE.SwapUsedBytes = lifecycle.ResourceAdmission.SwapUsedBytes
			response.SNE.SwapLimitBytes = lifecycle.ResourceAdmission.SwapLimitBytes
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

func writeSNELocalCapabilityError(w http.ResponseWriter, status int, code, message string) {
	response := sneOpenAIErrorBody{}
	response.Error.Message = message
	response.Error.Type = "sne_local_access_error"
	response.Error.Code = code
	response.SNE = &sneOpenAIRecovery{
		NoFallback: true,
		Recovery:   "Reopen Nexus from Pantheon or run `sirsi sne open` to establish a new local session.",
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

type sneOpenAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
	SNE     struct {
		RuntimeID               string `json:"runtime_id"`
		RuntimeSHA256           string `json:"runtime_sha256"`
		ModelManifestSHA256     string `json:"model_manifest_sha256"`
		Profile                 string `json:"profile"`
		RuntimeCatalogID        string `json:"runtime_catalog_id"`
		CatalogVersionSHA       string `json:"catalog_version_sha256"`
		DeviceFamily            string `json:"device_family"`
		ExecutionMode           string `json:"execution_mode,omitempty"`
		WeightFormat            string `json:"weight_format,omitempty"`
		WeightBits              int    `json:"weight_bits,omitempty"`
		Qualification           string `json:"qualification,omitempty"`
		SupportStatus           string `json:"support_status"`
		ModelCatalogEntry       string `json:"model_catalog_entry,omitempty"`
		CacheTopology           string `json:"cache_topology,omitempty"`
		ServingCacheCapacity    int    `json:"serving_cache_capacity,omitempty"`
		PrefixSessionsMaximum   int    `json:"prefix_sessions_maximum,omitempty"`
		PrefixSessionsSupported bool   `json:"prefix_sessions_supported"`
	} `json:"sne"`
}

type sneOpenAIModelList struct {
	Object string           `json:"object"`
	Data   []sneOpenAIModel `json:"data"`
}

func (s *Server) apiSNEOpenAIModels(w http.ResponseWriter, request *http.Request) {
	allowNexusOrigin(w, request)
	if origin := strings.TrimSpace(request.Header.Get("Origin")); origin != "" && w.Header().Get("Access-Control-Allow-Origin") == "" {
		writeSNEOpenAIError(w, http.StatusForbidden, "origin_not_allowed", "origin is not allowed", nil)
		return
	}
	if request.Method != http.MethodGet {
		writeSNEOpenAIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required", nil)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	model, err := s.sneReadModel(ctx)
	if err != nil {
		writeSNEOpenAIError(w, http.StatusServiceUnavailable, "sne_status_unavailable", "Pantheon could not verify the local SNE status", nil)
		return
	}
	item, admitted := activeReleaseSupportedSNETuple(model)
	if !admitted && model.Ready && model.ActiveModel != "" && model.Lifecycle.State == "ready" {
		writeSNEOpenAIAdmissionError(w, item)
		return
	}
	response, ok := buildSNEOpenAIModelList(model)
	if !ok {
		code := model.Lifecycle.ErrorCode
		if code == "" {
			code = "sne_not_ready"
		}
		writeSNEOpenAIError(w, http.StatusServiceUnavailable, code, "the verified SNE runtime is not ready", &model.Lifecycle)
		return
	}
	writeJSON(w, response)
}

func buildSNEOpenAIModelList(model SNEReadModel) (sneOpenAIModelList, bool) {
	item, ok := activeReleaseSupportedSNETuple(model)
	if !ok {
		return sneOpenAIModelList{}, false
	}
	entry := sneOpenAIModel{ID: model.ActiveModel, Object: "model", Created: model.GeneratedAt.Unix(), OwnedBy: "sirsi"}
	entry.SNE.RuntimeID = model.Lifecycle.RuntimeID
	entry.SNE.RuntimeSHA256 = model.Lifecycle.RuntimeSHA256
	entry.SNE.ModelManifestSHA256 = model.Lifecycle.ModelManifestSHA256
	entry.SNE.Profile = model.Lifecycle.Profile
	entry.SNE.RuntimeCatalogID = model.RuntimeCatalog.CatalogID
	entry.SNE.CatalogVersionSHA = model.RuntimeCatalog.VersionSHA256
	entry.SNE.DeviceFamily = model.DeviceFamily
	entry.SNE.ExecutionMode = item.ExecutionMode
	entry.SNE.WeightFormat = item.WeightFormat
	entry.SNE.WeightBits = item.WeightBits
	entry.SNE.Qualification = item.Qualification
	entry.SNE.SupportStatus = item.SupportStatus
	entry.SNE.ModelCatalogEntry = item.CatalogEntry
	entry.SNE.CacheTopology = item.CacheTopology
	entry.SNE.ServingCacheCapacity = item.ServingCacheCapacity
	entry.SNE.PrefixSessionsMaximum = item.PrefixSessionsMaximum
	entry.SNE.PrefixSessionsSupported = item.PrefixSessionsSupported
	return sneOpenAIModelList{Object: "list", Data: []sneOpenAIModel{entry}}, true
}

func activeReleaseSupportedSNETuple(model SNEReadModel) (SNECatalogItem, bool) {
	if !model.Ready || model.ActiveModel == "" || model.Lifecycle.State != "ready" || model.Lifecycle.RuntimeID == "" || !isLowerSHA256(model.Lifecycle.RuntimeSHA256) || !isLowerSHA256(model.Lifecycle.ModelManifestSHA256) || strings.TrimSpace(model.Lifecycle.Profile) == "" || model.RuntimeCatalog.State != "verified" || !model.RuntimeCatalog.SignedRequired {
		return SNECatalogItem{}, false
	}
	for _, candidate := range model.Catalog {
		if candidate.Active && candidate.ModelID == model.ActiveModel && candidate.RuntimeID == model.Lifecycle.RuntimeID {
			return candidate, candidate.SupportStatus == "release-supported"
		}
	}
	return SNECatalogItem{}, false
}

func isLowerSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
