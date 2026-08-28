package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

var prefixCachePressureIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// SNEPrefixCachePressureReceiptReader is implemented by the package-owned SNE
// evidence reader. Implementations must validate regular file identity,
// symlink rejection, schema, chronology, and plan/result coherence before a
// raw receipt reaches Pantheon.
type SNEPrefixCachePressureReceiptReader interface {
	LoadPressureExecutionReceipt(context.Context, string) (json.RawMessage, error)
	LoadPressureRetentionReceipt(context.Context, string) (json.RawMessage, error)
}

const SNEPrefixCacheReceiptReadToolSHA256 = "adf1564810523f6cfd93e12dccd68c75c5e365381369e8ff5e5799033dcf6877"

type snePrefixCacheToolReader struct {
	toolPath     string
	dataRoot     string
	identityJSON string
	run          func(context.Context, string, ...string) ([]byte, error)
}

func newSNEPrefixCacheToolReader(toolPath, dataRoot, identityJSON string) SNEPrefixCachePressureReceiptReader {
	if strings.TrimSpace(toolPath) == "" || strings.TrimSpace(dataRoot) == "" || strings.TrimSpace(identityJSON) == "" || !filepath.IsAbs(dataRoot) {
		return nil
	}
	return &snePrefixCacheToolReader{toolPath: toolPath, dataRoot: filepath.Clean(dataRoot), identityJSON: identityJSON, run: func(ctx context.Context, path string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, path, args...).Output()
	}}
}

func (reader *snePrefixCacheToolReader) LoadPressureExecutionReceipt(ctx context.Context, id string) (json.RawMessage, error) {
	return reader.load(ctx, "execution", id)
}

func (reader *snePrefixCacheToolReader) LoadPressureRetentionReceipt(ctx context.Context, id string) (json.RawMessage, error) {
	return reader.load(ctx, "retention", id)
}

func (reader *snePrefixCacheToolReader) load(ctx context.Context, kind, id string) (json.RawMessage, error) {
	if reader == nil || !prefixCachePressureIDPattern.MatchString(id) || (kind != "execution" && kind != "retention") {
		return nil, fmt.Errorf("invalid SNE prefix-cache receipt reader request")
	}
	info, err := os.Lstat(reader.toolPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("SNE prefix-cache receipt reader is unavailable")
	}
	data, err := os.ReadFile(reader.toolPath)
	if err != nil {
		return nil, fmt.Errorf("read SNE prefix-cache receipt reader: %w", err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != SNEPrefixCacheReceiptReadToolSHA256 {
		return nil, fmt.Errorf("SNE prefix-cache receipt reader identity mismatch")
	}
	output, err := reader.run(ctx, reader.toolPath, "-root", reader.dataRoot, "-kind", kind, "-id", id, "-identity-json", reader.identityJSON)
	if err != nil {
		return nil, fmt.Errorf("SNE prefix-cache receipt reader rejected evidence")
	}
	return json.RawMessage(output), nil
}

type PrefixCachePressureEvidenceView struct {
	State        string          `json:"state"`
	EvidenceType string          `json:"evidence_type"`
	Identity     string          `json:"identity"`
	Receipt      json.RawMessage `json:"receipt,omitempty"`
}

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

func (s *Server) apiSNEPrefixCachePressureExecutionReceipt(w http.ResponseWriter, request *http.Request) {
	s.apiSNEPrefixCachePressureEvidence(w, request, "receipts", "execution")
}

func (s *Server) apiSNEPrefixCachePressureRetentionReceipt(w http.ResponseWriter, request *http.Request) {
	s.apiSNEPrefixCachePressureEvidence(w, request, "retention", "retention")
}

func (s *Server) apiSNEPrefixCachePressureEvidence(w http.ResponseWriter, request *http.Request, route, evidenceType string) {
	if request.Method != http.MethodGet {
		writeError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	identity := strings.TrimPrefix(request.URL.Path, "/api/sne/prefix-cache-pressure/"+route+"/")
	if identity == "" || strings.Contains(identity, "/") || !prefixCachePressureIDPattern.MatchString(identity) {
		writeError(w, "invalid prefix-cache pressure evidence identity", http.StatusBadRequest)
		return
	}
	if s.snePressureReader == nil {
		writeJSON(w, PrefixCachePressureEvidenceView{State: "unavailable", EvidenceType: evidenceType, Identity: identity})
		return
	}
	var (
		receipt json.RawMessage
		err     error
	)
	if evidenceType == "execution" {
		receipt, err = s.snePressureReader.LoadPressureExecutionReceipt(request.Context(), identity)
	} else {
		receipt, err = s.snePressureReader.LoadPressureRetentionReceipt(request.Context(), identity)
	}
	if err != nil || !validPrefixCachePressureEvidence(receipt, evidenceType, identity) {
		writeJSON(w, PrefixCachePressureEvidenceView{State: "unavailable", EvidenceType: evidenceType, Identity: identity})
		return
	}
	writeJSON(w, PrefixCachePressureEvidenceView{State: "available", EvidenceType: evidenceType, Identity: identity, Receipt: receipt})
}

func validPrefixCachePressureEvidence(raw json.RawMessage, evidenceType, identity string) bool {
	var header struct {
		Schema            string `json:"schema"`
		RequestID         string `json:"request_id"`
		CleanupID         string `json:"cleanup_id"`
		HostID            string `json:"host_id"`
		ObservationSHA256 string `json:"observation_sha256"`
		Status            string `json:"status"`
		StartedAtUnix     int64  `json:"started_at_unix"`
		FinishedAtUnix    int64  `json:"finished_at_unix"`
		CreatedAtUnix     int64  `json:"created_at_unix"`
		CutoffUnix        int64  `json:"cutoff_unix"`
	}
	if json.Unmarshal(raw, &header) != nil {
		return false
	}
	if evidenceType == "execution" {
		return header.Schema == "sne.prefix-cache.pressure-policy.v1" && header.RequestID == identity && prefixCachePressureIDPattern.MatchString(header.HostID) && len(header.ObservationSHA256) == 64 && (header.Status == "started" || header.Status == "completed" || header.Status == "failed") && header.StartedAtUnix > 0 && (header.Status == "started" || header.FinishedAtUnix >= header.StartedAtUnix)
	}
	return header.Schema == "sne.prefix-cache.pressure-retention.v1" && header.CleanupID == identity && header.CreatedAtUnix > 0 && header.CutoffUnix > 0 && header.CutoffUnix < header.CreatedAtUnix
}
