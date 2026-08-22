package sne

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestRealSupervisorProcessScopedLifecycle(t *testing.T) {
	packageRoot := os.Getenv("SNE_REAL_PACKAGE")
	checkpoint := os.Getenv("SNE_REAL_CHECKPOINT")
	assistant := os.Getenv("SNE_REAL_ASSISTANT")
	if packageRoot == "" || checkpoint == "" || assistant == "" {
		t.Skip("real SNE package inputs not supplied")
	}
	serviceHash, err := fileSHA256(filepath.Join(packageRoot, "bin", "sned"))
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(packageRoot, "manifests", "model.json")
	manifestHash, err := fileSHA256(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	modelID := "gemma-4-12b-it-affine8-sne-v1"
	profile := SupervisorProfile{}
	profile.SNE.Endpoint = "http://127.0.0.1:8478"
	profile.SNE.Profile = "interactive"
	bindSupervisorTestPolicy(&profile)
	launch := LaunchConfig{
		Executable:           filepath.Join(packageRoot, "bin", "sned"),
		ModelManifest:        manifestPath,
		CheckpointDir:        checkpoint,
		TokenizerJSON:        filepath.Join(checkpoint, "tokenizer.json"),
		AssistantSafetensors: assistant,
		MetallibPath:         filepath.Join(packageRoot, "share", "mlx.metallib"),
		MLXDylib:             filepath.Join(packageRoot, "lib", "libmlx.dylib"),
		NativeLibraryDir:     filepath.Join(packageRoot, "lib", "runtime"),
		RuntimeSHA256:        serviceHash,
		ExpectedModelID:      modelID,
		ModelManifestSHA256:  manifestHash,
		StartupTimeout:       5 * time.Minute,
	}
	supervisor, err := NewSupervisor(profile, launch)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := supervisor.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer supervisor.Stop(context.Background())
	if err := supervisor.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	model := modelID
	firstPID := supervisorPID(supervisor)
	first := exactCompletion(t, profile.SNE.Endpoint, model)
	if err := supervisor.UnloadModel(ctx, model); err != nil {
		t.Fatal(err)
	}
	if supervisor.client.Ready(context.Background()) {
		t.Fatal("SNE remained ready after supervised unload")
	}
	if err := supervisor.LoadModel(ctx, model); err != nil {
		t.Fatal(err)
	}
	secondPID := supervisorPID(supervisor)
	second := exactCompletion(t, profile.SNE.Endpoint, model)
	if err := supervisor.ReloadModel(ctx, model); err != nil {
		t.Fatal(err)
	}
	thirdPID := supervisorPID(supervisor)
	third := exactCompletion(t, profile.SNE.Endpoint, model)
	if firstPID <= 0 || secondPID <= 0 || thirdPID <= 0 || firstPID == secondPID || secondPID == thirdPID || firstPID == thirdPID {
		t.Fatalf("process lifecycle PIDs=%d,%d,%d", firstPID, secondPID, thirdPID)
	}
	if first != second || second != third {
		t.Fatal("completion changed across supervised lifecycle")
	}
	want := "fda564ba3f7a0f028106d468420f674898ed99ac5bf2765ac9586206e39d73c5"
	sum := sha256.Sum256([]byte(first))
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("content SHA=%s want=%s", got, want)
	}
	t.Logf("pantheon_sne_lifecycle accepted=true pids=%d,%d,%d content_sha256=%s", firstPID, secondPID, thirdPID, want)
}

func TestRealSupervisorExactReadinessIdentity(t *testing.T) {
	packageRoot := os.Getenv("SNE_REAL_READINESS_PACKAGE")
	checkpoint := os.Getenv("SNE_REAL_READINESS_CHECKPOINT")
	if packageRoot == "" || checkpoint == "" {
		t.Skip("real SNE readiness inputs not supplied")
	}
	manifestPath := filepath.Join(packageRoot, "manifests", "model.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Model struct {
			ID string `json:"id"`
		} `json:"model"`
		Architecture struct {
			CacheTopology string `json:"cache_topology"`
		} `json:"architecture"`
		Qualification struct {
			ServingCacheCapacity int `json:"serving_cache_capacity"`
		} `json:"qualification"`
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil || manifest.Model.ID == "" {
		t.Fatalf("decode manifest identity: %v", err)
	}
	manifestDigest := sha256.Sum256(manifestData)
	serviceHash, err := fileSHA256(filepath.Join(packageRoot, "bin", "sned"))
	if err != nil {
		t.Fatal(err)
	}
	profile := SupervisorProfile{}
	profile.SNE.Endpoint = "http://127.0.0.1:8479"
	profile.SNE.Profile = "interactive"
	bindSupervisorTestPolicy(&profile)
	launch := LaunchConfig{
		Executable: filepath.Join(packageRoot, "bin", "sned"), ModelManifest: manifestPath,
		CheckpointDir: checkpoint, TokenizerJSON: filepath.Join(checkpoint, "tokenizer.json"),
		MetallibPath: filepath.Join(packageRoot, "share", "mlx.metallib"),
		MLXDylib:     filepath.Join(packageRoot, "lib", "libmlx.dylib"), NativeLibraryDir: filepath.Join(packageRoot, "lib", "runtime"),
		RuntimeSHA256: serviceHash, ExpectedModelID: manifest.Model.ID,
		ModelManifestSHA256: hex.EncodeToString(manifestDigest[:]), StartupTimeout: 5 * time.Minute,
	}
	if expected := os.Getenv("SNE_REAL_PREFIX_SESSIONS_MAXIMUM"); expected != "" {
		maximum, err := strconv.Atoi(expected)
		if err != nil || maximum < 1 {
			t.Fatalf("invalid SNE_REAL_PREFIX_SESSIONS_MAXIMUM=%q", expected)
		}
		launch.ExpectedCacheTopology = manifest.Architecture.CacheTopology
		launch.ExpectedServingCacheCapacity = manifest.Qualification.ServingCacheCapacity
		launch.ExpectedPrefixSessionsMaximum = maximum
	}
	if required := os.Getenv("SNE_REAL_REQUIRED_MEMORY_BYTES"); required != "" {
		bytes, err := strconv.ParseUint(required, 10, 64)
		if err != nil || bytes < 1 {
			t.Fatalf("invalid SNE_REAL_REQUIRED_MEMORY_BYTES=%q", required)
		}
		launch.RequiredMemoryBytes = bytes
	}
	supervisor, err := NewSupervisor(profile, launch)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := supervisor.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer supervisor.Stop(context.Background())
	if err := supervisor.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	identity, err := supervisor.client.ReadinessIdentity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := supervisor.validateReadinessIdentity(identity); err != nil {
		t.Fatal(err)
	}
	firstPID := supervisorPID(supervisor)
	secondPID := 0
	if os.Getenv("SNE_REAL_EXACT_RESTART") == "1" {
		if err := supervisor.Restart(ctx); err != nil {
			t.Fatal(err)
		}
		secondPID = supervisorPID(supervisor)
		if firstPID <= 0 || secondPID <= 0 || firstPID == secondPID {
			t.Fatalf("supervised restart did not create a distinct process: first=%d second=%d", firstPID, secondPID)
		}
		identity, err = supervisor.client.ReadinessIdentity(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := supervisor.validateReadinessIdentity(identity); err != nil {
			t.Fatal(err)
		}
	}
	resource := supervisor.ResourceAdmission()
	t.Logf("pantheon_sne_exact_readiness accepted=true model=%s runtime_sha256=%s manifest_sha256=%s service_version=%s cache_topology=%s serving_cache_capacity=%d prefix_sessions_maximum=%d required_bytes=%d available_bytes=%d reserve_bytes=%d lifecycle_reserve_bytes=%d swap_used_bytes=%d swap_limit_bytes=%d pressure=%s pressure_source=%s first_pid=%d second_pid=%d", manifest.Model.ID, serviceHash, launch.ModelManifestSHA256, identity.ServiceVersion, identity.CacheTopology, identity.ServingCacheCapacity, identity.PrefixSessionsMaximum, resource.RequiredBytes, resource.AvailableRAMBytes, resource.DynamicReserve, resource.LifecycleReserve, resource.SwapUsedBytes, resource.SwapLimitBytes, resource.Pressure, resource.PressureSource, firstPID, secondPID)
}

func supervisorPID(supervisor *Supervisor) int {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	if supervisor.cmd == nil || supervisor.cmd.Process == nil || supervisor.cancel == nil {
		return 0
	}
	return supervisor.cmd.Process.Pid
}

func exactCompletion(t *testing.T, endpoint, model string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"model":       model,
		"messages":    []map[string]string{{"role": "user", "content": "Hello!"}},
		"temperature": 0,
		"top_p":       0,
		"max_tokens":  32,
		"stream":      false,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, endpoint+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("completion status=%d body=%s", response.StatusCode, data)
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Choices) != 1 || envelope.Choices[0].Message.Content == "" {
		t.Fatal(fmt.Errorf("invalid SNE completion envelope"))
	}
	return envelope.Choices[0].Message.Content
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
