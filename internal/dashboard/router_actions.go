package dashboard

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
)

type RouterActionRequest struct {
	Action    string `json:"action"`
	ItemID    string `json:"item_id"`
	Expected  string `json:"expected"`
	Recipient string `json:"recipient,omitempty"`
	Message   string `json:"message"`
}
type RouterActionResult struct {
	ItemID     string `json:"item_id"`
	Action     string `json:"action"`
	ResponseID string `json:"response_id,omitempty"`
	Recipient  string `json:"recipient,omitempty"`
	Status     string `json:"status"`
}
type RouterActionProcessor func(context.Context, RouterActionRequest) (RouterActionResult, error)

func ownerToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// Owner writes are loopback/same-origin only. The read-only Nexus CORS allowlist
// cannot mint an owner token or mutate the queue. Host checking also prevents
// DNS rebinding. Local account processes remain within the existing OS trust.
func (s *Server) ownerOrigin(r *http.Request, write bool) bool {
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil || port != strconv.Itoa(s.cfg.Port) {
		return false
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return false
	}
	remote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !net.ParseIP(remote).IsLoopback() {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin != "" && origin != "http://"+r.Host {
		return false
	}
	if write && origin == "" {
		return false
	}
	site := r.Header.Get("Sec-Fetch-Site")
	if site != "" && site != "same-origin" {
		return false
	}
	return write || site == "same-origin" || origin == "http://"+r.Host
}
func routerJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func (s *Server) apiRouterOwnerSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		routerJSON(w, 405, map[string]string{"error": "GET required"})
		return
	}
	if !s.ownerOrigin(r, false) {
		routerJSON(w, 403, map[string]string{"error": "Owner console requires a same-origin loopback browser"})
		return
	}
	if s.cfg.RouterActionFn == nil || s.ownerCSRF == "" {
		routerJSON(w, 503, map[string]string{"error": "Owner actions are not configured"})
		return
	}
	routerJSON(w, 200, map[string]string{"actor": "owner", "token": s.ownerCSRF})
}
func (s *Server) apiRouterOwnerAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		routerJSON(w, 405, map[string]string{"error": "POST required"})
		return
	}
	if !s.ownerOrigin(r, true) || s.ownerCSRF == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Sirsi-Owner-Token")), []byte(s.ownerCSRF)) != 1 {
		routerJSON(w, 403, map[string]string{"error": "Owner session expired or origin refused; reload this console"})
		return
	}
	if s.cfg.RouterActionFn == nil {
		routerJSON(w, 503, map[string]string{"error": "Owner actions are not configured"})
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		routerJSON(w, 415, map[string]string{"error": "JSON required"})
		return
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	dec.DisallowUnknownFields()
	var req RouterActionRequest
	if dec.Decode(&req) != nil || dec.Decode(new(any)) != io.EOF {
		routerJSON(w, 400, map[string]string{"error": "Invalid or oversized action request"})
		return
	}
	if len(req.ItemID) > 512 || strings.TrimSpace(req.ItemID) == "" || len(req.Expected) != 64 || len(req.Message) > 32768 || strings.TrimSpace(req.Message) == "" || len(req.Recipient) > 128 || (req.Action != "reply" && req.Action != "assign" && req.Action != "close") {
		routerJSON(w, 400, map[string]string{"error": "Select an item and provide a valid action and message/disposition"})
		return
	}
	s.ownerActionMu.Lock()
	defer s.ownerActionMu.Unlock()
	result, err := s.cfg.RouterActionFn(r.Context(), req)
	if err != nil {
		routerJSON(w, 409, map[string]string{"error": err.Error()})
		return
	}
	routerJSON(w, 200, result)
}
