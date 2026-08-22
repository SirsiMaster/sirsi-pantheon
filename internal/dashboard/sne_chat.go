package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	sneChatUpstream = "http://127.0.0.1:8477"
	maxSNEChatBody  = 1 << 20
)

var sneChatHTTPClient = &http.Client{Transport: &http.Transport{
	Proxy:               nil,
	DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	MaxIdleConns:        8,
	MaxIdleConnsPerHost: 8,
	IdleConnTimeout:     90 * time.Second,
}}

func (s *Server) apiSNEChat(w http.ResponseWriter, r *http.Request) {
	allowNexusOrigin(w, r)
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" && w.Header().Get("Access-Control-Allow-Origin") == "" {
		writeSNEOpenAIError(w, http.StatusForbidden, "origin_not_allowed", "origin is not allowed", nil)
		return
	}
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeSNEOpenAIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	model, err := s.sneReadModel(ctx)
	cancel()
	if err != nil {
		writeSNEOpenAIError(w, http.StatusServiceUnavailable, "sne_status_unavailable", "Pantheon could not verify the local SNE status", nil)
		return
	}
	if !model.Ready || model.ActiveModel == "" || model.Lifecycle.State != "ready" || model.Lifecycle.RuntimeID == "" {
		code := model.Lifecycle.ErrorCode
		if code == "" {
			code = "sne_not_ready"
		}
		writeSNEOpenAIError(w, http.StatusServiceUnavailable, code, "the verified SNE runtime is not ready", &model.Lifecycle)
		return
	}
	item, admitted := activeReleaseSupportedSNETuple(model)
	if !admitted {
		writeSNEOpenAIAdmissionError(w, item)
		return
	}
	proxySNEChatTo(w, r, sneChatUpstream, model.ActiveModel)
}

func proxySNEChatTo(w http.ResponseWriter, r *http.Request, upstreamBase, expectedModel string) {
	parsed, err := url.Parse(upstreamBase)
	if err != nil || parsed.Scheme != "http" || !isLoopbackHost(parsed.Hostname()) {
		writeSNEOpenAIError(w, http.StatusInternalServerError, "invalid_local_upstream", "invalid SNE upstream", nil)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSNEChatBody))
	if err != nil {
		writeSNEOpenAIError(w, http.StatusRequestEntityTooLarge, "request_too_large", "invalid or oversized chat request", nil)
		return
	}
	var contract struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if json.Unmarshal(body, &contract) != nil || strings.TrimSpace(contract.Model) == "" {
		writeSNEOpenAIError(w, http.StatusBadRequest, "model_required", "chat request requires a model", nil)
		return
	}
	if contract.Model != expectedModel {
		writeSNEOpenAIError(w, http.StatusConflict, "model_identity_mismatch", "requested model does not match Pantheon's active signed model", nil)
		return
	}

	target := strings.TrimRight(upstreamBase, "/") + "/v1/chat/completions"
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		writeSNEOpenAIError(w, http.StatusInternalServerError, "local_request_construction_failed", "could not construct the local SNE request", nil)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	if contract.Stream {
		request.Header.Set("Accept", "text/event-stream")
	} else {
		request.Header.Set("Accept", "application/json")
	}
	response, err := sneChatHTTPClient.Do(request)
	if err != nil {
		writeSNEOpenAIError(w, http.StatusBadGateway, "local_sne_unreachable", "the admitted local SNE runtime could not be reached", nil)
		return
	}
	defer response.Body.Close()

	if contentType := response.Header.Get("Content-Type"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if retryAfter := response.Header.Get("Retry-After"); retryAfter != "" {
		w.Header().Set("Retry-After", retryAfter)
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(response.StatusCode)
	flusher, _ := w.(http.Flusher)
	buffer := make([]byte, 32*1024)
	for {
		count, readErr := response.Body.Read(buffer)
		if count > 0 {
			if _, writeErr := w.Write(buffer[:count]); writeErr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			return
		}
	}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
