package dashboard

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
)

// FabricProducer provides the single cross-surface FabricBoard contract.
type FabricProducer func() (ledger.FabricBoard, error)

// apiFabric serves the canonical work/message/lane payload. It fails honestly
// when the producer is unavailable so consumers can retain a labeled last-good
// payload instead of rendering fabricated zeroes.
func (s *Server) apiFabric(w http.ResponseWriter, r *http.Request) {
	allowNexusOrigin(w, r)
	w.Header().Set("Cache-Control", "no-store")
	if !fabricReadMethod(w, r) {
		return
	}
	offset, snapshot, selected, err := fabricTaskSelection(r.URL.RawQuery)
	if err != nil {
		writeError(w, "invalid fabric task page selection", http.StatusBadRequest)
		return
	}
	if s.cfg.FabricSource != nil {
		var generation *ledger.FabricGeneration
		if selected {
			generation, err = s.cfg.FabricSource.ReadPage(r.Context(), offset, snapshot)
		} else {
			generation, err = s.cfg.FabricSource.Read(r.Context())
		}
		if err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, ledger.ErrFabricPageRejected) {
				status = http.StatusBadRequest
			}
			if errors.Is(err, ledger.ErrFabricTasksChanged) {
				status = http.StatusConflict
			}
			writeError(w, "fabric production or page selection unavailable; refresh the task list", status)
			return
		}
		view, err := generation.View()
		if err != nil {
			writeError(w, "fabric generation unavailable", http.StatusBadGateway)
			return
		}
		board := view.Board
		if selected {
			page, err := ledger.FabricTasksFrom(view.Read.Snapshot, offset, snapshot)
			if err != nil {
				status := http.StatusBadRequest
				if errors.Is(err, ledger.ErrFabricTasksChanged) {
					status = http.StatusConflict
				}
				writeError(w, "fabric task page changed or unavailable; refresh the task list", status)
				return
			}
			board.Tasks = page
		}
		w.Header().Set("X-Sirsi-Fabric-Generation", view.ID)
		writeJSON(w, board)
		return
	}
	if s.cfg.FabricFn == nil {
		writeError(w, "fabric not available (producer not wired)", http.StatusServiceUnavailable)
		return
	}
	if selected {
		writeError(w, "fabric task continuation unavailable from this producer", http.StatusServiceUnavailable)
		return
	}
	board, err := s.cfg.FabricFn()
	if err != nil {
		writeError(w, "fabric production failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, board)
}

func fabricTaskSelection(raw string) (int, string, bool, error) {
	if len(raw) > 256 {
		return 0, "", false, errors.New("invalid query")
	}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return 0, "", false, errors.New("invalid query")
	}
	if len(q) == 0 {
		return 0, "", false, nil
	}
	if len(q) != 2 || len(q["tasks_offset"]) != 1 || len(q["tasks_snapshot"]) != 1 || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(q.Get("tasks_snapshot")) {
		return 0, "", false, errors.New("invalid task continuation")
	}
	offset, err := strconv.Atoi(q.Get("tasks_offset"))
	if err != nil || offset <= 0 || strconv.Itoa(offset) != q.Get("tasks_offset") || offset%ledger.FabricTaskPageSize != 0 {
		return 0, "", false, errors.New("invalid task offset")
	}
	return offset, q.Get("tasks_snapshot"), true, nil
}
