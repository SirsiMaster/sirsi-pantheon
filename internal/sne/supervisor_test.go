package sne

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("SNE_SUPERVISOR_HELPER_EXIT") == "1" {
		fmt.Fprintln(os.Stderr, "intentional startup rejection")
		os.Exit(42)
	}
	if os.Getenv("SNE_SUPERVISOR_HELPER") == "1" {
		listen := ""
		for index, argument := range os.Args {
			if argument == "--listen" && index+1 < len(os.Args) {
				listen = os.Args[index+1]
			}
		}
		countFile := os.Getenv("SNE_SUPERVISOR_COUNT_FILE")
		file, _ := os.OpenFile(countFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if file != nil {
			_, _ = fmt.Fprintln(file, os.Getpid())
			_ = file.Close()
		}
		if descendantFile := os.Getenv("SNE_SUPERVISOR_DESCENDANT_FILE"); descendantFile != "" {
			descendant := exec.Command("sleep", "300")
			if err := descendant.Start(); err != nil {
				os.Exit(43)
			}
			if err := os.WriteFile(descendantFile, []byte(strconv.Itoa(descendant.Process.Pid)), 0o600); err != nil {
				os.Exit(44)
			}
		}
		mux := http.NewServeMux()
		runtimeSHA := strings.Repeat("a", 64)
		nativeRuntimeSHA := strings.Repeat("d", 64)
		manifestSHA := os.Getenv("SNE_SUPERVISOR_MANIFEST_SHA")
		if manifestSHA == "" {
			manifestSHA = strings.Repeat("b", 64)
		}
		modelID := "gemma-test"
		mux.HandleFunc("/health/ready", func(w http.ResponseWriter, _ *http.Request) {
			failFile := os.Getenv("SNE_SUPERVISOR_FAIL_READY_FILE")
			if failFile != "" {
				if _, err := os.Stat(failFile); err == nil {
					starts, _ := os.ReadFile(countFile)
					fields := strings.Fields(string(starts))
					if len(fields) > 0 && fields[0] == strconv.Itoa(os.Getpid()) {
						w.WriteHeader(http.StatusServiceUnavailable)
						_, _ = w.Write([]byte(`{"error":{"code":"engine_unavailable"}}`))
						return
					}
				}
			}
			_, _ = fmt.Fprintf(w, `{"status":"ready","service_version":"test","api_version":"v0","api_contract":"sne.openai-chat.v2","profile":"interactive","runtime_sha256":%q,"native_runtime_sha256":%q,"model_id":%q,"model_manifest_sha256":%q,"max_concurrent_requests":1,"max_queued_requests":8,"queue_discipline":"fifo","request_timeout_ms":120000}`, runtimeSHA, nativeRuntimeSHA, modelID, manifestSHA)
		})
		mux.HandleFunc("/v1/sne/status", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprintf(w, `{"profile":"interactive","api_contract":"sne.openai-chat.v2","runtime_sha256":%q,"native_runtime_sha256":%q,"loaded_model":%q,"max_concurrent_requests":1,"max_queued_requests":8,"queue_discipline":"fifo","request_timeout_ms":120000}`, runtimeSHA, nativeRuntimeSHA, modelID)
		})
		mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprintf(w, `{"data":[{"id":%q,"manifest_sha256":%q}]}`, modelID, manifestSHA)
		})
		mux.HandleFunc("/v1/sne/model/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":{"code":"restart_required","message":"supervised lifecycle required","retryable":true}}`))
		})
		_ = http.ListenAndServe(listen, mux)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func bindSupervisorTestManifest(t *testing.T, launch *LaunchConfig) {
	t.Helper()
	data := []byte(`{"requirements":{"memory_bytes":1}}`)
	path := filepath.Join(t.TempDir(), "model.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	launch.ModelManifest = path
	launch.ModelManifestSHA256 = hex.EncodeToString(digest[:])
	tokenPath := filepath.Join(t.TempDir(), "sne-local-api.token")
	if err := os.WriteFile(tokenPath, []byte("abcdefghijklmnopqrstuvwxyz123456\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	launch.AccessTokenFile = tokenPath
	if launch.NativeRuntimeDylib == "" {
		launch.NativeRuntimeDylib = "/tmp/libsirsi_native_runtime.dylib"
	}
	if launch.NativeRuntimeSHA256 == "" {
		launch.NativeRuntimeSHA256 = strings.Repeat("d", 64)
	}
}

func bindSafeSupervisorResources(t *testing.T) {
	t.Helper()
	previous := sampleSNEResourcesFn
	sampleSNEResourcesFn = func() resourceSample { return safeResourceSample() }
	t.Cleanup(func() { sampleSNEResourcesFn = previous })
}

func bindSupervisorTestPolicy(profile *SupervisorProfile) {
	profile.SNE.MaxConcurrentRequests = 1
	profile.SNE.MaxQueuedRequests = 8
	profile.SNE.QueueDiscipline = "fifo"
	profile.SNE.RequestTimeoutMS = 120000
}

func TestWaitReadyReportsImmediateChildExit(t *testing.T) {
	bindSafeSupervisorResources(t)
	profile := SupervisorProfile{}
	profile.SNE.Endpoint = "http://127.0.0.1:1"
	profile.SNE.Profile = "interactive"
	bindSupervisorTestPolicy(&profile)
	launch := LaunchConfig{
		Executable: os.Args[0], ModelManifest: "/tmp/model.json",
		CheckpointDir: "/tmp/model", TokenizerJSON: "/tmp/tokenizer.json",
		MetallibPath: "/tmp/mlx.metallib", MLXDylib: "/tmp/libmlx.dylib",
		NativeLibraryDir: "/tmp", RuntimeSHA256: strings.Repeat("a", 64),
		ExpectedModelID: "gemma-test", ModelManifestSHA256: strings.Repeat("b", 64),
		StartupTimeout:       5 * time.Second,
		Environment:          []string{"SNE_SUPERVISOR_HELPER_EXIT=1"},
		allowTestEnvironment: true,
	}
	bindSupervisorTestManifest(t, &launch)
	supervisor, err := NewSupervisor(profile, launch)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err = supervisor.Start(ctx); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	err = supervisor.WaitReady(ctx)
	if err == nil || !strings.Contains(err.Error(), "exit status 42") {
		t.Fatalf("immediate child exit = %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("immediate child exit took %s to surface", time.Since(started))
	}
}

func TestSupervisorStopTerminatesCompleteProcessGroup(t *testing.T) {
	bindSafeSupervisorResources(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	profile := SupervisorProfile{}
	profile.SNE.Endpoint = "http://" + address
	profile.SNE.Profile = "interactive"
	bindSupervisorTestPolicy(&profile)
	dir := t.TempDir()
	descendantFile := filepath.Join(dir, "descendant.pid")
	launch := LaunchConfig{
		Executable: os.Args[0], ModelManifest: "/tmp/model.json", CheckpointDir: "/tmp/model", TokenizerJSON: "/tmp/tokenizer.json",
		AssistantSafetensors: "/tmp/assistant.safetensors", MetallibPath: "/tmp/mlx.metallib", MLXDylib: "/tmp/libmlx.dylib", NativeLibraryDir: "/tmp",
		RuntimeSHA256: strings.Repeat("a", 64), StartupTimeout: 5 * time.Second, ExpectedModelID: "gemma-test", ModelManifestSHA256: strings.Repeat("b", 64),
		Environment: []string{"SNE_SUPERVISOR_HELPER=1", "SNE_SUPERVISOR_COUNT_FILE=" + filepath.Join(dir, "starts.txt"), "SNE_SUPERVISOR_DESCENDANT_FILE=" + descendantFile}, allowTestEnvironment: true,
	}
	bindSupervisorTestManifest(t, &launch)
	launch.Environment = append(launch.Environment, "SNE_SUPERVISOR_MANIFEST_SHA="+launch.ModelManifestSHA256)
	supervisor, err := NewSupervisor(profile, launch)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = supervisor.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err = supervisor.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	pidBytes, err := os.ReadFile(descendantFile)
	if err != nil {
		t.Fatal(err)
	}
	descendantPID, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		t.Fatal(err)
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopCancel()
	if err := supervisor.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for syscall.Kill(descendantPID, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if err := syscall.Kill(descendantPID, 0); err != syscall.ESRCH {
		t.Fatalf("SNE descendant pid %d survived supervised stop: %v", descendantPID, err)
	}
}

func TestAdmittedChildEnvironmentReplacesInheritedDYLDPath(t *testing.T) {
	t.Setenv("DYLD_LIBRARY_PATH", "/private/tmp/stale-mlx")
	t.Setenv("SNE_ALLOW_LOCKED_SESSION", "1")
	t.Setenv("MLX_METAL_DEBUG", "1")
	t.Setenv("MTL_DEBUG_LAYER", "1")
	t.Setenv("PYTHONPATH", "/private/tmp/python")
	t.Setenv("GODEBUG", "gctrace=1")
	launch := LaunchConfig{
		MLXDylib:         filepath.Join(t.TempDir(), "package", "lib", "libmlx.dylib"),
		NativeLibraryDir: filepath.Join(t.TempDir(), "native"),
		Environment:      []string{"DYLD_LIBRARY_PATH=/private/tmp/also-stale", "SNE_TEST=1"}, allowTestEnvironment: true,
	}
	environment := admittedChildEnvironment(launch)
	want := "DYLD_LIBRARY_PATH=" + launch.NativeLibraryDir + string(filepath.ListSeparator) + filepath.Dir(launch.MLXDylib)
	var dyld []string
	for _, entry := range environment {
		if strings.HasPrefix(entry, "DYLD_LIBRARY_PATH=") {
			dyld = append(dyld, entry)
		}
	}
	if len(dyld) != 1 || dyld[0] != want {
		t.Fatalf("admitted DYLD identity = %q, want %q", dyld, want)
	}
	joined := strings.Join(environment, "\n")
	for _, forbidden := range []string{"SNE_ALLOW_LOCKED_SESSION=", "MLX_METAL_DEBUG=", "MTL_DEBUG_LAYER=", "PYTHONPATH=", "GODEBUG=", "/private/tmp/stale-mlx", "/private/tmp/also-stale"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("admitted child inherited forbidden environment %q: %s", forbidden, joined)
		}
	}
	if !strings.Contains(joined, "SNE_TEST=1") {
		t.Fatalf("test-only environment was not preserved: %s", joined)
	}
}

func TestNewSupervisorRejectsCustomProductionEnvironment(t *testing.T) {
	profile := SupervisorProfile{}
	profile.SNE.Endpoint = "http://127.0.0.1:8477"
	profile.SNE.Profile = "interactive"
	bindSupervisorTestPolicy(&profile)
	launch := LaunchConfig{
		Executable: "/package/bin/sned", ModelManifest: "/tmp/model.json",
		CheckpointDir: "/models/gemma-test", TokenizerJSON: "/models/gemma-test/tokenizer.json",
		MetallibPath: "/package/share/mlx.metallib", MLXDylib: "/package/lib/libmlx.dylib",
		NativeLibraryDir: "/package/lib", NativeRuntimeDylib: "/package/lib/libsirsi_native_runtime.dylib", RuntimeSHA256: strings.Repeat("a", 64), NativeRuntimeSHA256: strings.Repeat("d", 64),
		ExpectedModelID: "gemma-test", ModelManifestSHA256: strings.Repeat("b", 64),
		Environment: []string{"SNE_ALLOW_LOCKED_SESSION=1"},
	}
	if _, err := NewSupervisor(profile, launch); err == nil || !strings.Contains(err.Error(), "environment is prohibited") {
		t.Fatalf("custom production environment error = %v", err)
	}
}

func TestNewSupervisorRequiresMLXAndNativeLibraryIdentity(t *testing.T) {
	profile := SupervisorProfile{}
	profile.SNE.Endpoint = "http://127.0.0.1:8477"
	launch := LaunchConfig{
		Executable: "/tmp/sned", ModelManifest: "/tmp/model.json",
		CheckpointDir: "/tmp/model", TokenizerJSON: "/tmp/tokenizer.json",
		RuntimeSHA256: strings.Repeat("a", 64),
	}
	if _, err := NewSupervisor(profile, launch); err == nil {
		t.Fatal("NewSupervisor accepted launch without MLX/native-library identity")
	}
}

func TestSupervisorRejectsReadinessIdentityDrift(t *testing.T) {
	profile := SupervisorProfile{}
	profile.SNE.Endpoint = "http://127.0.0.1:8477"
	profile.SNE.Profile = "interactive"
	bindSupervisorTestPolicy(&profile)
	launch := LaunchConfig{
		Executable: "/package/bin/sned", ModelManifest: "/package/manifests/model.json",
		CheckpointDir: "/models/gemma-test", TokenizerJSON: "/models/gemma-test/tokenizer.json",
		MetallibPath: "/package/share/mlx.metallib", MLXDylib: "/package/lib/libmlx.dylib",
		NativeLibraryDir: "/package/lib/runtime", RuntimeSHA256: strings.Repeat("a", 64),
		ExpectedModelID: "gemma-test", ModelManifestSHA256: strings.Repeat("b", 64),
	}
	bindSupervisorTestManifest(t, &launch)
	supervisor, err := NewSupervisor(profile, launch)
	if err != nil {
		t.Fatal(err)
	}
	valid := ServiceReadinessIdentity{
		Status: "ready", ServiceVersion: "2.4.1", APIVersion: "v0", APIContract: "sne.openai-chat.v2", Profile: "interactive",
		RuntimeSHA256: launch.RuntimeSHA256, NativeRuntimeSHA256: launch.NativeRuntimeSHA256, LoadedModel: launch.ExpectedModelID,
		Models:                []Model{{ID: launch.ExpectedModelID, ManifestSHA256: launch.ModelManifestSHA256}},
		MaxConcurrentRequests: 1, MaxQueuedRequests: 8, QueueDiscipline: "fifo", RequestTimeoutMS: 120000,
	}
	if err := supervisor.validateReadinessIdentity(valid); err != nil {
		t.Fatalf("valid identity rejected: %v", err)
	}
	tests := map[string]func(*ServiceReadinessIdentity){
		"runtime":        func(identity *ServiceReadinessIdentity) { identity.RuntimeSHA256 = strings.Repeat("c", 64) },
		"native-runtime": func(identity *ServiceReadinessIdentity) { identity.NativeRuntimeSHA256 = strings.Repeat("c", 64) },
		"model":          func(identity *ServiceReadinessIdentity) { identity.LoadedModel = "other-model" },
		"manifest":       func(identity *ServiceReadinessIdentity) { identity.Models[0].ManifestSHA256 = strings.Repeat("d", 64) },
		"profile":        func(identity *ServiceReadinessIdentity) { identity.Profile = "fleet" },
		"api":            func(identity *ServiceReadinessIdentity) { identity.APIVersion = "v1" },
		"contract":       func(identity *ServiceReadinessIdentity) { identity.APIContract = "sne.openai-chat.v1" },
		"concurrency":    func(identity *ServiceReadinessIdentity) { identity.MaxConcurrentRequests = 4 },
		"queue":          func(identity *ServiceReadinessIdentity) { identity.MaxQueuedRequests = 32 },
		"discipline":     func(identity *ServiceReadinessIdentity) { identity.QueueDiscipline = "lifo" },
		"deadline":       func(identity *ServiceReadinessIdentity) { identity.RequestTimeoutMS = 30000 },
		"multiple": func(identity *ServiceReadinessIdentity) {
			identity.Models = append(identity.Models, identity.Models[0])
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			identity := valid
			identity.Models = append([]Model(nil), valid.Models...)
			mutate(&identity)
			if err := supervisor.validateReadinessIdentity(identity); err == nil {
				t.Fatal("drifted readiness identity was accepted")
			}
		})
	}
}

func TestSupervisorRequiresConfiguredCacheAndSessionCapacity(t *testing.T) {
	profile := SupervisorProfile{}
	profile.SNE.Endpoint = "http://127.0.0.1:8477"
	profile.SNE.Profile = "interactive"
	bindSupervisorTestPolicy(&profile)
	launch := LaunchConfig{
		Executable: "/package/bin/sned", ModelManifest: "/package/manifests/model.json",
		CheckpointDir: "/models/gemma-test", TokenizerJSON: "/models/gemma-test/tokenizer.json",
		MetallibPath: "/package/share/mlx.metallib", MLXDylib: "/package/lib/libmlx.dylib",
		NativeLibraryDir: "/package/lib/runtime", RuntimeSHA256: strings.Repeat("a", 64),
		ExpectedModelID: "gemma-test", ModelManifestSHA256: strings.Repeat("b", 64),
		ExpectedCacheTopology: "paged-ring-4096", ExpectedServingCacheCapacity: 4096, ExpectedPrefixSessionsMaximum: 2,
	}
	bindSupervisorTestManifest(t, &launch)
	supervisor, err := NewSupervisor(profile, launch)
	if err != nil {
		t.Fatal(err)
	}
	valid := ServiceReadinessIdentity{
		Status: "ready", ServiceVersion: "2.5.0", APIVersion: "v0", APIContract: "sne.openai-chat.v2", Profile: "interactive",
		RuntimeSHA256: launch.RuntimeSHA256, NativeRuntimeSHA256: launch.NativeRuntimeSHA256, LoadedModel: launch.ExpectedModelID,
		Models:       []Model{{ID: launch.ExpectedModelID, ManifestSHA256: launch.ModelManifestSHA256}},
		ReadyProfile: "interactive", ReadyRuntimeSHA256: launch.RuntimeSHA256, ReadyNativeRuntimeSHA256: launch.NativeRuntimeSHA256,
		ReadyModelID: launch.ExpectedModelID, ReadyManifestSHA256: launch.ModelManifestSHA256, ReadyAPIContract: "sne.openai-chat.v2",
		CacheTopology: "paged-ring-4096", ServingCacheCapacity: 4096, PrefixSessionsMaximum: 2,
		MaxConcurrentRequests: 1, MaxQueuedRequests: 8, QueueDiscipline: "fifo", RequestTimeoutMS: 120000,
		ReadyMaxConcurrentRequests: 1, ReadyMaxQueuedRequests: 8, ReadyQueueDiscipline: "fifo", ReadyRequestTimeoutMS: 120000,
	}
	if err := supervisor.validateReadinessIdentity(valid); err != nil {
		t.Fatalf("valid capacity identity rejected: %v", err)
	}
	valid.PrefixSessionsMaximum = 1
	if err := supervisor.validateReadinessIdentity(valid); err == nil {
		t.Fatal("reduced native session capacity was accepted")
	}
}

func TestReadinessAdmissionRejectsStaleProcessGeneration(t *testing.T) {
	supervisor := &Supervisor{
		cancel:     func() {},
		cmd:        &exec.Cmd{},
		generation: 7,
	}
	if supervisor.admitReadinessGeneration(6) {
		t.Fatal("stale process generation was admitted")
	}
	if !supervisor.admitReadinessGeneration(7) || supervisor.admitted != 7 {
		t.Fatal("current process generation was not admitted")
	}
}

func TestSupervisorRestartLaunchesFreshProcess(t *testing.T) {
	bindSafeSupervisorResources(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	profile := SupervisorProfile{}
	profile.SNE.Endpoint = "http://" + address
	profile.SNE.Profile = "interactive"
	bindSupervisorTestPolicy(&profile)
	countFile := filepath.Join(t.TempDir(), "starts.txt")
	launch := LaunchConfig{
		Executable: os.Args[0], ModelManifest: "/tmp/model.json",
		CheckpointDir: "/tmp/model", TokenizerJSON: "/tmp/tokenizer.json",
		AssistantSafetensors: "/tmp/assistant.safetensors", MetallibPath: "/tmp/mlx.metallib",
		MLXDylib: "/tmp/libmlx.dylib", NativeLibraryDir: "/tmp",
		RuntimeSHA256: strings.Repeat("a", 64), StartupTimeout: 5 * time.Second,
		ExpectedModelID: "gemma-test", ModelManifestSHA256: strings.Repeat("b", 64),
		Environment:          []string{"SNE_SUPERVISOR_HELPER=1", "SNE_SUPERVISOR_COUNT_FILE=" + countFile},
		allowTestEnvironment: true,
	}
	bindSupervisorTestManifest(t, &launch)
	launch.Environment = append(launch.Environment, "SNE_SUPERVISOR_MANIFEST_SHA="+launch.ModelManifestSHA256)
	supervisor, err := NewSupervisor(profile, launch)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = supervisor.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer supervisor.Stop(context.Background())
	if err = supervisor.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err = supervisor.Restart(ctx); err != nil {
		t.Fatal(err)
	}
	model := "gemma-4-12b-it-affine8-sne-v1"
	if err = supervisor.UnloadModel(ctx, model); err != nil {
		t.Fatal(err)
	}
	if err = supervisor.LoadModel(ctx, model); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(countFile)
	if err != nil {
		t.Fatal(err)
	}
	starts := strings.Fields(string(data))
	if len(starts) != 3 || starts[0] == starts[1] || starts[1] == starts[2] {
		t.Fatalf("fresh process starts=%q", starts)
	}
}

func TestSupervisorReplacesAdmittedProcessAfterConsecutiveReadinessFailures(t *testing.T) {
	bindSafeSupervisorResources(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	profile := SupervisorProfile{}
	profile.SNE.Endpoint = "http://" + address
	profile.SNE.Profile = "interactive"
	bindSupervisorTestPolicy(&profile)
	dir := t.TempDir()
	countFile := filepath.Join(dir, "starts.txt")
	failFile := filepath.Join(dir, "fail-ready")
	launch := LaunchConfig{
		Executable: os.Args[0], ModelManifest: "/tmp/model.json",
		CheckpointDir: "/tmp/model", TokenizerJSON: "/tmp/tokenizer.json",
		AssistantSafetensors: "/tmp/assistant.safetensors", MetallibPath: "/tmp/mlx.metallib",
		MLXDylib: "/tmp/libmlx.dylib", NativeLibraryDir: "/tmp",
		RuntimeSHA256: strings.Repeat("a", 64), StartupTimeout: 5 * time.Second,
		ExpectedModelID: "gemma-test", ModelManifestSHA256: strings.Repeat("b", 64),
		Environment: []string{
			"SNE_SUPERVISOR_HELPER=1",
			"SNE_SUPERVISOR_COUNT_FILE=" + countFile,
			"SNE_SUPERVISOR_FAIL_READY_FILE=" + failFile,
		},
		allowTestEnvironment: true,
	}
	bindSupervisorTestManifest(t, &launch)
	launch.Environment = append(launch.Environment, "SNE_SUPERVISOR_MANIFEST_SHA="+launch.ModelManifestSHA256)
	supervisor, err := NewSupervisor(profile, launch)
	if err != nil {
		t.Fatal(err)
	}
	supervisor.healthEvery = 20 * time.Millisecond
	supervisor.healthFails = 2
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = supervisor.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer supervisor.Stop(context.Background())
	if err = supervisor.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(failFile, []byte("fail\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for {
		data, readErr := os.ReadFile(countFile)
		if readErr == nil && len(strings.Fields(string(data))) >= 2 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("admitted unhealthy SNE process was not replaced")
		case <-time.After(20 * time.Millisecond):
		}
	}
	if err = supervisor.WaitReady(ctx); err != nil {
		t.Fatalf("replacement process did not pass exact readiness: %v", err)
	}
	data, err := os.ReadFile(countFile)
	if err != nil {
		t.Fatal(err)
	}
	starts := strings.Fields(string(data))
	if len(starts) != 2 || starts[0] == starts[1] {
		t.Fatalf("health recovery starts=%q, want exactly two distinct processes", starts)
	}
}
