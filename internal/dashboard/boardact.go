package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// boardActPrefix is the hosted board's ledger-edit API. It exists only when Config.BoardActFn is set
// (`sirsi router board-serve`), and every call needs a verified IAP identity; a POST also needs the
// board's own origin and the X-Sirsi-Board header, so a page on another origin cannot ride the owner's
// IAP session (CSRF).
const boardActPrefix = "/api/router/v1/act/"

// BoardActFn performs one ledger edit. actor is "board:<verified email>"; the implementation records it in
// whatever result, reason or timeline entry the edit writes. ErrBoardVerb means the verb does not exist.
type BoardActFn func(ctx context.Context, actor, verb string, body []byte) (any, error)

var ErrBoardVerb = errors.New("board: unknown action")

func (s *Server) apiBoardAct(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	email, err := s.iap.verify(r)
	if err != nil {
		writeBoardErr(w, http.StatusUnauthorized, "sign-in required")
		return
	}
	verb := strings.TrimPrefix(r.URL.Path, boardActPrefix)
	if r.Method == http.MethodGet && verb == "whoami" {
		_ = json.NewEncoder(w).Encode(map[string]string{"email": email})
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("Origin") != "https://"+r.Host || r.Header.Get("X-Sirsi-Board") != "1" ||
		!strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeBoardErr(w, http.StatusForbidden, "cross-origin edit refused")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		writeBoardErr(w, http.StatusBadRequest, "unreadable body")
		return
	}
	out, err := s.cfg.BoardActFn(r.Context(), "board:"+email, verb, body)
	switch {
	case errors.Is(err, ErrBoardVerb):
		writeBoardErr(w, http.StatusNotFound, err.Error())
	case err != nil:
		writeBoardErr(w, http.StatusUnprocessableEntity, err.Error())
	default:
		_ = json.NewEncoder(w).Encode(out)
	}
}

func writeBoardErr(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
