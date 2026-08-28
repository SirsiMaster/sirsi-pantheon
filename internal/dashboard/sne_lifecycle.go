package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

type sneSupervisor interface {
	Start(context.Context) error
	WaitReady(context.Context) error
	Stop(context.Context) error
	ResourceAdmission() sne.ResourceAdmission
}

const sneMetalSessionLockedCode = "metal_session_locked"

func classifySNELifecycleFailure(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "console is locked") ||
		(strings.Contains(message, "metal session") && strings.Contains(message, "locked")) {
		return sneMetalSessionLockedCode, "Unlock this Mac. Pantheon will retry the same verified model and runtime when the graphical session becomes active."
	}
	return "", ""
}

type SNELifecycleConfig struct {
	CatalogStoreRoot               string
	RuntimeCatalog                 string
	RuntimeCatalogSignature        string
	RuntimeCatalogPublicKey        string
	RuntimeCatalogFeedURL          string
	RuntimeCatalogFeedSignatureURL string
	RequireSignedCatalog           bool
	AdmissionRegistry              string
	StoreRoot                      string
	RecoveryBinary                 string
	RequireRecovery                bool
	PackagesRoot                   string
	ProfilePath                    string
	factory                        func(sne.SupervisorProfile, sne.LaunchConfig) (sneSupervisor, error)
	admit                          func(string, string, string, bool, sne.SupervisorProfile) (sne.ModelAdmission, error)
	resolve                        func(string, string, bool) (sne.SupervisorProfile, sne.LaunchConfig, error)
	httpClient                     *http.Client
	recover                        sneStoreRecoveryFunc
	graphicalSessionReady          func(context.Context) (bool, error)
	unlockRetryInterval            time.Duration
}

type SNELifecycleRequest struct {
	ModelID       string `json:"model_id"`
	RuntimeID     string `json:"runtime_id,omitempty"`
	AllowResearch bool   `json:"allow_research,omitempty"`
}

type SNELifecycleState struct {
	ID                  string                 `json:"id,omitempty"`
	ModelID             string                 `json:"model_id,omitempty"`
	RuntimeID           string                 `json:"runtime_id,omitempty"`
	RuntimeSHA256       string                 `json:"runtime_sha256,omitempty"`
	NativeRuntimeSHA256 string                 `json:"native_runtime_sha256,omitempty"`
	ModelManifestSHA256 string                 `json:"model_manifest_sha256,omitempty"`
	Profile             string                 `json:"profile,omitempty"`
	State               string                 `json:"state"`
	Error               string                 `json:"error,omitempty"`
	ErrorCode           string                 `json:"error_code,omitempty"`
	Recovery            string                 `json:"recovery,omitempty"`
	ResourceAdmission   *sne.ResourceAdmission `json:"resource_admission,omitempty"`
	StartedAt           *time.Time             `json:"started_at,omitempty"`
	FinishedAt          *time.Time             `json:"finished_at,omitempty"`
	allowResearch       bool
}

type SNELifecycleManager struct {
	cfg             SNELifecycleConfig
	mu              sync.RWMutex
	state           SNELifecycleState
	supervisor      sneSupervisor
	catalogMutating bool
	recoveryErr     error
	unlockCancel    context.CancelFunc
}

type SNERuntimeCatalogStatus struct {
	State                string   `json:"state"`
	SignedRequired       bool     `json:"signed_required"`
	CatalogID            string   `json:"catalog_id,omitempty"`
	VersionSHA256        string   `json:"version_sha256,omitempty"`
	Entries              int      `json:"entries"`
	Versions             int      `json:"versions"`
	RetainedVersions     []string `json:"retained_versions,omitempty"`
	RollbackAvailable    bool     `json:"rollback_available"`
	UpdateFeedConfigured bool     `json:"update_feed_configured"`
	Error                string   `json:"error,omitempty"`
}

type SNERuntimeCapabilities struct {
	CacheTopology         string
	ServingCacheCapacity  int
	PrefixSessionsMaximum int
	StreamingMode         string
}

type sneCheckoutReceipt struct {
	Schema            string `json:"schema"`
	ModelID           string `json:"model_id"`
	Revision          string `json:"revision"`
	LicenseIdentifier string `json:"license_identifier"`
	LicenseURL        string `json:"license_url"`
	LicenseAccepted   bool   `json:"license_accepted"`
	SourceURI         string `json:"source_uri"`
	CheckpointSHA256  string `json:"checkpoint_sha256"`
	ArtifactSetSHA256 string `json:"artifact_set_sha256"`
	CompletedAt       string `json:"completed_at"`
}

func DefaultSNELifecycleConfig() *SNELifecycleConfig {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	root := filepath.Join(home, "Library", "Application Support", "Sirsi")
	catalogBundle := filepath.Join(root, "Pantheon", runtimeCatalogCurrentLinkName)
	return &SNELifecycleConfig{
		CatalogStoreRoot:               filepath.Join(root, "Pantheon"),
		RuntimeCatalog:                 filepath.Join(catalogBundle, "runtime-packages.json"),
		RuntimeCatalogSignature:        filepath.Join(catalogBundle, "runtime-packages.json.sig"),
		RuntimeCatalogPublicKey:        filepath.Join(root, "Pantheon", "runtime-catalog-ed25519.pub"),
		RuntimeCatalogFeedURL:          strings.TrimSpace(os.Getenv("SIRSI_SNE_CATALOG_FEED_URL")),
		RuntimeCatalogFeedSignatureURL: strings.TrimSpace(os.Getenv("SIRSI_SNE_CATALOG_FEED_SIGNATURE_URL")),
		RequireSignedCatalog:           true,
		AdmissionRegistry:              filepath.Join(root, "Pantheon", "model-admission.json"),
		StoreRoot:                      filepath.Join(root, "SNE", "model-store"),
		RecoveryBinary:                 filepath.Join(root, "SNE", "bin", "sne-model-store-recover"),
		RequireRecovery:                true,
		PackagesRoot:                   filepath.Join(root, "SNE", "packages"),
		ProfilePath:                    filepath.Join(root, "Pantheon", "sne-profile.yaml"),
	}
}

const runtimeCatalogCurrentLinkName = "runtime-catalog-current"

func NewSNELifecycleManager(cfg SNELifecycleConfig) *SNELifecycleManager {
	if cfg.factory == nil {
		cfg.factory = func(profile sne.SupervisorProfile, launch sne.LaunchConfig) (sneSupervisor, error) {
			return sne.NewSupervisor(profile, launch)
		}
	}
	if cfg.admit == nil {
		cfg.admit = sne.AdmitModel
	}
	if cfg.graphicalSessionReady == nil {
		cfg.graphicalSessionReady = sneGraphicalSessionReady
	}
	if cfg.unlockRetryInterval <= 0 {
		cfg.unlockRetryInterval = 5 * time.Second
	}
	manager := &SNELifecycleManager{cfg: cfg, state: SNELifecycleState{State: "stopped"}}
	if cfg.RequireRecovery {
		recoverStore := cfg.recover
		if recoverStore == nil {
			recoverStore = runSNEStoreRecovery
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		manager.recoveryErr = recoverStore(ctx, cfg.RecoveryBinary, cfg.StoreRoot)
		cancel()
		if manager.recoveryErr != nil {
			manager.state.State = "failed"
			manager.state.Error = manager.recoveryErr.Error()
		}
	}
	return manager
}

func sneGraphicalSessionReady(ctx context.Context) (bool, error) {
	output, err := exec.CommandContext(ctx, "/usr/sbin/ioreg", "-l", "-w0", "-n", "Root", "-d", "1").Output()
	if err != nil {
		return false, fmt.Errorf("inspect macOS graphical session: %w", err)
	}
	return parseIOConsoleLocked(output)
}

func parseIOConsoleLocked(output []byte) (bool, error) {
	const marker = `"IOConsoleLocked" = `
	for _, raw := range strings.Split(string(output), "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, marker) {
			continue
		}
		switch strings.TrimSpace(strings.TrimPrefix(line, marker)) {
		case "No":
			return true, nil
		case "Yes":
			return false, nil
		default:
			return false, fmt.Errorf("macOS graphical session reported invalid IOConsoleLocked value")
		}
	}
	return false, fmt.Errorf("macOS graphical session did not report IOConsoleLocked")
}

func (manager *SNELifecycleManager) cancelUnlockRetryLocked() {
	if manager.unlockCancel != nil {
		manager.unlockCancel()
		manager.unlockCancel = nil
	}
}

func (manager *SNELifecycleManager) scheduleUnlockRetryLocked() {
	manager.cancelUnlockRetryLocked()
	request := SNELifecycleRequest{ModelID: manager.state.ModelID, RuntimeID: manager.state.RuntimeID, AllowResearch: manager.state.allowResearch}
	if request.ModelID == "" || manager.cfg.graphicalSessionReady == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	manager.unlockCancel = cancel
	interval := manager.cfg.unlockRetryInterval
	go manager.waitForGraphicalSession(ctx, request, interval)
}

func (manager *SNELifecycleManager) waitForGraphicalSession(ctx context.Context, request SNELifecycleRequest, interval time.Duration) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		ready, err := manager.cfg.graphicalSessionReady(ctx)
		if err == nil && ready {
			manager.mu.Lock()
			matches := manager.state.State == "failed" && manager.state.ErrorCode == sneMetalSessionLockedCode &&
				manager.state.ModelID == request.ModelID && manager.state.RuntimeID == request.RuntimeID
			manager.unlockCancel = nil
			manager.mu.Unlock()
			if matches {
				_, _ = manager.Start(request)
			}
			return
		}
		timer.Reset(interval)
	}
}

func (manager *SNELifecycleManager) Available() map[string]bool {
	available := map[string]bool{}
	if manager == nil {
		return available
	}
	if manager.recoveryErr != nil {
		return available
	}
	catalog, err := manager.loadRuntimeCatalog()
	if err != nil {
		return available
	}
	for _, entry := range catalog.Entries {
		if manager.packageRootAllowed(entry.PackageRoot) == nil && sne.VerifyRuntimePackageBoundary(entry) == nil {
			available[entry.ModelID] = true
		}
	}
	return available
}

func (manager *SNELifecycleManager) RuntimeSelections() map[string][]string {
	selections := map[string][]string{}
	if manager == nil {
		return selections
	}
	if manager.recoveryErr != nil {
		return selections
	}
	catalog, err := manager.loadRuntimeCatalog()
	if err != nil {
		return selections
	}
	for _, entry := range catalog.Entries {
		selections[entry.ModelID] = append(selections[entry.ModelID], entry.EffectiveRuntimeID())
	}
	return selections
}

func (manager *SNELifecycleManager) RuntimeCapabilities() map[string]SNERuntimeCapabilities {
	capabilities := map[string]SNERuntimeCapabilities{}
	if manager == nil || manager.recoveryErr != nil {
		return capabilities
	}
	catalog, err := manager.loadRuntimeCatalog()
	if err != nil {
		return capabilities
	}
	for _, entry := range catalog.Entries {
		key := entry.ModelID + "\x00" + entry.EffectiveRuntimeID()
		capabilities[key] = SNERuntimeCapabilities{CacheTopology: entry.CacheTopology, ServingCacheCapacity: entry.ServingCacheCapacity, PrefixSessionsMaximum: entry.PrefixSessionsMaximum, StreamingMode: entry.StreamingMode}
	}
	return capabilities
}

func (manager *SNELifecycleManager) Snapshot() SNELifecycleState {
	if manager == nil {
		return SNELifecycleState{State: "not-configured"}
	}
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.state
}

func (manager *SNELifecycleManager) CatalogStatus() SNERuntimeCatalogStatus {
	status := SNERuntimeCatalogStatus{State: "not-configured"}
	if manager == nil {
		return status
	}
	status.SignedRequired = manager.cfg.RequireSignedCatalog
	status.UpdateFeedConfigured = strings.TrimSpace(manager.cfg.RuntimeCatalogFeedURL) != "" && strings.TrimSpace(manager.cfg.RuntimeCatalogFeedSignatureURL) != ""
	catalog, err := manager.loadRuntimeCatalog()
	if err != nil {
		status.State, status.Error = "invalid", err.Error()
		return status
	}
	status.State, status.CatalogID, status.Entries = "verified", catalog.CatalogID, len(catalog.Entries)
	if strings.TrimSpace(manager.cfg.CatalogStoreRoot) == "" {
		return status
	}
	version, err := sne.CurrentRuntimeCatalogVersion(manager.cfg.CatalogStoreRoot)
	if err != nil {
		status.State, status.Error = "invalid", err.Error()
		return status
	}
	versions, err := sne.ListRuntimeCatalogVersions(manager.cfg.CatalogStoreRoot)
	if err != nil {
		status.State, status.Error = "invalid", err.Error()
		return status
	}
	status.VersionSHA256 = version
	status.Versions = len(versions)
	status.RetainedVersions = versions
	status.RollbackAvailable = len(versions) > 1
	return status
}

func (manager *SNELifecycleManager) beginCatalogMutation() error {
	if manager == nil || strings.TrimSpace(manager.cfg.CatalogStoreRoot) == "" {
		return fmt.Errorf("SNE runtime catalog store is not configured")
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	state := manager.state.State
	if state == "starting" || state == "ready" || state == "stopping" {
		return fmt.Errorf("stop SNE before changing runtime catalogs")
	}
	if manager.catalogMutating {
		return fmt.Errorf("SNE runtime catalog mutation is already in progress")
	}
	manager.cancelUnlockRetryLocked()
	manager.catalogMutating = true
	return nil
}

func (manager *SNELifecycleManager) endCatalogMutation() {
	manager.mu.Lock()
	manager.catalogMutating = false
	manager.mu.Unlock()
}

func (manager *SNELifecycleManager) RollbackCatalog(version string) (SNERuntimeCatalogStatus, error) {
	if err := manager.beginCatalogMutation(); err != nil {
		return manager.CatalogStatus(), err
	}
	defer manager.endCatalogMutation()
	if err := sne.RollbackSignedRuntimeCatalog(manager.cfg.CatalogStoreRoot, version, manager.cfg.RuntimeCatalogPublicKey, manager.cfg.PackagesRoot); err != nil {
		return manager.CatalogStatus(), err
	}
	return manager.CatalogStatus(), nil
}

func (manager *SNELifecycleManager) RemoveCatalogVersion(version string) (SNERuntimeCatalogStatus, error) {
	if err := manager.beginCatalogMutation(); err != nil {
		return manager.CatalogStatus(), err
	}
	defer manager.endCatalogMutation()
	if err := sne.RemoveInactiveRuntimeCatalog(manager.cfg.CatalogStoreRoot, version); err != nil {
		return manager.CatalogStatus(), err
	}
	return manager.CatalogStatus(), nil
}

func (manager *SNELifecycleManager) CheckCatalogUpdates(ctx context.Context) (sne.RuntimeCatalogFeed, error) {
	if manager == nil || strings.TrimSpace(manager.cfg.RuntimeCatalogFeedURL) == "" || strings.TrimSpace(manager.cfg.RuntimeCatalogFeedSignatureURL) == "" {
		return sne.RuntimeCatalogFeed{}, fmt.Errorf("SNE runtime catalog update feed is not configured")
	}
	return sne.FetchSignedRuntimeCatalogFeed(ctx, manager.cfg.httpClient, manager.cfg.RuntimeCatalogFeedURL, manager.cfg.RuntimeCatalogFeedSignatureURL, manager.cfg.RuntimeCatalogPublicKey)
}

func (manager *SNELifecycleManager) InstallCatalogUpdate(ctx context.Context, version string) (SNERuntimeCatalogStatus, error) {
	if err := manager.beginCatalogMutation(); err != nil {
		return manager.CatalogStatus(), err
	}
	defer manager.endCatalogMutation()
	if strings.TrimSpace(manager.cfg.RuntimeCatalogFeedURL) == "" || strings.TrimSpace(manager.cfg.RuntimeCatalogFeedSignatureURL) == "" {
		return manager.CatalogStatus(), fmt.Errorf("SNE runtime catalog update feed is not configured")
	}
	if _, err := sne.FetchAndInstallRuntimeCatalogUpdate(ctx, manager.cfg.httpClient, manager.cfg.RuntimeCatalogFeedURL, manager.cfg.RuntimeCatalogFeedSignatureURL, manager.cfg.RuntimeCatalogPublicKey, manager.cfg.CatalogStoreRoot, manager.cfg.PackagesRoot, version); err != nil {
		return manager.CatalogStatus(), err
	}
	return manager.CatalogStatus(), nil
}

func (manager *SNELifecycleManager) Start(request SNELifecycleRequest) (SNELifecycleState, error) {
	if manager == nil || strings.TrimSpace(request.ModelID) == "" {
		return SNELifecycleState{}, fmt.Errorf("SNE lifecycle and model ID are required")
	}
	if manager.recoveryErr != nil {
		return manager.Snapshot(), fmt.Errorf("SNE model store is not recovered: %w", manager.recoveryErr)
	}
	manager.mu.Lock()
	if manager.catalogMutating {
		manager.mu.Unlock()
		return SNELifecycleState{}, fmt.Errorf("SNE runtime catalog mutation is in progress")
	}
	if manager.state.State == "starting" || manager.state.State == "ready" || manager.state.State == "stopping" {
		manager.mu.Unlock()
		return SNELifecycleState{}, fmt.Errorf("SNE lifecycle is already %s", manager.state.State)
	}
	manager.cancelUnlockRetryLocked()
	now := time.Now().UTC()
	state := SNELifecycleState{ID: fmt.Sprintf("sne-lifecycle-%d", now.UnixNano()), ModelID: request.ModelID, RuntimeID: request.RuntimeID, State: "starting", StartedAt: &now, allowResearch: request.AllowResearch}
	manager.state = state
	manager.mu.Unlock()
	go manager.runStart(state)
	return state, nil
}

func (manager *SNELifecycleManager) runStart(state SNELifecycleState) {
	resolve := manager.resolveLaunch
	if manager.cfg.resolve != nil {
		resolve = manager.cfg.resolve
	}
	profile, launch, err := resolve(state.ModelID, state.RuntimeID, state.allowResearch)
	if err != nil {
		manager.finish(state.ID, "failed", err)
		return
	}
	manager.setResolvedIdentity(state.ID, launch.RuntimeID, launch.RuntimeSHA256, launch.NativeRuntimeSHA256, launch.ModelManifestSHA256, profile.SNE.Profile)
	supervisor, err := manager.cfg.factory(profile, launch)
	if err != nil {
		manager.finish(state.ID, "failed", err)
		return
	}
	if err := supervisor.Start(context.Background()); err != nil {
		manager.finishAdmission(state.ID, err, supervisor.ResourceAdmission())
		return
	}
	readyContext, cancel := context.WithTimeout(context.Background(), launch.StartupTimeout)
	defer cancel()
	if err := supervisor.WaitReady(readyContext); err != nil {
		_ = supervisor.Stop(context.Background())
		manager.finish(state.ID, "failed", err)
		return
	}
	manager.mu.Lock()
	if manager.state.ID == state.ID {
		manager.supervisor = supervisor
		manager.state.State = "ready"
		manager.state.Error = ""
		manager.state.ErrorCode = ""
		manager.state.Recovery = ""
		manager.state.ResourceAdmission = nil
	}
	manager.mu.Unlock()
}

func (manager *SNELifecycleManager) setResolvedIdentity(id, runtimeID, runtimeSHA256, nativeRuntimeSHA256, modelManifestSHA256, profile string) {
	if strings.TrimSpace(runtimeID) == "" {
		return
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.state.ID == id {
		manager.state.RuntimeID = runtimeID
		manager.state.RuntimeSHA256 = runtimeSHA256
		manager.state.NativeRuntimeSHA256 = nativeRuntimeSHA256
		manager.state.ModelManifestSHA256 = modelManifestSHA256
		manager.state.Profile = profile
	}
}

func (manager *SNELifecycleManager) Stop(ctx context.Context) (SNELifecycleState, error) {
	if manager == nil {
		return SNELifecycleState{}, fmt.Errorf("SNE lifecycle is not configured")
	}
	manager.mu.Lock()
	manager.cancelUnlockRetryLocked()
	if manager.state.State == "starting" {
		manager.mu.Unlock()
		return SNELifecycleState{}, fmt.Errorf("SNE start is still in progress")
	}
	if manager.supervisor == nil {
		manager.state.State = "stopped"
		state := manager.state
		manager.mu.Unlock()
		return state, nil
	}
	manager.state.State = "stopping"
	supervisor := manager.supervisor
	manager.mu.Unlock()
	err := supervisor.Stop(ctx)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.supervisor = nil
	now := time.Now().UTC()
	manager.state.FinishedAt = &now
	if err != nil {
		manager.state.State, manager.state.Error = "failed", err.Error()
		return manager.state, err
	}
	manager.state.State, manager.state.Error = "stopped", ""
	manager.state.ErrorCode = ""
	manager.state.Recovery = ""
	manager.state.ResourceAdmission = nil
	return manager.state, nil
}

func (manager *SNELifecycleManager) finishAdmission(id string, err error, resource sne.ResourceAdmission) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.state.ID != id {
		return
	}
	now := time.Now().UTC()
	manager.state.State = "failed"
	manager.state.FinishedAt = &now
	manager.state.Error = err.Error()
	var admissionErr *sne.ResourceAdmissionError
	if errors.As(err, &admissionErr) {
		manager.state.ErrorCode = admissionErr.Code
		manager.state.Recovery = admissionErr.Recovery
		measured := resource
		manager.state.ResourceAdmission = &measured
		return
	}
	if code, recovery := classifySNELifecycleFailure(err); code != "" {
		manager.state.ErrorCode = code
		manager.state.Recovery = recovery
		manager.scheduleUnlockRetryLocked()
	}
}

func (manager *SNELifecycleManager) finish(id, state string, err error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.state.ID != id {
		return
	}
	now := time.Now().UTC()
	manager.state.State, manager.state.FinishedAt = state, &now
	if err != nil {
		manager.state.Error = err.Error()
		if code, recovery := classifySNELifecycleFailure(err); code != "" {
			manager.state.ErrorCode = code
			manager.state.Recovery = recovery
			manager.scheduleUnlockRetryLocked()
		}
	}
}

func (manager *SNELifecycleManager) resolveLaunch(modelID, runtimeID string, allowResearch bool) (sne.SupervisorProfile, sne.LaunchConfig, error) {
	profile, err := sne.LoadOrMigrateSupervisorProfile(manager.cfg.ProfilePath)
	if err != nil {
		return profile, sne.LaunchConfig{}, err
	}
	catalog, err := manager.loadRuntimeCatalog()
	if err != nil {
		return profile, sne.LaunchConfig{}, err
	}
	packaged, err := catalog.ResolveRuntime(modelID, runtimeID)
	if err != nil {
		return profile, sne.LaunchConfig{}, err
	}
	if err = manager.packageRootAllowed(packaged.PackageRoot); err != nil {
		return profile, sne.LaunchConfig{}, err
	}
	if err = sne.VerifyRuntimePackageBoundary(packaged); err != nil {
		return profile, sne.LaunchConfig{}, fmt.Errorf("SNE runtime package is not self-contained: %w", err)
	}
	manifestPath := filepath.Join(packaged.PackageRoot, "manifests", "model.json")
	registry, err := sne.LoadModelAdmissionRegistry(manager.cfg.AdmissionRegistry)
	if err != nil {
		return profile, sne.LaunchConfig{}, err
	}
	var catalogEntry string
	for _, entry := range registry.Entries {
		if entry.ModelID == modelID {
			catalogEntry = entry.CatalogEntry
			break
		}
	}
	if catalogEntry == "" {
		return profile, sne.LaunchConfig{}, fmt.Errorf("model %q is not admitted", modelID)
	}
	admitted, err := manager.cfg.admit(manager.cfg.AdmissionRegistry, catalogEntry, manifestPath, allowResearch, profile)
	if err != nil {
		return profile, sne.LaunchConfig{}, err
	}
	checkpointDir, receipt, err := findSNECheckout(manager.cfg.StoreRoot, modelID)
	if err != nil {
		return profile, sne.LaunchConfig{}, err
	}
	if !receipt.LicenseAccepted || receipt.CheckpointSHA256 != admitted.CheckpointSHA256 || receipt.ArtifactSetSHA256 != admitted.ArtifactSetSHA256 {
		return profile, sne.LaunchConfig{}, fmt.Errorf("installed SNE receipt does not match admitted tuple %q", catalogEntry)
	}
	assistantSafetensors, err := resolveAssistantSafetensors(manifestPath, checkpointDir)
	if err != nil {
		return profile, sne.LaunchConfig{}, err
	}
	executable := filepath.Join(packaged.PackageRoot, "bin", "sned")
	metallib := filepath.Join(packaged.PackageRoot, "share", "mlx.metallib")
	mlxDylib := filepath.Join(packaged.PackageRoot, "lib", "libmlx.dylib")
	jacclDylib := filepath.Join(packaged.PackageRoot, "lib", "libjaccl.dylib")
	nativeDir := filepath.Join(packaged.PackageRoot, "lib", "runtime")
	nativeRuntime := filepath.Join(nativeDir, "libsirsi_native_runtime.dylib")
	required := []string{executable, manifestPath, metallib, mlxDylib, jacclDylib, nativeRuntime, filepath.Join(checkpointDir, "tokenizer.json")}
	if assistantSafetensors != "" {
		required = append(required, assistantSafetensors)
	}
	for _, path := range required {
		if info, statErr := os.Stat(path); statErr != nil || !info.Mode().IsRegular() {
			return profile, sne.LaunchConfig{}, fmt.Errorf("required SNE launch artifact is unavailable: %s", path)
		}
	}
	identities := []struct {
		label, path, expected string
	}{
		{"runtime binary", executable, packaged.RuntimeSHA256},
		{"native runtime", nativeRuntime, packaged.NativeRuntimeSHA256},
		{"MLX dylib", mlxDylib, packaged.MLXDylibSHA256},
		{"metallib", metallib, packaged.MetallibSHA256},
		{"JACCL dylib", jacclDylib, packaged.JACCLSHA256},
	}
	for _, identity := range identities {
		digest, digestErr := sha256File(identity.path)
		if digestErr != nil || digest != identity.expected {
			return profile, sne.LaunchConfig{}, fmt.Errorf("SNE %s identity mismatch", identity.label)
		}
	}
	manifestSHA256, err := sha256File(manifestPath)
	if err != nil {
		return profile, sne.LaunchConfig{}, fmt.Errorf("hash admitted SNE model manifest: %w", err)
	}
	serviceVersion := packaged.EffectiveServiceVersion()
	if serviceVersion == "" {
		return profile, sne.LaunchConfig{}, fmt.Errorf("SNE runtime package has no canonical service version")
	}
	return profile, sne.LaunchConfig{
		Executable: executable, ModelManifest: manifestPath, CheckpointDir: checkpointDir,
		TokenizerJSON: filepath.Join(checkpointDir, "tokenizer.json"), AssistantSafetensors: assistantSafetensors,
		MetallibPath: metallib, RuntimeID: packaged.RuntimeID,
		MLXDylib: mlxDylib, NativeLibraryDir: nativeDir, NativeRuntimeDylib: nativeRuntime,
		RuntimeSHA256: packaged.RuntimeSHA256, NativeRuntimeSHA256: packaged.NativeRuntimeSHA256,
		ServiceVersion:  serviceVersion,
		ExpectedModelID: modelID, ModelManifestSHA256: manifestSHA256,
		ExpectedCacheTopology: packaged.CacheTopology, ExpectedServingCacheCapacity: packaged.ServingCacheCapacity,
		ExpectedPrefixSessionsMaximum: packaged.PrefixSessionsMaximum,
		RequiredMemoryBytes:           admitted.MemoryBytes,
		StartupTimeout:                5 * time.Minute,
	}, nil
}

func (manager *SNELifecycleManager) loadRuntimeCatalog() (sne.RuntimePackageCatalog, error) {
	var catalog sne.RuntimePackageCatalog
	var err error
	if manager.cfg.RequireSignedCatalog {
		if strings.TrimSpace(manager.cfg.RuntimeCatalogSignature) == "" || strings.TrimSpace(manager.cfg.RuntimeCatalogPublicKey) == "" {
			return sne.RuntimePackageCatalog{}, fmt.Errorf("signed SNE runtime catalog requires signature and trusted public key paths")
		}
		catalog, err = sne.LoadSignedRuntimePackageCatalog(manager.cfg.RuntimeCatalog, manager.cfg.RuntimeCatalogSignature, manager.cfg.RuntimeCatalogPublicKey)
	} else {
		catalog, err = sne.LoadRuntimePackageCatalog(manager.cfg.RuntimeCatalog)
	}
	if err != nil {
		return sne.RuntimePackageCatalog{}, err
	}
	return catalog.MaterializePackageRoots(manager.cfg.PackagesRoot)
}

func resolveAssistantSafetensors(manifestPath, checkpointDir string) (string, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", fmt.Errorf("read SNE manifest assistant identity: %w", err)
	}
	var manifest struct {
		Execution struct {
			Mode      string `json:"mode"`
			Assistant struct {
				CheckpointSHA256 string `json:"checkpoint_sha256"`
			} `json:"assistant"`
		} `json:"execution"`
		Artifacts struct {
			Integrity struct {
				Files []struct {
					Root   string `json:"root"`
					Path   string `json:"path"`
					Role   string `json:"role"`
					SHA256 string `json:"sha256"`
				} `json:"files"`
			} `json:"integrity"`
		} `json:"artifacts"`
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("decode SNE manifest assistant identity: %w", err)
	}
	var assistantPath, assistantSHA string
	for _, artifact := range manifest.Artifacts.Integrity.Files {
		if artifact.Root != "assistant" {
			continue
		}
		if assistantPath != "" || artifact.Role != "assistant" || artifact.Path == "" || filepath.IsAbs(artifact.Path) || filepath.Clean(artifact.Path) != artifact.Path || strings.HasPrefix(artifact.Path, "..") {
			return "", fmt.Errorf("SNE manifest has an invalid assistant artifact")
		}
		assistantPath, assistantSHA = artifact.Path, artifact.SHA256
	}
	if manifest.Execution.Mode != "mtp" {
		if assistantPath != "" {
			return "", fmt.Errorf("plain SNE manifest unexpectedly declares an assistant")
		}
		return "", nil
	}
	if assistantPath == "" || assistantSHA == "" || assistantSHA != manifest.Execution.Assistant.CheckpointSHA256 {
		return "", fmt.Errorf("MTP SNE manifest does not bind exactly one assistant identity")
	}
	resolved := filepath.Join(checkpointDir, assistantPath)
	digest, err := sha256File(resolved)
	if err != nil || digest != assistantSHA {
		return "", fmt.Errorf("installed MTP assistant identity mismatch")
	}
	return resolved, nil
}

func (manager *SNELifecycleManager) packageRootAllowed(root string) error {
	packagesRoot, err := filepath.Abs(manager.cfg.PackagesRoot)
	if err != nil {
		return err
	}
	packageRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(packagesRoot, packageRoot)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("SNE package root escapes the installed packages directory")
	}
	return nil
}

func findSNECheckout(root, modelID string) (string, sneCheckoutReceipt, error) {
	var checkpoint string
	var selected sneCheckoutReceipt
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != ".sne-checkout.json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var receipt sneCheckoutReceipt
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&receipt); err != nil {
			return fmt.Errorf("decode SNE checkout receipt: %w", err)
		}
		if receipt.ModelID != modelID {
			return nil
		}
		if receipt.Schema != "sne.model-checkout.v1" || checkpoint != "" {
			return fmt.Errorf("invalid or duplicate installed SNE receipt for %q", modelID)
		}
		checkpoint, selected = filepath.Dir(path), receipt
		return nil
	})
	if err != nil {
		return "", selected, err
	}
	if checkpoint == "" {
		return "", selected, fmt.Errorf("model %q is not transactionally installed", modelID)
	}
	return checkpoint, selected, nil
}

func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func (s *Server) apiSNEStart(w http.ResponseWriter, request *http.Request) {
	if !prepareSNEControlRequest(w, request) {
		return
	}
	if request.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if s.sneLifecycle == nil {
		writeError(w, "SNE lifecycle is not configured", http.StatusServiceUnavailable)
		return
	}
	var input SNELifecycleRequest
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, "invalid SNE start request", http.StatusBadRequest)
		return
	}
	state, err := s.sneLifecycle.Start(input)
	if err != nil {
		writeError(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, state)
}

func (s *Server) apiSNEStop(w http.ResponseWriter, request *http.Request) {
	if !prepareSNEControlRequest(w, request) {
		return
	}
	if request.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if s.sneLifecycle == nil {
		writeError(w, "SNE lifecycle is not configured", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 30*time.Second)
	defer cancel()
	state, err := s.sneLifecycle.Stop(ctx)
	if err != nil {
		writeError(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, state)
}

func (s *Server) apiSNELifecycle(w http.ResponseWriter, request *http.Request) {
	if !prepareSNEControlRequest(w, request) {
		return
	}
	if request.Method != http.MethodGet {
		writeError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	if s.sneLifecycle == nil {
		writeError(w, "SNE lifecycle is not configured", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, s.sneLifecycle.Snapshot())
}

type sneCatalogVersionRequest struct {
	VersionSHA256 string `json:"version_sha256"`
}

func (s *Server) apiSNECatalogRollback(w http.ResponseWriter, request *http.Request) {
	s.apiSNECatalogMutation(w, request, "rollback")
}

func (s *Server) apiSNECatalogRemove(w http.ResponseWriter, request *http.Request) {
	s.apiSNECatalogMutation(w, request, "remove")
}

type sneCatalogUpdatesResponse struct {
	FeedID   string   `json:"feed_id"`
	Current  string   `json:"current_version_sha256,omitempty"`
	Versions []string `json:"versions"`
}

func (s *Server) apiSNECatalogUpdates(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	if s.sneLifecycle == nil {
		writeError(w, "SNE lifecycle is not configured", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Minute)
	defer cancel()
	feed, err := s.sneLifecycle.CheckCatalogUpdates(ctx)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadGateway)
		return
	}
	status := s.sneLifecycle.CatalogStatus()
	response := sneCatalogUpdatesResponse{FeedID: feed.FeedID, Current: status.VersionSHA256, Versions: []string{}}
	for _, entry := range feed.Entries {
		response.Versions = append(response.Versions, entry.VersionSHA256)
	}
	writeJSON(w, response)
}

func (s *Server) apiSNECatalogInstall(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if !sameOriginRequest(request) {
		writeError(w, "cross-origin SNE catalog update rejected", http.StatusForbidden)
		return
	}
	if s.sneLifecycle == nil {
		writeError(w, "SNE lifecycle is not configured", http.StatusServiceUnavailable)
		return
	}
	var input sneCatalogVersionRequest
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.VersionSHA256) == "" {
		writeError(w, "invalid SNE catalog update request", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Minute)
	defer cancel()
	status, err := s.sneLifecycle.InstallCatalogUpdate(ctx, input.VersionSHA256)
	if err != nil {
		writeError(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, status)
}

func (s *Server) apiSNECatalogMutation(w http.ResponseWriter, request *http.Request, action string) {
	if request.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if !sameOriginRequest(request) {
		writeError(w, "cross-origin SNE catalog mutation rejected", http.StatusForbidden)
		return
	}
	if s.sneLifecycle == nil {
		writeError(w, "SNE lifecycle is not configured", http.StatusServiceUnavailable)
		return
	}
	var input sneCatalogVersionRequest
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.VersionSHA256) == "" {
		writeError(w, "invalid SNE catalog version request", http.StatusBadRequest)
		return
	}
	var status SNERuntimeCatalogStatus
	var err error
	if action == "rollback" {
		status, err = s.sneLifecycle.RollbackCatalog(input.VersionSHA256)
	} else {
		status, err = s.sneLifecycle.RemoveCatalogVersion(input.VersionSHA256)
	}
	if err != nil {
		writeError(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, status)
}
