package dashboard

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/SirsiMaster/sirsi-pantheon/internal/engine"
)

// EngineSelection exposes the engine-neutral policy owner to the dashboard.
// A nil controller is reported as unavailable rather than as a fabricated
// default, so the UI never implies that MLX/OMLX/SNE is live by configuration
// alone.
type EngineSelection interface {
	Snapshot() engine.SelectionSnapshot
	Select(engine.RoutePolicy) (engine.SelectionSnapshot, error)
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
	if r.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.EngineSelection == nil {
		writeError(w, "engine selection is not configured", http.StatusServiceUnavailable)
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	var policy engine.RoutePolicy
	if err := decoder.Decode(&policy); err != nil {
		writeError(w, "invalid engine selection: "+err.Error(), http.StatusBadRequest)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, "invalid engine selection: trailing JSON", http.StatusBadRequest)
		return
	}
	snapshot, err := s.cfg.EngineSelection.Select(policy)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	allowNexusOrigin(w, r)
	writeJSON(w, snapshot)
}
