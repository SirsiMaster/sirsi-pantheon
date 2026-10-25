package dashboard

import (
	"net/http"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
)

// FabricProducer provides the single cross-surface FabricBoard contract.
type FabricProducer func() (ledger.FabricBoard, error)

// apiFabric serves the canonical work/message/lane payload. It fails honestly
// when the producer is unavailable so consumers can retain a labeled last-good
// payload instead of rendering fabricated zeroes.
func (s *Server) apiFabric(w http.ResponseWriter, r *http.Request) {
	allowNexusOrigin(w, r)
	if s.cfg.FabricReadFn != nil {
		// Serialize read + transition observation: concurrent endpoint reads must
		// never feed an older snapshot after a newer one and invent a reversal.
		s.fabricMu.Lock()
		defer s.fabricMu.Unlock()
		read, err := s.cfg.FabricReadFn()
		if err != nil {
			writeError(w, "fabric production failed", http.StatusBadGateway)
			return
		}
		at, err := time.Parse(time.RFC3339, read.Snapshot.GeneratedAt)
		if err != nil {
			writeError(w, "fabric timestamp unavailable", http.StatusBadGateway)
			return
		}
		transitions := s.fabricFleet.Observe(read.Snapshot, at)
		read.Evidence.Activity = []ledger.FabricActivity{}
		read.Evidence.ActivitySeeded = transitions.Seeded
		for _, event := range transitions.Activity {
			read.Evidence.Activity = append(read.Evidence.Activity, ledger.FabricActivity{At: event.At, Agent: event.Agent, TaskID: event.TaskID, From: event.From, To: event.To})
		}
		writeJSON(w, ledger.FabricFrom(read.Snapshot, read.Node, s.cfg.FabricBuild, read.Evidence))
		return
	}
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
