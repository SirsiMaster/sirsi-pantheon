package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
	"github.com/SirsiMaster/sirsi-pantheon/internal/snemodels"
)

type SNEInstallConfig struct {
	SourceCatalog         string
	AdmissionRegistry     string
	ModelCatalogRoot      string
	PreparedRoot          string
	StoreRoot             string
	CheckoutBinary        string
	RecoveryBinary        string
	RemoveBinary          string
	SupportBundleBinary   string
	SupportBundleVerifier string
	RequireRecovery       bool
	BearerTokenEnv        string
	Acquire               func(context.Context, snemodels.SourceEntry, snemodels.AcquireOptions) (snemodels.AcquireResult, error)
	recover               sneStoreRecoveryFunc
	remove                sneModelRemoveFunc
	supportBundle         sneSupportBundleFunc
}

type sneModelRemoveFunc func(context.Context, string, string, string, string) (json.RawMessage, error)
type sneSupportBundleFunc func(context.Context, string, string) ([]byte, error)

type SNEInstallRequest struct {
	CatalogEntry  string `json:"catalog_entry"`
	AcceptLicense bool   `json:"accept_license"`
	AllowResearch bool   `json:"allow_research"`
}

type SNERemoveRequest struct {
	CatalogEntry string `json:"catalog_entry"`
	ModelID      string `json:"model_id"`
}

type SNEDiscardPreparedRequest struct {
	CatalogEntry string `json:"catalog_entry"`
}

type SNEDiscardPreparedReceipt struct {
	CatalogEntry string `json:"catalog_entry"`
	Revision     string `json:"revision"`
	Removed      bool   `json:"removed"`
}

type SNEInstallJob struct {
	ID           string              `json:"id"`
	CatalogEntry string              `json:"catalog_entry"`
	ModelID      string              `json:"model_id"`
	State        string              `json:"state"`
	Progress     *snemodels.Progress `json:"progress,omitempty"`
	Checkout     json.RawMessage     `json:"checkout,omitempty"`
	Error        string              `json:"error,omitempty"`
	StartedAt    time.Time           `json:"started_at"`
	FinishedAt   *time.Time          `json:"finished_at,omitempty"`
}

type SNEInstallManager struct {
	cfg         SNEInstallConfig
	mu          sync.RWMutex
	jobs        map[string]SNEInstallJob
	recoveryErr error
}

func DefaultSNEInstallConfig() *SNEInstallConfig {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	applicationSupport := filepath.Join(home, "Library", "Application Support", "Sirsi")
	return &SNEInstallConfig{
		SourceCatalog:         filepath.Join(applicationSupport, "Pantheon", "model-sources.json"),
		AdmissionRegistry:     filepath.Join(applicationSupport, "Pantheon", "model-admission.json"),
		ModelCatalogRoot:      filepath.Join(applicationSupport, "SNE", "model-catalog", "gemma4-v1"),
		PreparedRoot:          filepath.Join(applicationSupport, "SNE", "prepared-sources"),
		StoreRoot:             filepath.Join(applicationSupport, "SNE", "model-store"),
		CheckoutBinary:        filepath.Join(applicationSupport, "SNE", "bin", "sne-model-checkout"),
		RecoveryBinary:        filepath.Join(applicationSupport, "SNE", "bin", "sne-model-store-recover"),
		RemoveBinary:          filepath.Join(applicationSupport, "SNE", "bin", "sne-model-remove"),
		SupportBundleBinary:   filepath.Join(applicationSupport, "SNE", "tools", "support-bundle.zsh"),
		SupportBundleVerifier: filepath.Join(applicationSupport, "SNE", "tools", "verify-support-bundle-privacy.zsh"),
		RequireRecovery:       true,
		BearerTokenEnv:        "HF_TOKEN",
	}
}

func (manager *SNEInstallManager) SupportBundle(ctx context.Context) ([]byte, error) {
	if manager == nil {
		return nil, fmt.Errorf("SNE package manager is not configured")
	}
	export := manager.cfg.supportBundle
	if export == nil {
		export = runSNESupportBundle
	}
	return export(ctx, manager.cfg.SupportBundleBinary, manager.cfg.SupportBundleVerifier)
}

func runSNESupportBundle(ctx context.Context, binary, verifier string) ([]byte, error) {
	if info, err := os.Stat(binary); err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return nil, fmt.Errorf("packaged SNE support exporter is unavailable")
	}
	if info, err := os.Stat(verifier); err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return nil, fmt.Errorf("packaged SNE privacy verifier is unavailable; repair or update SNE")
	}
	temp, err := os.MkdirTemp("", "pantheon-sne-support-")
	if err != nil {
		return nil, fmt.Errorf("create private support workspace")
	}
	defer os.RemoveAll(temp)
	output := filepath.Join(temp, "sirsi-sne-support.zip")
	if err := exec.CommandContext(ctx, binary, output).Run(); err != nil {
		return nil, fmt.Errorf("packaged SNE support export failed")
	}
	info, err := os.Stat(output)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 4<<20 {
		return nil, fmt.Errorf("packaged SNE support archive is invalid")
	}
	if err := exec.CommandContext(ctx, verifier, output).Run(); err != nil {
		return nil, fmt.Errorf("packaged SNE support archive failed privacy verification")
	}
	data, err := os.ReadFile(output)
	if err != nil {
		return nil, fmt.Errorf("read packaged SNE support archive")
	}
	return data, nil
}

func (manager *SNEInstallManager) Remove(ctx context.Context, request SNERemoveRequest) (json.RawMessage, error) {
	if manager == nil {
		return nil, fmt.Errorf("SNE model installer is not configured")
	}
	if manager.recoveryErr != nil {
		return nil, fmt.Errorf("SNE model store is not recovered: %w", manager.recoveryErr)
	}
	if strings.TrimSpace(request.CatalogEntry) == "" || strings.TrimSpace(request.ModelID) == "" {
		return nil, fmt.Errorf("catalog entry and model ID are required")
	}
	registry, err := sne.LoadModelAdmissionRegistry(manager.cfg.AdmissionRegistry)
	if err != nil {
		return nil, err
	}
	matched := false
	for _, entry := range registry.Entries {
		if entry.CatalogEntry == request.CatalogEntry && entry.ModelID == request.ModelID {
			matched = true
			break
		}
	}
	if !matched {
		return nil, fmt.Errorf("catalog entry and model ID do not match an admitted tuple")
	}
	remove := manager.cfg.remove
	if remove == nil {
		remove = runSNEModelRemove
	}
	return remove(ctx, manager.cfg.RemoveBinary, manager.cfg.ModelCatalogRoot, request.ModelID, manager.cfg.StoreRoot)
}

func (manager *SNEInstallManager) DiscardPrepared(request SNEDiscardPreparedRequest) (SNEDiscardPreparedReceipt, error) {
	if manager == nil {
		return SNEDiscardPreparedReceipt{}, fmt.Errorf("SNE model installer is not configured")
	}
	sources, err := snemodels.LoadSourceCatalog(manager.cfg.SourceCatalog)
	if err != nil {
		return SNEDiscardPreparedReceipt{}, err
	}
	source, err := sources.Resolve(strings.TrimSpace(request.CatalogEntry))
	if err != nil {
		return SNEDiscardPreparedReceipt{}, err
	}
	manager.mu.RLock()
	for _, job := range manager.jobs {
		if job.State == "acquiring" || job.State == "checking-out" {
			manager.mu.RUnlock()
			return SNEDiscardPreparedReceipt{}, fmt.Errorf("model installation is active; retained source cannot be discarded")
		}
	}
	manager.mu.RUnlock()
	destination := filepath.Join(manager.cfg.PreparedRoot, source.CatalogEntry, source.Revision)
	info, err := os.Lstat(destination)
	if os.IsNotExist(err) {
		return SNEDiscardPreparedReceipt{}, fmt.Errorf("retained prepared source does not exist")
	}
	if err != nil || !info.IsDir() {
		return SNEDiscardPreparedReceipt{}, fmt.Errorf("retained prepared source is invalid")
	}
	if err := removePreparedSource(manager.cfg.PreparedRoot, destination); err != nil {
		return SNEDiscardPreparedReceipt{}, err
	}
	return SNEDiscardPreparedReceipt{CatalogEntry: source.CatalogEntry, Revision: source.Revision, Removed: true}, nil
}

func runSNEModelRemove(ctx context.Context, binary, catalogRoot, modelID, storeRoot string) (json.RawMessage, error) {
	if info, err := os.Stat(binary); err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return nil, fmt.Errorf("SNE model removal helper is unavailable")
	}
	command := exec.CommandContext(ctx, binary,
		"--catalog", catalogRoot,
		"--model-id", modelID,
		"--readiness-policy", "identity",
		"--store", storeRoot,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("model removal failed: %s", strings.TrimSpace(string(output)))
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(output, &envelope); err != nil || len(envelope.Result) == 0 {
		return nil, fmt.Errorf("model removal returned an invalid result")
	}
	return envelope.Result, nil
}

func NewSNEInstallManager(cfg SNEInstallConfig) *SNEInstallManager {
	if cfg.Acquire == nil {
		cfg.Acquire = snemodels.Acquire
	}
	if cfg.BearerTokenEnv == "" {
		cfg.BearerTokenEnv = "HF_TOKEN"
	}
	manager := &SNEInstallManager{cfg: cfg, jobs: map[string]SNEInstallJob{}}
	if cfg.RequireRecovery {
		recoverStore := cfg.recover
		if recoverStore == nil {
			recoverStore = runSNEStoreRecovery
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		manager.recoveryErr = recoverStore(ctx, cfg.RecoveryBinary, cfg.StoreRoot)
		cancel()
	}
	return manager
}

func (manager *SNEInstallManager) Available() map[string]bool {
	available := map[string]bool{}
	if manager == nil {
		return available
	}
	if manager.recoveryErr != nil {
		return available
	}
	catalog, err := snemodels.LoadSourceCatalog(manager.cfg.SourceCatalog)
	if err != nil {
		return available
	}
	if info, err := os.Stat(manager.cfg.CheckoutBinary); err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return available
	}
	for _, entry := range catalog.Entries {
		available[entry.CatalogEntry] = true
	}
	return available
}

func (manager *SNEInstallManager) LifecycleToolsStatus() (bool, string) {
	if manager == nil {
		return false, "SNE lifecycle tools are not configured."
	}
	if manager.recoveryErr != nil {
		return false, "The SNE model store must recover before lifecycle tools are available."
	}
	for _, tool := range []struct {
		name string
		path string
	}{
		{name: "checkout", path: manager.cfg.CheckoutBinary},
		{name: "recovery", path: manager.cfg.RecoveryBinary},
		{name: "removal", path: manager.cfg.RemoveBinary},
	} {
		if info, err := os.Stat(tool.path); err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
			return false, "The packaged SNE " + tool.name + " tool is unavailable. Repair or reinstall SNE."
		}
	}
	return true, "Packaged checkout, recovery, and removal tools are available."
}

func (manager *SNEInstallManager) Start(request SNEInstallRequest) (SNEInstallJob, error) {
	if manager == nil {
		return SNEInstallJob{}, fmt.Errorf("SNE model installer is not configured")
	}
	if manager.recoveryErr != nil {
		return SNEInstallJob{}, fmt.Errorf("SNE model store is not recovered: %w", manager.recoveryErr)
	}
	if !request.AcceptLicense {
		return SNEInstallJob{}, fmt.Errorf("model license acceptance is required")
	}
	sources, err := snemodels.LoadSourceCatalog(manager.cfg.SourceCatalog)
	if err != nil {
		return SNEInstallJob{}, err
	}
	source, err := sources.Resolve(request.CatalogEntry)
	if err != nil {
		return SNEInstallJob{}, err
	}
	admission, err := sne.LoadModelAdmissionRegistry(manager.cfg.AdmissionRegistry)
	if err != nil {
		return SNEInstallJob{}, err
	}
	var modelID string
	for _, entry := range admission.Entries {
		if entry.CatalogEntry != request.CatalogEntry {
			continue
		}
		if entry.Qualification == "research" && !request.AllowResearch {
			return SNEInstallJob{}, fmt.Errorf("research tuple requires explicit opt-in")
		}
		modelID = entry.ModelID
		break
	}
	if modelID == "" {
		return SNEInstallJob{}, fmt.Errorf("catalog entry is not admitted by Pantheon")
	}
	if info, err := os.Stat(manager.cfg.CheckoutBinary); err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return SNEInstallJob{}, fmt.Errorf("SNE checkout helper is unavailable")
	}
	manager.mu.Lock()
	for _, existing := range manager.jobs {
		if existing.State == "acquiring" || existing.State == "checking-out" {
			manager.mu.Unlock()
			return SNEInstallJob{}, fmt.Errorf("another model installation is active")
		}
	}
	id := fmt.Sprintf("sne-install-%d", time.Now().UTC().UnixNano())
	job := SNEInstallJob{ID: id, CatalogEntry: request.CatalogEntry, ModelID: modelID, State: "acquiring", StartedAt: time.Now().UTC()}
	manager.jobs[id] = job
	manager.mu.Unlock()
	go manager.run(job, source)
	return job, nil
}

func (manager *SNEInstallManager) Job(id string) (SNEInstallJob, bool) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	job, ok := manager.jobs[id]
	return job, ok
}

func (manager *SNEInstallManager) run(job SNEInstallJob, source snemodels.SourceEntry) {
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()
	destination := filepath.Join(manager.cfg.PreparedRoot, source.CatalogEntry, source.Revision)
	result, err := manager.cfg.Acquire(ctx, source, snemodels.AcquireOptions{
		Destination: destination, BearerToken: os.Getenv(manager.cfg.BearerTokenEnv),
		Progress: func(progress snemodels.Progress) { manager.setProgress(job.ID, progress) },
	})
	if err != nil {
		manager.finish(job.ID, nil, fmt.Errorf("acquisition failed: %w", err))
		return
	}
	manager.setState(job.ID, "checking-out")
	sourceURI := fmt.Sprintf("%s://%s@%s", source.Provider, source.Repository, source.Revision)
	command := exec.CommandContext(ctx, manager.cfg.CheckoutBinary,
		"--catalog", manager.cfg.ModelCatalogRoot,
		"--model-id", job.ModelID,
		"--readiness-policy", "performance",
		"--source-dir", result.SourceDir,
		"--source-uri", sourceURI,
		"--store", manager.cfg.StoreRoot,
		"--accept-license",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		manager.finish(job.ID, nil, fmt.Errorf("checkout failed: %s", strings.TrimSpace(string(output))))
		return
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(output, &envelope); err != nil || len(envelope.Result) == 0 {
		manager.finish(job.ID, nil, fmt.Errorf("checkout returned an invalid result"))
		return
	}
	if err := removePreparedSource(manager.cfg.PreparedRoot, destination); err != nil {
		manager.finish(job.ID, nil, fmt.Errorf("checkout succeeded but prepared source cleanup failed: %w", err))
		return
	}
	manager.finish(job.ID, envelope.Result, nil)
}

func removePreparedSource(preparedRoot, destination string) error {
	root, err := filepath.Abs(preparedRoot)
	if err != nil {
		return fmt.Errorf("resolve prepared root: %w", err)
	}
	target, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve prepared source: %w", err)
	}
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("locate prepared source: %w", err)
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing cleanup outside prepared root")
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("remove verified prepared source: %w", err)
	}
	return nil
}

func (manager *SNEInstallManager) setProgress(id string, progress snemodels.Progress) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	job := manager.jobs[id]
	job.Progress = &progress
	manager.jobs[id] = job
}

func (manager *SNEInstallManager) setState(id, state string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	job := manager.jobs[id]
	job.State = state
	manager.jobs[id] = job
}

func (manager *SNEInstallManager) finish(id string, checkout json.RawMessage, err error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	job := manager.jobs[id]
	now := time.Now().UTC()
	job.FinishedAt = &now
	if err != nil {
		job.State, job.Error = "failed", err.Error()
	} else {
		job.State, job.Checkout = "installed", checkout
	}
	manager.jobs[id] = job
}

func (s *Server) apiSNEInstall(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if !sameOriginRequest(request) {
		writeError(w, "cross-origin model installation rejected", http.StatusForbidden)
		return
	}
	if s.sneJobs == nil {
		writeError(w, "SNE model installer is not configured", http.StatusServiceUnavailable)
		return
	}
	var input SNEInstallRequest
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, "invalid model installation request", http.StatusBadRequest)
		return
	}
	job, err := s.sneJobs.Start(input)
	if err != nil {
		writeError(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, job)
}

func (s *Server) apiSNEInstallStatus(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	if s.sneJobs == nil {
		writeError(w, "SNE model installer is not configured", http.StatusServiceUnavailable)
		return
	}
	job, ok := s.sneJobs.Job(request.URL.Query().Get("id"))
	if !ok {
		writeError(w, "unknown SNE installation job", http.StatusNotFound)
		return
	}
	writeJSON(w, job)
}

func (s *Server) apiSNEDiscardPrepared(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if !sameOriginRequest(request) {
		writeError(w, "cross-origin retained-source cleanup rejected", http.StatusForbidden)
		return
	}
	if s.sneJobs == nil {
		writeError(w, "SNE model installer is not configured", http.StatusServiceUnavailable)
		return
	}
	var input SNEDiscardPreparedRequest
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, "invalid retained-source cleanup request", http.StatusBadRequest)
		return
	}
	receipt, err := s.sneJobs.DiscardPrepared(input)
	if err != nil {
		writeError(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, map[string]any{"result": receipt})
}

func (s *Server) apiSNERemove(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if !sameOriginRequest(request) {
		writeError(w, "cross-origin model removal rejected", http.StatusForbidden)
		return
	}
	if s.sneJobs == nil || s.sneLifecycle == nil {
		writeError(w, "SNE model removal is not configured", http.StatusServiceUnavailable)
		return
	}
	state := s.sneLifecycle.Snapshot()
	if state.State != "stopped" {
		writeError(w, "stop SNE before removing a model", http.StatusConflict)
		return
	}
	var input SNERemoveRequest
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, "invalid model removal request", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Minute)
	defer cancel()
	result, err := s.sneJobs.Remove(ctx, input)
	if err != nil {
		writeError(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]any{"result": result})
}

func sameOriginRequest(request *http.Request) bool {
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	return origin == "http://"+request.Host || origin == "https://"+request.Host
}

// prepareSNEControlRequest admits an explicit mutation only from Pantheon's
// own origin or the narrow Nexus origin allowlist. It handles browser
// preflight without broadening model installation or license acceptance.
func prepareSNEControlRequest(w http.ResponseWriter, request *http.Request) bool {
	allowNexusOrigin(w, request)
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if origin != "" && !sameOriginRequest(request) && w.Header().Get("Access-Control-Allow-Origin") == "" {
		writeError(w, "origin is not allowed to control SNE", http.StatusForbidden)
		return false
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	if request.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	return true
}
