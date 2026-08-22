package dashboard

import (
	"net/http"
	"strings"
)

func (s *Server) apiSNEAccessRotate(w http.ResponseWriter, request *http.Request) {
	allowNexusOrigin(w, request)
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if origin != "" && !sameOriginRequest(request) && w.Header().Get("Access-Control-Allow-Origin") == "" {
		writeSNEOpenAIError(w, http.StatusForbidden, "origin_not_allowed", "origin is not allowed", nil)
		return
	}
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	if request.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Method != http.MethodPost {
		writeSNEOpenAIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required", nil)
		return
	}
	if s.sneAccess == nil || strings.TrimSpace(s.sneAccessPath) == "" {
		writeSNEOpenAIError(w, http.StatusServiceUnavailable, "local_capability_rotation_unavailable", "local capability rotation is not configured", nil)
		return
	}
	token, err := RotateSNELocalAccessToken(s.sneAccessPath)
	if err != nil {
		writeSNEOpenAIError(w, http.StatusInternalServerError, "local_capability_rotation_failed", "local capability rotation failed closed", nil)
		return
	}
	s.sneAccess.replace(token)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]string{"access_token": token, "token_type": "Bearer"})
}
