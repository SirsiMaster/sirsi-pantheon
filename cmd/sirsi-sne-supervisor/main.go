package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

func main() {
	profilePath := flag.String("profile", "", "Pantheon SNE supervisor profile")
	admissionRegistry := flag.String("model-admission-registry", "", "Pantheon SNE model admission registry")
	readinessRegistry := flag.String("model-readiness-registry", "", "SNE evidence-readiness registry matching the admission catalog")
	readinessPolicy := flag.String("readiness-policy", sne.ReadinessPerformance, "required evidence gate: identity, correctness, performance, or release")
	catalogEntry := flag.String("catalog-entry", "", "exact admitted SNE catalog entry")
	allowResearch := flag.Bool("allow-research", false, "allow an explicitly selected research-qualified model")
	checkAdmission := flag.Bool("check-admission", false, "validate the selected model tuple and exit without launching SNE")
	executable := flag.String("sned", "", "path to the admitted sned executable")
	manifest := flag.String("model-manifest", "", "path to the admitted SNE model manifest")
	checkpoint := flag.String("checkpoint-dir", "", "complete target checkpoint directory")
	tokenizer := flag.String("tokenizer-json", "", "pinned tokenizer.json")
	assistant := flag.String("assistant-safetensors", "", "pinned MTP assistant safetensors")
	metallib := flag.String("metallib", "", "manifest-bound MLX metallib")
	mlxDylib := flag.String("mlx-dylib", "", "manifest-bound MLX dylib")
	nativeLibraryDir := flag.String("native-library-dir", "", "directory containing admitted SNE native libraries")
	runtimeSHA := flag.String("runtime-sha256", "", "admitted sned service executable SHA-256")
	nativeRuntimeSHA := flag.String("native-runtime-sha256", "", "admitted native inference dylib SHA-256")
	nativeRuntimeDylib := flag.String("native-runtime-dylib", "", "path to the admitted native inference dylib")
	serviceVersion := flag.String("service-version", "", "admitted SNE service version")
	accessTokenFile := flag.String("local-access-token-file", "", "private Pantheon-managed SNE local capability file")
	flag.Parse()
	if !*checkAdmission && strings.TrimSpace(*serviceVersion) == "" {
		fmt.Fprintln(os.Stderr, "admitted SNE service version is required")
		os.Exit(64)
	}

	profile, err := sne.LoadSupervisorProfile(*profilePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(64)
	}
	admission, readiness, err := sne.AdmitModelWithReadiness(*admissionRegistry, *catalogEntry, *manifest, *readinessRegistry, *readinessPolicy, *allowResearch, profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(64)
	}
	modelID, err := admittedModelID(*manifest)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(64)
	}
	if modelID != admission.ModelID {
		fmt.Fprintln(os.Stderr, "admitted model ID does not match selected catalog tuple")
		os.Exit(64)
	}
	manifestBytes, err := os.ReadFile(*manifest)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(64)
	}
	manifestDigest := sha256.Sum256(manifestBytes)
	manifestSHA256 := hex.EncodeToString(manifestDigest[:])
	if *checkAdmission {
		resource, resourceErr := sne.EvaluateResourceAdmission(admission.MemoryBytes, profile.SNE.YieldToForeground)
		if resourceErr != nil {
			printResourceAdmissionFailure(resourceErr, resource)
			os.Exit(69)
		}
		fmt.Printf("pantheon_sne admission=true catalog_entry=%s model=%s qualification=%s memory_bytes=%d readiness_policy=%s readiness_as_of=%s stability=%s clean100=%s disposition=%s\n", admission.CatalogEntry, admission.ModelID, admission.Qualification, admission.MemoryBytes, *readinessPolicy, readiness.RegistryAsOf, readiness.Stability, readiness.Clean100, readiness.Disposition)
		fmt.Printf("pantheon_sne caretaker=true required_bytes=%d available_bytes=%d reserve_bytes=%d lifecycle_reserve_bytes=%d swap_used_bytes=%d swap_limit_bytes=%d pressure=%s pressure_source=%s\n", resource.RequiredBytes, resource.AvailableRAMBytes, resource.DynamicReserve, resource.LifecycleReserve, resource.SwapUsedBytes, resource.SwapLimitBytes, resource.Pressure, resource.PressureSource)
		return
	}
	supervisor, err := sne.NewSupervisor(profile, sne.LaunchConfig{
		Executable: *executable, ModelManifest: *manifest, CheckpointDir: *checkpoint,
		TokenizerJSON: *tokenizer, AssistantSafetensors: *assistant,
		MetallibPath: *metallib,
		MLXDylib:     *mlxDylib, NativeLibraryDir: *nativeLibraryDir, NativeRuntimeDylib: *nativeRuntimeDylib,
		RuntimeSHA256: *runtimeSHA, NativeRuntimeSHA256: *nativeRuntimeSHA, ServiceVersion: *serviceVersion, ExpectedModelID: modelID, ModelManifestSHA256: manifestSHA256,
		RequiredMemoryBytes: admission.MemoryBytes, StartupTimeout: 5 * time.Minute,
		AccessTokenFile: *accessTokenFile,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(64)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := supervisor.Start(ctx); err != nil {
		if !printResourceAdmissionFailure(err, supervisor.ResourceAdmission()) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(70)
	}
	if err := supervisor.WaitReady(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(70)
	}
	resource := supervisor.ResourceAdmission()
	fmt.Printf("pantheon_sne caretaker=true required_bytes=%d available_bytes=%d reserve_bytes=%d lifecycle_reserve_bytes=%d swap_used_bytes=%d swap_limit_bytes=%d pressure=%s pressure_source=%s\n", resource.RequiredBytes, resource.AvailableRAMBytes, resource.DynamicReserve, resource.LifecycleReserve, resource.SwapUsedBytes, resource.SwapLimitBytes, resource.Pressure, resource.PressureSource)
	fmt.Printf("pantheon_sne ready=true profile=%s endpoint=%s catalog_entry=%s model=%s readiness_policy=%s clean100=%s disposition=%s\n", profile.SNE.Profile, profile.SNE.Endpoint, admission.CatalogEntry, admission.ModelID, *readinessPolicy, readiness.Clean100, readiness.Disposition)
	reloadSignal := make(chan os.Signal, 1)
	signal.Notify(reloadSignal, syscall.SIGHUP)
	defer signal.Stop(reloadSignal)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-reloadSignal:
				reloadContext, reloadCancel := context.WithTimeout(context.Background(), 10*time.Minute)
				err := supervisor.ReloadModel(reloadContext, modelID)
				reloadCancel()
				if err != nil {
					if !printResourceAdmissionFailure(err, supervisor.ResourceAdmission()) {
						fmt.Fprintf(os.Stderr, "pantheon_sne supervised_restart=false error=%q\n", err)
					}
					continue
				}
				fmt.Printf("pantheon_sne supervised_restart=true model=%s\n", modelID)
			}
		}
	}()
	if err := supervisor.Wait(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(70)
	}
	stopContext, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer stopCancel()
	if err := supervisor.Stop(stopContext); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(70)
	}
}

func printResourceAdmissionFailure(err error, resource sne.ResourceAdmission) bool {
	var admissionErr *sne.ResourceAdmissionError
	if !errors.As(err, &admissionErr) {
		return false
	}
	fmt.Fprintf(os.Stderr, "pantheon_sne admission=false code=%s required_bytes=%d available_bytes=%d reserve_bytes=%d lifecycle_reserve_bytes=%d swap_used_bytes=%d swap_limit_bytes=%d pressure=%s recovery=%q\n", admissionErr.Code, resource.RequiredBytes, resource.AvailableRAMBytes, resource.DynamicReserve, resource.LifecycleReserve, resource.SwapUsedBytes, resource.SwapLimitBytes, resource.Pressure, admissionErr.Recovery)
	return true
}

func admittedModelID(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open admitted SNE model manifest: %w", err)
	}
	defer file.Close()
	var manifest struct {
		Model struct {
			ID string `json:"id"`
		} `json:"model"`
	}
	if err := json.NewDecoder(file).Decode(&manifest); err != nil {
		return "", fmt.Errorf("decode admitted SNE model manifest: %w", err)
	}
	if strings.TrimSpace(manifest.Model.ID) == "" {
		return "", fmt.Errorf("admitted SNE model manifest has no model ID")
	}
	return manifest.Model.ID, nil
}
