package dashboard

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
)

// apiStats returns the current system stats snapshot as JSON.
func (s *Server) apiStats(w http.ResponseWriter, r *http.Request) {
	if s.cfg.StatsFn == nil {
		writeError(w, "stats not available", http.StatusServiceUnavailable)
		return
	}
	data, err := s.cfg.StatsFn()
	if err != nil {
		writeError(w, "stats collection failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

// apiNotifications returns notification history as JSON.
// Query params: limit (default 50), source, severity.
func (s *Server) apiNotifications(w http.ResponseWriter, r *http.Request) {
	if s.cfg.NotifyDB == nil {
		writeError(w, "notification store not available", http.StatusServiceUnavailable)
		return
	}

	limit := parseIntParam(r, "limit", 50)
	source := r.URL.Query().Get("source")
	severity := r.URL.Query().Get("severity")

	var (
		results []notify.Notification
		err     error
	)

	switch {
	case source != "":
		results, err = s.cfg.NotifyDB.BySource(source, limit)
	case severity != "":
		results, err = s.cfg.NotifyDB.BySeverity(severity, limit)
	default:
		results, err = s.cfg.NotifyDB.Recent(limit)
	}

	if err != nil {
		writeError(w, "query failed", http.StatusInternalServerError)
		return
	}

	if results == nil {
		results = []notify.Notification{}
	}
	writeJSON(w, results)
}

// apiStele returns recent Stele ledger entries as JSON.
// Query params: limit (default 100), type (event type filter).
func (s *Server) apiStele(w http.ResponseWriter, r *http.Request) {
	stelePath := s.cfg.StelePath
	if stelePath == "" {
		stelePath = stele.DefaultPath()
	}

	data, err := os.ReadFile(stelePath)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, []stele.Entry{})
			return
		}
		writeError(w, "stele read failed", http.StatusInternalServerError)
		return
	}

	typeFilter := r.URL.Query().Get("type")
	limit := parseIntParam(r, "limit", 100)

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")

	var entries []stele.Entry
	// Walk backwards (newest first) to respect limit efficiently.
	for i := len(lines) - 1; i >= 0 && len(entries) < limit; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var e stele.Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if typeFilter != "" && e.Type != typeFilter {
			continue
		}
		entries = append(entries, e)
	}

	if entries == nil {
		entries = []stele.Entry{}
	}
	writeJSON(w, entries)
}

// parseIntParam reads an integer query parameter with a default.
func parseIntParam(r *http.Request, key string, defaultVal int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultVal
	}
	return n
}
