package sne

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

type SupervisorProfile struct {
	SchemaVersion string `yaml:"schema_version"`
	Product       string `yaml:"product"`
	SNE           struct {
		Profile               string `yaml:"profile"`
		Endpoint              string `yaml:"endpoint"`
		HealthPath            string `yaml:"health_path"`
		ModelsPath            string `yaml:"models_path"`
		RestartPolicy         string `yaml:"restart_policy"`
		MemoryCeilingBytes    uint64 `yaml:"memory_ceiling_bytes"`
		MaxConcurrentRequests int    `yaml:"max_concurrent_requests"`
		MaxQueuedRequests     int    `yaml:"max_queued_requests"`
		QueueDiscipline       string `yaml:"queue_discipline"`
		RequestTimeoutMS      int64  `yaml:"request_timeout_ms"`
		YieldToForeground     bool   `yaml:"yield_to_foreground"`
	} `yaml:"sne"`
}

type LaunchConfig struct {
	Executable           string
	ModelManifest        string
	CheckpointDir        string
	TokenizerJSON        string
	AssistantSafetensors string
	MetallibPath         string
	MLXDylib             string
	NativeLibraryDir     string
	NativeRuntimeDylib   string
	// RuntimeSHA256 identifies the executing bin/sned service.
	RuntimeSHA256 string
	// NativeRuntimeSHA256 identifies the dyld-mapped inference dylib.
	NativeRuntimeSHA256           string
	RuntimeID                     string
	ServiceVersion                string
	ExpectedModelID               string
	ModelManifestSHA256           string
	ExpectedCacheTopology         string
	ExpectedServingCacheCapacity  int
	ExpectedPrefixSessionsMaximum int
	ExpectedAPIContract           string
	RequiredMemoryBytes           uint64
	Environment                   []string
	allowTestEnvironment          bool
	StartupTimeout                time.Duration
	AccessTokenFile               string
}

type Supervisor struct {
	profile SupervisorProfile
	launch  LaunchConfig
	client  *Client

	lifecycleMu sync.Mutex
	mu          sync.Mutex
	parent      context.Context
	cancel      context.CancelFunc
	cmd         *exec.Cmd
	done        chan error
	exited      chan processExit
	resource    ResourceAdmission
	generation  uint64
	admitted    uint64
	healthEvery time.Duration
	healthFails int
}

type processExit struct {
	generation uint64
	err        error
}

func LoadSupervisorProfile(path string) (SupervisorProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SupervisorProfile{}, fmt.Errorf("read SNE supervisor profile: %w", err)
	}
	var profile SupervisorProfile
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err = decoder.Decode(&profile); err != nil {
		return SupervisorProfile{}, fmt.Errorf("decode SNE supervisor profile: %w", err)
	}
	if profile.SchemaVersion != "pantheon.sne-supervisor.v0" || (profile.SNE.Profile != "interactive" && profile.SNE.Profile != "fleet") || profile.SNE.RestartPolicy != "on-failure" {
		return SupervisorProfile{}, fmt.Errorf("unsupported SNE supervisor profile")
	}
	if err = validateSupervisorServingPolicy(profile); err != nil {
		return SupervisorProfile{}, err
	}
	parsed, err := url.Parse(profile.SNE.Endpoint)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" {
		return SupervisorProfile{}, fmt.Errorf("SNE supervisor endpoint must be loopback HTTP")
	}
	return profile, nil
}

func NewSupervisor(profile SupervisorProfile, launch LaunchConfig) (*Supervisor, error) {
	if err := validateSupervisorServingPolicy(profile); err != nil {
		return nil, err
	}
	if launch.Executable == "" || launch.ModelManifest == "" || launch.CheckpointDir == "" || launch.TokenizerJSON == "" || launch.MetallibPath == "" || launch.MLXDylib == "" || launch.NativeLibraryDir == "" || launch.NativeRuntimeDylib == "" || len(launch.RuntimeSHA256) != 64 || len(launch.NativeRuntimeSHA256) != 64 || strings.TrimSpace(launch.ExpectedModelID) == "" || len(launch.ModelManifestSHA256) != 64 {
		return nil, fmt.Errorf("complete SNE launch identity is required")
	}
	if strings.TrimSpace(launch.ExpectedAPIContract) == "" {
		launch.ExpectedAPIContract = "sne.openai-chat.v2"
	}
	if len(launch.Environment) > 0 && !launch.allowTestEnvironment {
		return nil, fmt.Errorf("custom SNE child environment is prohibited")
	}
	mlxPath, err := filepath.Abs(launch.MLXDylib)
	if err != nil {
		return nil, fmt.Errorf("resolve admitted MLX dylib: %w", err)
	}
	nativeDir, err := filepath.Abs(launch.NativeLibraryDir)
	if err != nil {
		return nil, fmt.Errorf("resolve SNE native library directory: %w", err)
	}
	launch.MLXDylib = filepath.Clean(mlxPath)
	launch.NativeLibraryDir = filepath.Clean(nativeDir)
	manifestData, err := os.ReadFile(launch.ModelManifest)
	if err != nil {
		return nil, fmt.Errorf("read SNE launch manifest: %w", err)
	}
	manifestDigest := sha256.Sum256(manifestData)
	if hex.EncodeToString(manifestDigest[:]) != launch.ModelManifestSHA256 {
		return nil, fmt.Errorf("SNE launch manifest hash mismatch")
	}
	var manifest admissionManifest
	if err = json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("decode SNE launch manifest: %w", err)
	}
	if manifest.Requirements.MemoryBytes == 0 {
		return nil, fmt.Errorf("SNE launch manifest has no measured memory footprint")
	}
	if launch.RequiredMemoryBytes == 0 {
		launch.RequiredMemoryBytes = manifest.Requirements.MemoryBytes
	} else if launch.RequiredMemoryBytes != manifest.Requirements.MemoryBytes {
		return nil, fmt.Errorf("SNE launch memory footprint %d disagrees with manifest %d", launch.RequiredMemoryBytes, manifest.Requirements.MemoryBytes)
	}
	token, err := readPrivateAccessToken(launch.AccessTokenFile)
	if err != nil {
		return nil, err
	}
	client, err := NewAuthenticatedClient(profile.SNE.Endpoint, token)
	if err != nil {
		return nil, err
	}
	if launch.StartupTimeout <= 0 {
		launch.StartupTimeout = 5 * time.Minute
	}
	return &Supervisor{
		profile: profile, launch: launch, client: client,
		healthEvery: 2 * time.Second, healthFails: 3,
	}, nil
}

func validateSupervisorServingPolicy(profile SupervisorProfile) error {
	expected := expectedServingPolicy(profile.SNE.Profile)
	if profile.SNE.MaxConcurrentRequests != expected.MaxConcurrentRequests || profile.SNE.MaxQueuedRequests != expected.MaxQueuedRequests || profile.SNE.QueueDiscipline != expected.QueueDiscipline || profile.SNE.RequestTimeoutMS != expected.RequestTimeoutMS {
		return fmt.Errorf("unsupported SNE supervisor serving policy")
	}
	return nil
}

func (s *Supervisor) Start(parent context.Context) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.start(parent)
}

func (s *Supervisor) start(parent context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return fmt.Errorf("SNE supervisor is already running")
	}
	ctx, cancel := context.WithCancel(parent)
	s.parent = parent
	s.cancel = cancel
	s.done = make(chan error, 1)
	s.exited = make(chan processExit, 1)
	if err := s.startLocked(ctx, false); err != nil {
		cancel()
		s.parent = nil
		s.cancel = nil
		return err
	}
	go s.monitor(ctx)
	return nil
}

func (s *Supervisor) startLocked(ctx context.Context, lifecycleRestart bool) error {
	if s.launch.RequiredMemoryBytes > 0 {
		var resource ResourceAdmission
		var err error
		if lifecycleRestart {
			resource, err = EvaluateRestartResourceAdmission(s.launch.RequiredMemoryBytes, s.profile.SNE.YieldToForeground)
		} else {
			resource, err = EvaluateResourceAdmission(s.launch.RequiredMemoryBytes, s.profile.SNE.YieldToForeground)
		}
		s.resource = resource
		if ceilingErr := ValidateHostMemoryCeiling(s.profile.SNE.MemoryCeilingBytes, resource); ceilingErr != nil {
			return ceilingErr
		}
		if err != nil {
			return err
		}
	}
	parsed, _ := url.Parse(s.profile.SNE.Endpoint)
	args := []string{
		"--listen", parsed.Host,
		"--profile", s.profile.SNE.Profile,
		"--version", s.launch.ServiceVersion,
		"--runtime-sha256", s.launch.RuntimeSHA256,
		"--native-runtime-sha256", s.launch.NativeRuntimeSHA256,
		"--native-runtime-dylib", s.launch.NativeRuntimeDylib,
		"--model-manifest", s.launch.ModelManifest,
		"--checkpoint-dir", s.launch.CheckpointDir,
		"--tokenizer-json", s.launch.TokenizerJSON,
		"--assistant-safetensors", s.launch.AssistantSafetensors,
		"--metallib", s.launch.MetallibPath,
		"--mlx-dylib", s.launch.MLXDylib,
		"--max-concurrent-requests", strconv.Itoa(s.profile.SNE.MaxConcurrentRequests),
		"--max-queued-requests", strconv.Itoa(s.profile.SNE.MaxQueuedRequests),
		"--request-timeout", (time.Duration(s.profile.SNE.RequestTimeoutMS) * time.Millisecond).String(),
		"--access-token-file", s.launch.AccessTokenFile,
		"--parent-pid", strconv.Itoa(os.Getpid()),
	}
	cmd := exec.CommandContext(ctx, s.launch.Executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = admittedChildEnvironment(s.launch)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start SNE service: %w", err)
	}
	s.cmd = cmd
	s.generation++
	s.admitted = 0
	return nil
}

func readPrivateAccessToken(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("SNE local access token file is required")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect SNE local access token: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("SNE local access token must be a private regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read SNE local access token: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if len(token) < 32 || len(token) > 512 {
		return "", fmt.Errorf("SNE local access token must contain 32 to 512 bytes")
	}
	return token, nil
}

// ResourceAdmission returns the most recent successful per-launch caretaker
// decision. A zero value means the caller supplied no measured footprint.
func (s *Supervisor) ResourceAdmission() ResourceAdmission {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resource
}

func admittedChildEnvironment(launch LaunchConfig) []string {
	const dyldKey = "DYLD_LIBRARY_PATH="
	allowed := []string{"HOME", "USER", "LOGNAME", "TMPDIR", "LANG", "LC_ALL", "LC_CTYPE", "TZ", "__CF_USER_TEXT_ENCODING"}
	environment := make([]string, 0, len(allowed)+len(launch.Environment)+2)
	for _, key := range allowed {
		if value, ok := os.LookupEnv(key); ok {
			environment = append(environment, key+"="+value)
		}
	}
	environment = append(environment, "PATH=/usr/bin:/bin:/usr/sbin:/sbin")
	if launch.allowTestEnvironment {
		for _, entry := range launch.Environment {
			if !strings.HasPrefix(entry, dyldKey) {
				environment = append(environment, entry)
			}
		}
	}
	directories := []string{launch.NativeLibraryDir, filepath.Dir(launch.MLXDylib)}
	if directories[0] == directories[1] {
		directories = directories[:1]
	}
	return append(environment, dyldKey+strings.Join(directories, string(os.PathListSeparator)))
}

func (s *Supervisor) monitor(ctx context.Context) {
	for {
		s.mu.Lock()
		cmd := s.cmd
		generation := s.generation
		s.mu.Unlock()
		memoryStop := make(chan struct{})
		healthStop := make(chan struct{})
		if s.profile.SNE.MemoryCeilingBytes > 0 {
			go enforceMemoryCeiling(ctx, cmd.Process.Pid, s.profile.SNE.MemoryCeilingBytes, memoryStop)
		}
		go s.watchAdmittedReadiness(ctx, cmd, generation, healthStop)
		err := cmd.Wait()
		if ctx.Err() != nil {
			_ = signalSNEProcessGroup(cmd, syscall.SIGKILL)
		}
		close(memoryStop)
		close(healthStop)
		if ctx.Err() != nil {
			s.done <- nil
			return
		}
		select {
		case s.exited <- processExit{generation: generation, err: fmt.Errorf("SNE process exited before readiness: %v", err)}:
		default:
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			s.done <- nil
			return
		case <-timer.C:
		}
		s.mu.Lock()
		startErr := s.startLocked(ctx, true)
		s.mu.Unlock()
		if startErr != nil {
			s.done <- fmt.Errorf("SNE exited (%v) and restart failed: %w", err, startErr)
			return
		}
	}
}

// watchAdmittedReadiness is dormant until WaitReady admits this exact process
// generation. Consecutive readiness or identity failures terminate only that
// registered child; monitor performs the existing memory-gated replacement.
func (s *Supervisor) watchAdmittedReadiness(ctx context.Context, cmd *exec.Cmd, generation uint64, stop <-chan struct{}) {
	ticker := time.NewTicker(s.healthEvery)
	defer ticker.Stop()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
		}
		s.mu.Lock()
		admitted := s.admitted == generation && s.generation == generation && s.cmd == cmd
		s.mu.Unlock()
		if !admitted {
			failures = 0
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, s.healthEvery)
		identity, err := s.client.ReadinessIdentity(probeCtx)
		cancel()
		if err == nil {
			err = s.validateReadinessIdentity(identity)
		}
		if err == nil {
			failures = 0
			continue
		}
		failures++
		if failures < s.healthFails {
			continue
		}
		s.mu.Lock()
		current := s.admitted == generation && s.generation == generation && s.cmd == cmd
		if current {
			s.admitted = 0
		}
		s.mu.Unlock()
		if current && cmd.Process != nil {
			_ = signalSNEProcessGroup(cmd, syscall.SIGTERM)
		}
		return
	}
}

func enforceMemoryCeiling(ctx context.Context, pid int, ceiling uint64, stop <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			output, err := exec.CommandContext(ctx, "ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
			if err != nil {
				continue
			}
			kilobytes, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
			if err == nil && kilobytes*1024 > ceiling {
				_ = syscall.Kill(-pid, syscall.SIGTERM)
				return
			}
		}
	}
}

func (s *Supervisor) WaitReady(ctx context.Context) error {
	timeout, cancel := context.WithTimeout(ctx, s.launch.StartupTimeout)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var lastProbeError error
	for {
		s.mu.Lock()
		probeGeneration := s.generation
		s.mu.Unlock()
		identity, err := s.client.ReadinessIdentity(timeout)
		if err == nil {
			if err = s.validateReadinessIdentity(identity); err != nil {
				return err
			}
			if s.admitReadinessGeneration(probeGeneration) {
				return nil
			}
			err = fmt.Errorf("SNE process generation changed during readiness admission")
		}
		lastProbeError = err
		select {
		case event := <-s.exited:
			// An admitted generation can be intentionally terminated after a
			// health failure while monitor is already starting its replacement.
			// That ancestor's exit receipt must not poison readiness admission
			// for the new child. Only the generation we just probed can fail this
			// attempt; older receipts are consumed and ignored.
			s.mu.Lock()
			currentGeneration := s.generation
			s.mu.Unlock()
			if event.generation >= currentGeneration {
				return event.err
			}
			continue
		case err := <-s.done:
			return fmt.Errorf("SNE stopped before readiness: %w", err)
		case <-timeout.Done():
			return fmt.Errorf("SNE readiness timeout after last probe %v: %w", lastProbeError, timeout.Err())
		case <-ticker.C:
		}
	}
}

func (s *Supervisor) admitReadinessGeneration(generation uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel == nil || s.cmd == nil || s.generation != generation {
		return false
	}
	s.admitted = generation
	return true
}

func (s *Supervisor) validateReadinessIdentity(identity ServiceReadinessIdentity) error {
	if identity.Status != "ready" || strings.TrimSpace(identity.ServiceVersion) == "" || identity.APIVersion != "v0" || identity.APIContract != s.launch.ExpectedAPIContract {
		return fmt.Errorf("SNE readiness identity has unsupported status/version/API")
	}
	if identity.Profile != s.profile.SNE.Profile {
		return fmt.Errorf("SNE readiness profile mismatch: got %q want %q", identity.Profile, s.profile.SNE.Profile)
	}
	if identity.MaxConcurrentRequests != s.profile.SNE.MaxConcurrentRequests || identity.MaxQueuedRequests != s.profile.SNE.MaxQueuedRequests || identity.QueueDiscipline != s.profile.SNE.QueueDiscipline || identity.RequestTimeoutMS != s.profile.SNE.RequestTimeoutMS {
		return fmt.Errorf("SNE readiness serving-policy mismatch")
	}
	if identity.RuntimeSHA256 != s.launch.RuntimeSHA256 {
		return fmt.Errorf("SNE readiness service identity mismatch")
	}
	if identity.NativeRuntimeSHA256 != s.launch.NativeRuntimeSHA256 {
		return fmt.Errorf("SNE readiness native runtime identity mismatch")
	}
	if identity.LoadedModel != s.launch.ExpectedModelID {
		return fmt.Errorf("SNE readiness loaded-model mismatch: got %q want %q", identity.LoadedModel, s.launch.ExpectedModelID)
	}
	if len(identity.Models) != 1 || identity.Models[0].ID != s.launch.ExpectedModelID || identity.Models[0].ManifestSHA256 != s.launch.ModelManifestSHA256 {
		return fmt.Errorf("SNE readiness advertised-model identity mismatch")
	}
	if identity.ReadyProfile != "" || identity.ReadyRuntimeSHA256 != "" || identity.ReadyNativeRuntimeSHA256 != "" || identity.ReadyModelID != "" || identity.ReadyManifestSHA256 != "" || identity.ReadyAPIContract != "" {
		if identity.ReadyProfile != identity.Profile || identity.ReadyRuntimeSHA256 != identity.RuntimeSHA256 || identity.ReadyNativeRuntimeSHA256 != identity.NativeRuntimeSHA256 || identity.ReadyModelID != identity.LoadedModel || identity.ReadyManifestSHA256 != s.launch.ModelManifestSHA256 || identity.ReadyAPIContract != identity.APIContract || identity.ReadyMaxConcurrentRequests != identity.MaxConcurrentRequests || identity.ReadyMaxQueuedRequests != identity.MaxQueuedRequests || identity.ReadyQueueDiscipline != identity.QueueDiscipline || identity.ReadyRequestTimeoutMS != identity.RequestTimeoutMS {
			return fmt.Errorf("SNE readiness endpoint identity disagrees with status/models")
		}
	}
	if s.launch.ExpectedCacheTopology != "" && identity.CacheTopology != s.launch.ExpectedCacheTopology {
		return fmt.Errorf("SNE readiness cache-topology mismatch: got %q want %q", identity.CacheTopology, s.launch.ExpectedCacheTopology)
	}
	if s.launch.ExpectedServingCacheCapacity > 0 && identity.ServingCacheCapacity != s.launch.ExpectedServingCacheCapacity {
		return fmt.Errorf("SNE readiness serving-cache capacity mismatch: got %d want %d", identity.ServingCacheCapacity, s.launch.ExpectedServingCacheCapacity)
	}
	if s.launch.ExpectedPrefixSessionsMaximum > 0 && identity.PrefixSessionsMaximum != s.launch.ExpectedPrefixSessionsMaximum {
		return fmt.Errorf("SNE readiness prefix-session capacity mismatch: got %d want %d", identity.PrefixSessionsMaximum, s.launch.ExpectedPrefixSessionsMaximum)
	}
	return nil
}

func (s *Supervisor) Stop(ctx context.Context) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.stop(ctx, true)
}

// Wait reports a terminal supervised-lifecycle failure after readiness. A
// caller such as launchd's foreground command must exit when this fires so the
// service manager can perform a clean, independently admitted replacement.
func (s *Supervisor) Wait(ctx context.Context) error {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done == nil {
		return fmt.Errorf("SNE supervisor is not running")
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return nil
	}
}

func signalSNEProcessGroup(cmd *exec.Cmd, signal syscall.Signal) error {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, signal)
}

func (s *Supervisor) stop(ctx context.Context, clearParent bool) error {
	s.mu.Lock()
	if s.cancel == nil {
		s.mu.Unlock()
		return nil
	}
	cmd := s.cmd
	done := s.done
	if cmd != nil && cmd.Process != nil {
		_ = signalSNEProcessGroup(cmd, syscall.SIGTERM)
	}
	s.cancel()
	s.cancel = nil
	if clearParent {
		s.parent = nil
	}
	s.mu.Unlock()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if cmd != nil && cmd.Process != nil {
			_ = signalSNEProcessGroup(cmd, syscall.SIGKILL)
		}
		return ctx.Err()
	}
}

// Restart replaces the complete SNE process. It never asks MLX to close and
// reopen a model inside one address space.
func (s *Supervisor) Restart(ctx context.Context) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	parent := s.parent
	running := s.cancel != nil
	s.mu.Unlock()
	if !running || parent == nil {
		return fmt.Errorf("SNE supervisor is not running")
	}
	if err := parent.Err(); err != nil {
		return fmt.Errorf("SNE supervisor parent context is closed: %w", err)
	}
	if err := s.stop(ctx, false); err != nil {
		return fmt.Errorf("stop SNE for supervised restart: %w", err)
	}
	if err := s.startLifecycle(parent); err != nil {
		return fmt.Errorf("start SNE after supervised restart: %w", err)
	}
	if err := s.WaitReady(ctx); err != nil {
		return fmt.Errorf("wait for SNE after supervised restart: %w", err)
	}
	return nil
}

func (s *Supervisor) startLifecycle(parent context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return fmt.Errorf("SNE supervisor is already running")
	}
	ctx, cancel := context.WithCancel(parent)
	s.parent = parent
	s.cancel = cancel
	s.done = make(chan error, 1)
	s.exited = make(chan processExit, 1)
	if err := s.startLocked(ctx, true); err != nil {
		cancel()
		s.cancel = nil
		s.cmd = nil
		s.done = nil
		s.exited = nil
		return err
	}
	go s.monitor(ctx)
	return nil
}

// ReloadModel honors SNE's fail-closed lifecycle contract. A future engine may
// reload safely in-process; the current engine explicitly delegates to a fresh
// Pantheon-owned process.
func (s *Supervisor) ReloadModel(ctx context.Context, model string) error {
	err := s.client.ReloadModel(ctx, model)
	if err == nil {
		return nil
	}
	if !IsRestartRequired(err) {
		return err
	}
	return s.Restart(ctx)
}

// UnloadModel terminates the complete SNE process while preserving the parent
// context needed for a later supervised load. Native MLX is never closed and
// reopened inside one address space.
func (s *Supervisor) UnloadModel(ctx context.Context, model string) error {
	if strings.TrimSpace(model) == "" {
		return fmt.Errorf("SNE unload requires a model")
	}
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	running := s.cancel != nil
	s.mu.Unlock()
	if !running {
		return nil
	}
	err := s.client.UnloadModel(ctx, model)
	if err != nil && !IsRestartRequired(err) {
		return err
	}
	return s.stop(ctx, false)
}

// LoadModel starts a fresh admitted process after supervised unload. If the
// process is already running, load is idempotent.
func (s *Supervisor) LoadModel(ctx context.Context, model string) error {
	if strings.TrimSpace(model) == "" {
		return fmt.Errorf("SNE load requires a model")
	}
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	running := s.cancel != nil
	parent := s.parent
	s.mu.Unlock()
	if running {
		return nil
	}
	if parent == nil {
		return fmt.Errorf("SNE supervisor has no preserved parent context")
	}
	if err := parent.Err(); err != nil {
		return fmt.Errorf("SNE supervisor parent context is closed: %w", err)
	}
	if err := s.startLifecycle(parent); err != nil {
		return fmt.Errorf("start SNE for supervised load: %w", err)
	}
	if err := s.WaitReady(ctx); err != nil {
		return fmt.Errorf("wait for SNE after supervised load: %w", err)
	}
	return nil
}
