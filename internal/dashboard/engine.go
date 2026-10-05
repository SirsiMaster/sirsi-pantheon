package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/engine"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerboard"
)

// EngineSelection exposes the engine-neutral policy owner to the dashboard.
// A nil controller is reported as unavailable rather than as a fabricated
// default, so the UI never implies that MLX/OMLX/SNE is live by configuration
// alone.
type EngineSelection interface {
	Snapshot() engine.SelectionSnapshot
	Select(engine.RoutePolicy) (engine.SelectionSnapshot, error)
}

type atomicEngineRouteSelection interface {
	SelectRoute(engine.Kind, engine.BackendVariant) (engine.SelectionSnapshot, error)
}

// EnginePromptExecutor is implemented by the canonical Pantheon selection
// controller. It is separate from EngineSelection so read-only dashboard test
// doubles and older deployments can still expose selection without claiming a
// prompt execution path.
type EnginePromptExecutor interface {
	CompletePrompt(context.Context, engine.PromptRequest) (engine.Completion, engine.Receipt, error)
}

// EnginePromptPolicyExecutor lets a dashboard request execute against the
// exact policy snapshot captured when the request was accepted.
type EnginePromptPolicyExecutor interface {
	Policy() engine.RoutePolicy
	CompletePromptWithPolicy(context.Context, engine.PromptRequest, engine.RoutePolicy) (engine.Completion, engine.Receipt, error)
}

func (s *Server) apiEngine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	allowNexusOrigin(w, r)
	if s.cfg.EngineSelection == nil {
		writeError(w, "engine selection is not configured", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, s.cfg.EngineSelection.Snapshot())
}

func (s *Server) apiEngineSelect(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		if !prepareEngineSelectionPreflight(w, r) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST, OPTIONS")
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.EngineSelection == nil {
		writeError(w, "engine selection is not configured", http.StatusServiceUnavailable)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, "engine selection requires Content-Type: application/json", http.StatusUnsupportedMediaType)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		status := http.StatusBadRequest
		var limitErr *http.MaxBytesError
		if errors.As(err, &limitErr) {
			status = http.StatusRequestEntityTooLarge
		}
		writeError(w, "invalid engine selection: "+err.Error(), status)
		return
	}
	if err := routerboard.ValidateJSONObjectNoNullFields(body); err != nil {
		writeError(w, "invalid engine selection: "+err.Error(), http.StatusBadRequest)
		return
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		if err == nil {
			err = errors.New("request must be a JSON object")
		}
		writeError(w, "invalid engine selection: "+err.Error(), http.StatusBadRequest)
		return
	}
	allowed := map[string]struct{}{
		"preferred": {}, "preferred_variant": {}, "allow_fallback": {}, "required_capabilities": {},
	}
	for key := range object {
		if _, ok := allowed[key]; !ok {
			writeError(w, "invalid engine selection: unknown or noncanonical field "+strconv.Quote(key), http.StatusBadRequest)
			return
		}
	}
	_, hasPreferred := object["preferred"]
	_, hasVariant := object["preferred_variant"]
	_, hasFallbackPolicy := object["allow_fallback"]
	_, hasCapabilityPolicy := object["required_capabilities"]
	routeOnly := len(object) == 2 && hasPreferred && hasVariant
	fullPolicy := len(object) == 4 && hasPreferred && hasVariant && hasFallbackPolicy && hasCapabilityPolicy
	if !routeOnly && !fullPolicy {
		writeError(w, "engine selection must specify exactly a route or a complete policy", http.StatusBadRequest)
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var policy engine.RoutePolicy
	if err := decoder.Decode(&policy); err != nil {
		writeError(w, "invalid engine selection: "+err.Error(), http.StatusBadRequest)
		return
	}
	var snapshot engine.SelectionSnapshot
	if routeOnly {
		selector, ok := s.cfg.EngineSelection.(atomicEngineRouteSelection)
		if !ok {
			writeError(w, "engine selection does not support atomic route updates", http.StatusServiceUnavailable)
			return
		}
		snapshot, err = selector.SelectRoute(policy.Preferred, policy.PreferredVariant)
	} else {
		snapshot, err = s.cfg.EngineSelection.Select(policy)
	}
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	allowNexusOrigin(w, r)
	writeJSON(w, snapshot)
}

func prepareEngineSelectionPreflight(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Allow", "POST, OPTIONS")
	requestedMethod := strings.TrimSpace(r.Header.Get("Access-Control-Request-Method"))
	if requestedMethod != "" && requestedMethod != http.MethodPost {
		writeError(w, "engine selection preflight only allows POST", http.StatusMethodNotAllowed)
		return false
	}
	for _, requestedHeader := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
		switch strings.ToLower(strings.TrimSpace(requestedHeader)) {
		case "":
		case "authorization", "content-type":
		default:
			writeError(w, "engine selection preflight contains a disallowed header", http.StatusForbidden)
			return false
		}
	}
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	return true
}
