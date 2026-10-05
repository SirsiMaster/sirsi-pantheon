package dashboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerboard"
)

// apiControl serves the authenticated canonical worker-control envelope to a
// client dashboard. A missing source is distinct from an unavailable source:
// only the former permits the UI to use the local FleetSnapshot contract.
func (s *Server) apiControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, "canonical worker snapshot is read-only", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if s.cfg.ControlSnapshotFn == nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "canonical control source is not configured"})
		return
	}
	body, err := s.cfg.ControlSnapshotFn(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "canonical worker snapshot unavailable"})
		return
	}
	if len(body) > 8<<20 {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "canonical worker snapshot exceeds 8388608-byte limit"})
		return
	}
	if _, err := routerboard.DecodeControlEnvelope(body); err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "canonical worker snapshot rejected"})
		return
	}
	_, _ = w.Write(body)
}

// apiControlAction is the loopback capability boundary for browser-originated
// worker actions. Validation and the receipt are request-bound; only the
// configured producer can reach M5, so this process cannot mutate a local
// replica when acting as a constrained client.
func (s *Server) apiControlAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.sneAccess == nil {
		writeError(w, "worker actions require a configured local capability", http.StatusServiceUnavailable)
		return
	}
	if s.cfg.ControlActionFn == nil {
		writeError(w, "canonical worker action source is not configured", http.StatusServiceUnavailable)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, "worker action requires Content-Type: application/json", http.StatusUnsupportedMediaType)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, routerboard.ControlActionBodyLimit))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, fmt.Sprintf("worker action request exceeds the %d-byte limit", routerboard.ControlActionBodyLimit), http.StatusRequestEntityTooLarge)
			return
		}
		writeError(w, "worker action request could not be read", http.StatusBadRequest)
		return
	}
	request, err := routerboard.DecodeControlActionRequest(body)
	if err != nil {
		writeError(w, "worker action request rejected: "+err.Error(), http.StatusBadRequest)
		return
	}
	result, err := s.cfg.ControlActionFn(r.Context(), body)
	if err != nil {
		var rejection *routerboard.ControlActionFailureError
		if errors.As(err, &rejection) && rejection != nil {
			if verifyErr := rejection.Failure.VerifyControlActionFailure(body); verifyErr == nil && rejection.Failure.Verb == request.Verb {
				failureBody, marshalErr := json.Marshal(rejection.Failure)
				if marshalErr == nil && len(failureBody) <= routerboard.ControlActionBodyLimit {
					w.WriteHeader(http.StatusUnprocessableEntity)
					_, _ = w.Write(failureBody)
					return
				}
			}
		}
		writeError(w, "canonical worker action failed", http.StatusBadGateway)
		return
	}
	if len(result) > routerboard.ControlActionBodyLimit {
		writeError(w, "canonical worker action receipt exceeds the response limit", http.StatusBadGateway)
		return
	}
	if err := validateControlActionProxyReceipt(body, result); err != nil {
		writeError(w, "canonical worker action receipt rejected", http.StatusBadGateway)
		return
	}
	_, _ = w.Write(result)
}

func validateControlActionProxyReceipt(requestBody []byte, responseBody []byte) error {
	if err := routerboard.ValidateJSONNoDuplicateKeys(responseBody); err != nil {
		return fmt.Errorf("ambiguous receipt JSON: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	decoder.DisallowUnknownFields()
	var response routerboard.ControlActionResponse
	if err := decoder.Decode(&response); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("receipt contains multiple JSON values")
		}
		return fmt.Errorf("receipt has trailing JSON: %w", err)
	}
	return response.VerifyControlActionResponseForRequest(requestBody)
}
