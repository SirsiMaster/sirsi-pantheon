package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

// PrefixCachePressureAuthorizationView is the shared non-visual read contract.
// SNE policy, execution, replay protection, and retention remain external.
type PrefixCachePressureAuthorizationView struct {
	State         string                                       `json:"state"`
	Receipt       sne.PrefixCachePressureReceipt               `json:"receipt"`
	Confirmation  *PreparedAction                              `json:"confirmation,omitempty"`
	Authorization *sne.PrefixCachePressureAuthorizationReceipt `json:"authorization,omitempty"`
}

type prefixCachePressureAuthorizeRequest struct {
	RequestID         string `json:"request_id"`
	ObservationSHA256 string `json:"observation_sha256"`
	ConfirmToken      string `json:"confirm_token,omitempty"`
	ActionHash        string `json:"action_hash,omitempty"`
}

type pendingPrefixCachePressureAuthorization struct {
	receipt   sne.PrefixCachePressureReceipt
	expiresAt time.Time
}

type prefixCachePressureAuthorizationManager struct {
	mu          sync.Mutex
	confirm     *ConfirmGuard
	now         func() time.Time
	hostID      func() (string, error)
	sample      func() sne.ResourceAdmission
	containment func() string
	pending     map[string]pendingPrefixCachePressureAuthorization
}

func newPrefixCachePressureAuthorizationManager(confirm *ConfirmGuard) *prefixCachePressureAuthorizationManager {
	return &prefixCachePressureAuthorizationManager{
		confirm:     confirm,
		now:         time.Now,
		hostID:      os.Hostname,
		sample:      sne.SampleResourceState,
		containment: func() string { return "" },
		pending:     make(map[string]pendingPrefixCachePressureAuthorization),
	}
}

func (manager *prefixCachePressureAuthorizationManager) prepare() (PrefixCachePressureAuthorizationView, error) {
	if manager == nil || manager.confirm == nil {
		return PrefixCachePressureAuthorizationView{}, fmt.Errorf("prefix-cache pressure authorization is unavailable")
	}
	if state := strings.ToLower(strings.TrimSpace(manager.containment())); state == "disabled" || state == "quarantined" {
		return PrefixCachePressureAuthorizationView{}, fmt.Errorf("SNE is %s; prefix-cache pressure action remains unavailable", state)
	}
	now := manager.now().UTC()
	hostID, err := manager.hostID()
	if err != nil {
		return PrefixCachePressureAuthorizationView{}, fmt.Errorf("resolve pressure host identity: %w", err)
	}
	receipt, err := sne.NewPrefixCachePressureReceipt(hostID, manager.sample(), now)
	if err != nil {
		return PrefixCachePressureAuthorizationView{}, err
	}
	params := map[string]string{
		"host_id": hostID, "request_id": receipt.Observation.RequestID, "observation_sha256": receipt.ObservationSHA256,
	}
	prepared, err := manager.confirm.Prepare(sne.PrefixCachePressureOperation, hostID, params,
		"Review measured prefix-cache pressure and explicitly authorize SNE to calculate its own bounded cache action.",
		[]string{receipt.Observation.RequestID}, "SNE policy and execution remain receipt-gated.")
	if err != nil {
		return PrefixCachePressureAuthorizationView{}, err
	}
	manager.mu.Lock()
	manager.pending[receipt.Observation.RequestID] = pendingPrefixCachePressureAuthorization{receipt: receipt, expiresAt: prepared.ExpiresAt}
	manager.mu.Unlock()
	return PrefixCachePressureAuthorizationView{State: "owner-confirmation-required", Receipt: receipt, Confirmation: prepared}, nil
}

func (manager *prefixCachePressureAuthorizationManager) authorize(input prefixCachePressureAuthorizeRequest) (PrefixCachePressureAuthorizationView, error) {
	if manager == nil || manager.confirm == nil {
		return PrefixCachePressureAuthorizationView{}, fmt.Errorf("prefix-cache pressure authorization is unavailable")
	}
	manager.mu.Lock()
	pending, ok := manager.pending[input.RequestID]
	manager.mu.Unlock()
	if !ok || pending.receipt.ObservationSHA256 != input.ObservationSHA256 {
		return PrefixCachePressureAuthorizationView{}, fmt.Errorf("prefix-cache pressure observation is unavailable or mismatched")
	}
	receipt := pending.receipt
	if state := strings.ToLower(strings.TrimSpace(manager.containment())); state == "disabled" || state == "quarantined" {
		return PrefixCachePressureAuthorizationView{}, fmt.Errorf("SNE is %s; prefix-cache pressure action remains unavailable", state)
	}
	now := manager.now().UTC()
	params := map[string]string{"host_id": receipt.Observation.HostID, "request_id": receipt.Observation.RequestID, "observation_sha256": receipt.ObservationSHA256}
	if err := manager.confirm.Validate(input.ConfirmToken, sne.PrefixCachePressureOperation, receipt.Observation.HostID, params, input.ActionHash); err != nil {
		return PrefixCachePressureAuthorizationView{}, err
	}
	// The authorization inherits the exact single-use confirmation deadline,
	// never a fresh TTL. The observation supplies its upper bound.
	authorization, err := sne.IssuePrefixCachePressureAuthorizationReceipt(receipt, receipt.Observation.HostID, pending.expiresAt, now)
	if err != nil {
		return PrefixCachePressureAuthorizationView{}, err
	}
	manager.mu.Lock()
	delete(manager.pending, receipt.Observation.RequestID)
	manager.mu.Unlock()
	return PrefixCachePressureAuthorizationView{State: "authorization-accepted", Receipt: receipt, Authorization: &authorization}, nil
}

func (s *Server) apiSNEPrefixCachePressure(w http.ResponseWriter, request *http.Request) {
	if !prepareSNEControlRequest(w, request) {
		return
	}
	if s.snePressure == nil {
		writeError(w, "prefix-cache pressure authorization is unavailable", http.StatusServiceUnavailable)
		return
	}
	switch request.Method {
	case http.MethodGet:
		view, err := s.snePressure.prepare()
		if err != nil {
			writeError(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, view)
	case http.MethodPost:
		var input prefixCachePressureAuthorizeRequest
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, "invalid prefix-cache pressure authorization request", http.StatusBadRequest)
			return
		}
		view, err := s.snePressure.authorize(input)
		if err != nil {
			writeError(w, err.Error(), http.StatusForbidden)
			return
		}
		writeJSON(w, view)
	default:
		writeError(w, "GET or POST required", http.StatusMethodNotAllowed)
	}
}
