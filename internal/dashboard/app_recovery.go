package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/apprecovery"
)

type recoveryTargetView struct {
	TargetID         string `json:"target_id"`
	Kind             string `json:"kind"`
	RestoreSupported bool   `json:"restore_supported"`
	FreshSupported   bool   `json:"fresh_supported"`
	AutoResume       bool   `json:"auto_resume"`
	Mode             string `json:"mode,omitempty"`
	Phase            string `json:"phase,omitempty"`
	FailureCode      string `json:"failure_code,omitempty"`
}

type recoveryRequest struct {
	TargetID string `json:"target_id"`
	Mode     string `json:"mode"`
}

func (s *Server) apiRecovery(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	if s.appRecovery == nil {
		writeError(w, "application recovery is not configured", http.StatusServiceUnavailable)
		return
	}
	views := s.recoveryViews()
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"schema": "pantheon.app-recovery-read-model.v1", "targets": views})
}

func (s *Server) recoveryViews() []recoveryTargetView {
	views := make([]recoveryTargetView, 0)
	if s.appRecovery == nil {
		return views
	}
	for _, capability := range s.appRecovery.Capabilities() {
		view := recoveryTargetView{TargetID: capability.TargetID, Kind: string(capability.Kind), RestoreSupported: capability.RestoreSupported, FreshSupported: capability.FreshSupported, AutoResume: capability.AutoResume}
		if receipt, err := s.appRecovery.Latest(capability.TargetID); err == nil {
			view.Mode = string(receipt.Mode)
			view.Phase = string(receipt.Phase)
			view.FailureCode = receipt.FailureCode
		} else if !os.IsNotExist(err) {
			view.Phase = "receipt_unavailable"
		}
		views = append(views, view)
	}
	return views
}

func (s *Server) apiRecoveryRestart(w http.ResponseWriter, request *http.Request) {
	if !s.prepareRecoveryMutation(w, request) {
		return
	}
	var input recoveryRequest
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input.TargetID == "" {
		writeError(w, "invalid recovery request", http.StatusBadRequest)
		return
	}
	mode := apprecovery.Mode(input.Mode)
	if mode != apprecovery.ModeRestore && mode != apprecovery.ModeFresh {
		writeError(w, "restart mode must be restore or fresh", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Minute)
	defer cancel()
	receipt, err := s.appRecovery.Recover(ctx, input.TargetID, mode)
	view := recoveryTargetView{TargetID: receipt.TargetID, Kind: string(receipt.Kind), Mode: string(receipt.Mode), Phase: string(receipt.Phase), FailureCode: receipt.FailureCode}
	if err != nil {
		writeRecoveryJSONStatus(w, view, http.StatusConflict)
		return
	}
	view.FailureCode = ""
	writeJSON(w, view)
}

func (s *Server) apiRecoveryResume(w http.ResponseWriter, request *http.Request) {
	if !s.prepareRecoveryMutation(w, request) {
		return
	}
	var input recoveryRequest
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input.TargetID == "" || input.Mode != "" {
		writeError(w, "invalid recovery resume request", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Minute)
	defer cancel()
	receipt, err := s.appRecovery.Resume(ctx, input.TargetID)
	view := recoveryTargetView{TargetID: receipt.TargetID, Kind: string(receipt.Kind), Mode: string(receipt.Mode), Phase: string(receipt.Phase), FailureCode: receipt.FailureCode}
	if err != nil {
		writeRecoveryJSONStatus(w, view, http.StatusConflict)
		return
	}
	view.FailureCode = ""
	writeJSON(w, view)
}

func (s *Server) prepareRecoveryMutation(w http.ResponseWriter, request *http.Request) bool {
	if request.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return false
	}
	if !sameOriginRequest(request) {
		writeError(w, "cross-origin application recovery rejected", http.StatusForbidden)
		return false
	}
	if s.appRecovery == nil {
		writeError(w, "application recovery is not configured", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func writeRecoveryJSONStatus(w http.ResponseWriter, value any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
