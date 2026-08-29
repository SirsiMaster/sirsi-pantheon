package dashboard

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

func TestSNESupportDiagnosticsPreservesIdentityAndExcludesSensitiveFields(t *testing.T) {
	version := strings.Repeat("a", 64)
	failedAdmission := sne.ResourceAdmission{RequiredBytes: 25_545_459_702, AvailableRAMBytes: 18 << 30, LifecycleReserve: 4 << 30, SwapUsedBytes: 4 << 30, SwapLimitBytes: 3 << 30, Pressure: "normal"}
	model := SNEReadModel{
		ServiceState: "ready", ActiveModel: "gemma-4-12b-it-affine-8", DeviceFamily: "Apple M5 Max", UnifiedMemoryBytes: 48 << 30,
		LifecycleToolsReady: true, LifecycleToolsStatus: "Packaged checkout, recovery, and removal tools are available.",
		RuntimeCatalog: SNERuntimeCatalogStatus{State: "verified", SignedRequired: true, CatalogID: "sne-gemma4-v1", VersionSHA256: version, Entries: 11, Versions: 2, RetainedVersions: []string{version, strings.Repeat("b", 64)}, RollbackAvailable: true, UpdateFeedConfigured: true, Error: "/Users/owner/private/catalog"},
		Lifecycle:      SNELifecycleState{State: "failed", ModelID: "gemma-4-12b-it-affine-8", RuntimeID: "sne-v2", RuntimeSHA256: strings.Repeat("c", 64), ModelManifestSHA256: strings.Repeat("d", 64), Profile: "interactive", Error: "DYLD_LIBRARY_PATH=/private/tmp/secret", ErrorCode: "swap_cleanup_required", Recovery: "Restart the Mac and retry.", ResourceAdmission: &failedAdmission},
		Catalog:        []SNECatalogItem{{CatalogEntry: "gemma-4-12b-affine-8", ModelID: "gemma-4-12b-it-affine-8", RuntimeID: "sne-v2", ParameterClass: "12b", ExecutionMode: "mtp", WeightFormat: "affine", WeightBits: 8, MemoryBytes: 24 << 30, Qualification: "admitted", Installed: true, Active: true, State: "active", CacheTopology: "grouped-deferred", ServingCacheCapacity: 4096, PrefixSessionsMaximum: 0, PrefixSessionsSupported: false, Reason: "api_key=secret /Users/owner/model"}},
	}
	recovery := []recoveryTargetView{{TargetID: "nexus-local", Kind: "app_saved_state", RestoreSupported: true, FreshSupported: true, AutoResume: true, Mode: "restore", Phase: "ready"}}
	diagnostics := buildSNESupportDiagnostics(model, sne.ResourceAdmission{TotalRAMBytes: 48 << 30, AvailableRAMBytes: 20 << 30, SwapUsedBytes: 128 << 20, Pressure: "normal", PressureSource: "host_statistics64"}, recovery, time.Unix(1, 0))
	data, err := json.Marshal(diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{"/Users/", "/private/tmp", "DYLD_", "api_key", "private_key", "prompt", "generated_text", "assistant_safetensors", "Authorization", "Bearer", "access_token", "sne-local-api.token"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("diagnostics leaked forbidden value %q: %s", forbidden, text)
		}
	}
	for _, required := range []string{sneSupportDiagnosticsSchema, "sne-gemma4-v1", version, "sne-v2", strings.Repeat("c", 64), strings.Repeat("d", 64), "interactive", "grouped-deferred", "serving_cache_capacity", "gemma-4-12b-it-affine-8", "Apple M5 Max", "nexus-local", "app_saved_state", "auto_resume"} {
		if !strings.Contains(text, required) {
			t.Fatalf("diagnostics omitted identity %q: %s", required, text)
		}
	}
	if diagnostics.Lifecycle.ErrorCode != "swap_cleanup_required" || diagnostics.Lifecycle.Recovery != "Restart the Mac and retry." || diagnostics.Resources.SwapUsedBytes != failedAdmission.SwapUsedBytes {
		t.Fatalf("diagnostics omitted actionable failed admission: %+v", diagnostics)
	}
	if !diagnostics.LifecycleToolsReady || !strings.Contains(diagnostics.LifecycleToolsStatus, "removal") {
		t.Fatalf("diagnostics omitted lifecycle tool health: %+v", diagnostics)
	}
}

func TestSNESupportBundleAPIIsSameOriginBoundedAndDownloadable(t *testing.T) {
	want := []byte("PK\x03\x04support")
	manager := NewSNEInstallManager(SNEInstallConfig{supportBundle: func(_ context.Context, binary, verifier string) ([]byte, error) {
		if binary != "/package/tools/support-bundle.zsh" {
			t.Fatalf("unexpected helper: %q", binary)
		}
		if verifier != "/package/tools/verify-support-bundle-privacy.zsh" {
			t.Fatalf("unexpected verifier: %q", verifier)
		}
		return want, nil
	}, SupportBundleBinary: "/package/tools/support-bundle.zsh", SupportBundleVerifier: "/package/tools/verify-support-bundle-privacy.zsh"})
	server := &Server{sneJobs: manager}

	bad := httptest.NewRequest(http.MethodPost, "http://pantheon.test/api/sne/support-bundle", nil)
	bad.Host = "pantheon.test"
	bad.Header.Set("Origin", "https://attacker.example")
	badResult := httptest.NewRecorder()
	server.apiSNESupportBundle(badResult, bad)
	if badResult.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d", badResult.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "http://pantheon.test/api/sne/support-bundle", nil)
	request.Host = "pantheon.test"
	request.Header.Set("Origin", "http://pantheon.test")
	result := httptest.NewRecorder()
	server.apiSNESupportBundle(result, request)
	if result.Code != http.StatusOK || result.Header().Get("Content-Type") != "application/zip" || result.Header().Get("X-Sirsi-Support-Privacy-Verified") != "true" || !strings.Contains(result.Header().Get("Content-Disposition"), ".zip") || result.Body.String() != string(want) {
		t.Fatalf("unexpected support response: status=%d headers=%v body=%q", result.Code, result.Header(), result.Body.String())
	}
}

func TestSNESupportBundleRejectsMissingPackagedPrivacyVerifier(t *testing.T) {
	exporter := t.TempDir() + "/support-bundle.zsh"
	if err := os.WriteFile(exporter, []byte("#!/bin/zsh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	manager := NewSNEInstallManager(SNEInstallConfig{SupportBundleBinary: exporter})
	_, err := manager.SupportBundle(context.Background())
	if err == nil || !strings.Contains(err.Error(), "privacy verifier is unavailable") {
		t.Fatalf("missing verifier error = %v", err)
	}
}

func TestSNESupportBundleUIRequiresConsentAndReportsVerifiedOutcome(t *testing.T) {
	server := testServer(t, Config{})
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	page := string(body)
	for _, required := range []string{
		"Export complete privacy-safe SNE support bundle",
		"Create a privacy-safe SNE support bundle?",
		"e.key==='Enter'||e.key===' '",
		"Exported privacy-verified SNE support bundle.",
		"Support bundle export failed:",
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("SNE support UI omitted %q", required)
		}
	}
}

func TestSNESupportBundleRealPackagedIntegration(t *testing.T) {
	root := strings.TrimSpace(os.Getenv("SNE_TEST_PACKAGE_ROOT"))
	if root == "" {
		t.Skip("set SNE_TEST_PACKAGE_ROOT to run the real packaged support integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	archive, err := runSNESupportBundle(ctx,
		root+"/tools/support-bundle.zsh",
		root+"/tools/verify-support-bundle-privacy.zsh",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(archive) < 4 || string(archive[:4]) != "PK\x03\x04" {
		t.Fatalf("packaged integration returned an invalid ZIP: bytes=%d", len(archive))
	}
}
