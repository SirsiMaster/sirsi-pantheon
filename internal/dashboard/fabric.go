package dashboard

import (
	"net/http"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
)

// FabricProducer provides the single cross-surface FabricBoard contract.
type FabricProducer func() (ledger.FabricBoard, error)

// apiFabric serves the canonical work/message/lane payload. It fails honestly
// when the producer is unavailable so consumers can retain a labeled last-good
// payload instead of rendering fabricated zeroes.
func (s *Server) apiFabric(w http.ResponseWriter, r *http.Request) {
	allowNexusOrigin(w, r)
	if s.cfg.FabricFn == nil {
		writeError(w, "fabric not available (producer not wired)", http.StatusServiceUnavailable)
		return
	}
	board, err := s.cfg.FabricFn()
	if err != nil {
		writeError(w, "fabric production failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, board)
}
