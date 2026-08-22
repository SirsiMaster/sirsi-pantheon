package dashboard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/snemodels"
)

func TestSNEInstallAPICompletesVerifiedTransaction(t *testing.T) {
	cfg := sneInstallFixture(t)
	cfg.Acquire = func(_ context.Context, entry snemodels.SourceEntry, options snemodels.AcquireOptions) (snemodels.AcquireResult, error) {
		if err := os.MkdirAll(options.Destination, 0o700); err != nil {
			return snemodels.AcquireResult{}, err
		}
		if err := os.WriteFile(filepath.Join(options.Destination, "model.bin"), []byte("data"), 0o600); err != nil {
			return snemodels.AcquireResult{}, err
		}
		progress := snemodels.Progress{CatalogEntry: entry.CatalogEntry, FilesDone: 1, FilesTotal: 1, BytesDone: 4, BytesTotal: 4}
		if options.Progress != nil {
			options.Progress(progress)
		}
		return snemodels.AcquireResult{CatalogEntry: entry.CatalogEntry, SourceDir: options.Destination}, nil
	}
	ts := testServer(t, Config{SNEInstall: &cfg})
	defer ts.Close()
	body := bytes.NewBufferString(`{"catalog_entry":"test-entry","accept_license":true,"allow_research":false}`)
	response, err := http.Post(ts.URL+"/api/sne/install", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	var job SNEInstallJob
	if err := json.NewDecoder(response.Body).Decode(&job); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, err := http.Get(ts.URL + "/api/sne/install/status?id=" + job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.NewDecoder(status.Body).Decode(&job); err != nil {
			status.Body.Close()
			t.Fatal(err)
		}
		status.Body.Close()
		if job.State == "installed" {
			prepared := filepath.Join(cfg.PreparedRoot, "test-entry", strings.Repeat("d", 40))
			if _, err := os.Stat(prepared); !os.IsNotExist(err) {
				t.Fatalf("verified prepared source was not removed: %v", err)
			}
			return
		}
		if job.State == "failed" {
			t.Fatalf("job failed: %s", job.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("installation did not complete")
}

func TestSNEInstallRetainsPreparedSourceWhenCheckoutFails(t *testing.T) {
	cfg := sneInstallFixture(t)
	if err := os.WriteFile(cfg.CheckoutBinary, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg.Acquire = func(_ context.Context, entry snemodels.SourceEntry, options snemodels.AcquireOptions) (snemodels.AcquireResult, error) {
		if err := os.MkdirAll(options.Destination, 0o700); err != nil {
			return snemodels.AcquireResult{}, err
		}
		if err := os.WriteFile(filepath.Join(options.Destination, "model.bin"), []byte("data"), 0o600); err != nil {
			return snemodels.AcquireResult{}, err
		}
		return snemodels.AcquireResult{CatalogEntry: entry.CatalogEntry, SourceDir: options.Destination}, nil
	}
	manager := NewSNEInstallManager(cfg)
	job, err := manager.Start(SNEInstallRequest{CatalogEntry: "test-entry", AcceptLicense: true})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		current, ok := manager.Job(job.ID)
		if !ok {
			t.Fatal("install job disappeared")
		}
		if current.State == "failed" {
			prepared := filepath.Join(cfg.PreparedRoot, "test-entry", strings.Repeat("d", 40), "model.bin")
			if _, err := os.Stat(prepared); err != nil {
				t.Fatalf("failed checkout did not retain prepared source: %v", err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("failed checkout did not finish")
}

func TestRemovePreparedSourceRejectsPreparedRoot(t *testing.T) {
	root := t.TempDir()
	if err := removePreparedSource(root, root); err == nil {
		t.Fatal("cleanup accepted the prepared root itself")
	}
}

func TestSNEDiscardPreparedUsesSignedCatalogIdentity(t *testing.T) {
	cfg := sneInstallFixture(t)
	manager := NewSNEInstallManager(cfg)
	destination := filepath.Join(cfg.PreparedRoot, "test-entry", strings.Repeat("d", 40))
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "model.bin.partial"), []byte("part"), 0o600); err != nil {
		t.Fatal(err)
	}
	receipt, err := manager.DiscardPrepared(SNEDiscardPreparedRequest{CatalogEntry: "test-entry"})
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Removed || receipt.CatalogEntry != "test-entry" || receipt.Revision != strings.Repeat("d", 40) {
		t.Fatalf("unexpected cleanup receipt: %+v", receipt)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("retained prepared source still exists: %v", err)
	}
}

func TestSNEDiscardPreparedRejectsUnknownAndActiveIdentity(t *testing.T) {
	cfg := sneInstallFixture(t)
	manager := NewSNEInstallManager(cfg)
	if _, err := manager.DiscardPrepared(SNEDiscardPreparedRequest{CatalogEntry: "../escape"}); err == nil {
		t.Fatal("unknown/path-like catalog entry was accepted")
	}
	manager.jobs["active"] = SNEInstallJob{ID: "active", State: "acquiring"}
	if _, err := manager.DiscardPrepared(SNEDiscardPreparedRequest{CatalogEntry: "test-entry"}); err == nil || !strings.Contains(err.Error(), "active") {
		t.Fatalf("active installation did not block cleanup: %v", err)
	}
}

func TestSNEDiscardPreparedAPIRejectsCrossOriginAndReturnsReceipt(t *testing.T) {
	cfg := sneInstallFixture(t)
	destination := filepath.Join(cfg.PreparedRoot, "test-entry", strings.Repeat("d", 40))
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	server := &Server{sneJobs: NewSNEInstallManager(cfg)}
	body := `{"catalog_entry":"test-entry"}`
	bad := httptest.NewRequest(http.MethodPost, "http://pantheon.test/api/sne/prepared/discard", strings.NewReader(body))
	bad.Host = "pantheon.test"
	bad.Header.Set("Origin", "https://attacker.example")
	badResult := httptest.NewRecorder()
	server.apiSNEDiscardPrepared(badResult, bad)
	if badResult.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d", badResult.Code)
	}
	request := httptest.NewRequest(http.MethodPost, "http://pantheon.test/api/sne/prepared/discard", strings.NewReader(body))
	request.Host = "pantheon.test"
	result := httptest.NewRecorder()
	server.apiSNEDiscardPrepared(result, request)
	if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), `"removed":true`) {
		t.Fatalf("cleanup response = %d %s", result.Code, result.Body.String())
	}
}

func TestSNEDiscardPreparedControlIsFailureScopedAndAccessible(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodGet, "http://pantheon.test/sne", nil)
	result := httptest.NewRecorder()
	server.handleOverview(result, request)
	if result.Code != http.StatusOK {
		t.Fatalf("overview status = %d", result.Code)
	}
	page := result.Body.String()
	for _, required := range []string{
		"if(job.state==='failed')",
		"[Discard retained download]",
		"Discard retained download for ",
		"discard.tabIndex=0",
		"discard.setAttribute('role','button')",
		"discard.onkeydown=function(e)",
		"Installed models and shared model-store objects are not changed.",
		"/api/sne/prepared/discard",
		"catalog_entry:catalogEntry",
		"Retained download cleanup rejected:",
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("overview omitted retained-source cleanup contract %q", required)
		}
	}
}

func TestSNEInstallAPIFailsClosed(t *testing.T) {
	cfg := sneInstallFixture(t)
	ts := testServer(t, Config{SNEInstall: &cfg})
	defer ts.Close()
	for _, test := range []struct {
		name, body, origin string
		status             int
	}{
		{"license", `{"catalog_entry":"test-entry"}`, "", http.StatusConflict},
		{"origin", `{"catalog_entry":"test-entry","accept_license":true}`, "https://evil.example", http.StatusForbidden},
		{"unknown-field", `{"catalog_entry":"test-entry","accept_license":true,"command":"rm"}`, "", http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/sne/install", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != test.status {
				t.Fatalf("status = %d, want %d", response.StatusCode, test.status)
			}
		})
	}
}

func TestSNEInstallRejectsUnrecoveredModelStore(t *testing.T) {
	cfg := sneInstallFixture(t)
	cfg.RequireRecovery = true
	cfg.recover = func(context.Context, string, string) error {
		return fmt.Errorf("interrupted removal receipt is invalid")
	}
	manager := NewSNEInstallManager(cfg)
	if len(manager.Available()) != 0 {
		t.Fatal("unrecovered model store remained install-available")
	}
	if _, err := manager.Start(SNEInstallRequest{CatalogEntry: "test-entry", AcceptLicense: true}); err == nil || !strings.Contains(err.Error(), "not recovered") {
		t.Fatalf("install did not fail closed on recovery: %v", err)
	}
}

func TestSNEModelRemovalRequiresExactAdmittedIdentity(t *testing.T) {
	cfg := sneInstallFixture(t)
	cfg.remove = func(_ context.Context, _, _, _, _ string) (json.RawMessage, error) {
		t.Fatal("removal helper must not run for a mismatched identity")
		return nil, nil
	}
	manager := NewSNEInstallManager(cfg)
	if _, err := manager.Remove(context.Background(), SNERemoveRequest{CatalogEntry: "test-entry", ModelID: "wrong-model"}); err == nil || !strings.Contains(err.Error(), "do not match") {
		t.Fatalf("expected exact identity rejection, got %v", err)
	}
}

func TestSNEModelRemovalReturnsNativeReceipt(t *testing.T) {
	cfg := sneInstallFixture(t)
	cfg.remove = func(_ context.Context, binary, catalog, modelID, store string) (json.RawMessage, error) {
		if modelID != "test-model" || catalog != cfg.ModelCatalogRoot || store != cfg.StoreRoot {
			t.Fatalf("unexpected removal identity: %q %q %q", catalog, modelID, store)
		}
		return json.RawMessage(`{"model_id":"test-model","objects_retained":2}`), nil
	}
	manager := NewSNEInstallManager(cfg)
	result, err := manager.Remove(context.Background(), SNERemoveRequest{CatalogEntry: "test-entry", ModelID: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), `"objects_retained":2`) {
		t.Fatalf("unexpected removal receipt: %s", result)
	}
}

func TestSNEModelRemovalAPIRejectsCrossOriginAndActiveRuntime(t *testing.T) {
	cfg := sneInstallFixture(t)
	cfg.remove = func(_ context.Context, _, _, _, _ string) (json.RawMessage, error) {
		t.Fatal("removal helper must not run")
		return nil, nil
	}
	server := &Server{
		sneJobs:      NewSNEInstallManager(cfg),
		sneLifecycle: &SNELifecycleManager{state: SNELifecycleState{State: "stopped"}},
	}
	body := `{"catalog_entry":"test-entry","model_id":"test-model"}`

	crossOrigin := httptest.NewRequest(http.MethodPost, "http://pantheon.test/api/sne/remove", strings.NewReader(body))
	crossOrigin.Host = "pantheon.test"
	crossOrigin.Header.Set("Origin", "https://attacker.example")
	crossOriginResult := httptest.NewRecorder()
	server.apiSNERemove(crossOriginResult, crossOrigin)
	if crossOriginResult.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d, want %d", crossOriginResult.Code, http.StatusForbidden)
	}

	server.sneLifecycle.state = SNELifecycleState{State: "ready", ModelID: "test-model"}
	active := httptest.NewRequest(http.MethodPost, "http://pantheon.test/api/sne/remove", strings.NewReader(body))
	active.Host = "pantheon.test"
	activeResult := httptest.NewRecorder()
	server.apiSNERemove(activeResult, active)
	if activeResult.Code != http.StatusConflict || !strings.Contains(activeResult.Body.String(), "stop SNE") {
		t.Fatalf("active removal response = %d %s", activeResult.Code, activeResult.Body.String())
	}
}

func TestSNEModelRemovalAPIReturnsExactReceipt(t *testing.T) {
	cfg := sneInstallFixture(t)
	cfg.remove = func(_ context.Context, _, _, modelID, _ string) (json.RawMessage, error) {
		if modelID != "test-model" {
			t.Fatalf("model ID = %q", modelID)
		}
		return json.RawMessage(`{"model_id":"test-model","objects_removed":3,"objects_retained":2}`), nil
	}
	server := &Server{
		sneJobs:      NewSNEInstallManager(cfg),
		sneLifecycle: &SNELifecycleManager{state: SNELifecycleState{State: "stopped"}},
	}
	request := httptest.NewRequest(http.MethodPost, "http://pantheon.test/api/sne/remove", strings.NewReader(`{"catalog_entry":"test-entry","model_id":"test-model"}`))
	request.Host = "pantheon.test"
	result := httptest.NewRecorder()
	server.apiSNERemove(result, request)
	if result.Code != http.StatusOK {
		t.Fatalf("removal status = %d: %s", result.Code, result.Body.String())
	}
	var envelope struct {
		Result struct {
			ModelID         string `json:"model_id"`
			ObjectsRemoved  int    `json:"objects_removed"`
			ObjectsRetained int    `json:"objects_retained"`
		} `json:"result"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Result.ModelID != "test-model" || envelope.Result.ObjectsRemoved != 3 || envelope.Result.ObjectsRetained != 2 {
		t.Fatalf("unexpected removal receipt: %+v", envelope.Result)
	}
}

func TestSNEModelRemovalControlIsSeparateAndKeyboardAccessible(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodGet, "http://pantheon.test/sne", nil)
	result := httptest.NewRecorder()
	server.handleOverview(result, request)
	if result.Code != http.StatusOK {
		t.Fatalf("overview status = %d", result.Code)
	}
	page := result.Body.String()
	for _, required := range []string{
		"[Remove model]",
		"Remove installed model ",
		"remove.tabIndex=0",
		"remove.onkeydown=function(e)",
		"if(e.key==='Enter'||e.key===' ')",
		"retain any objects shared by another installed model",
		"allow the model to be installed again later",
		"row.appendChild(action)",
		"row.appendChild(remove)",
		".t-action:focus-visible,.nav-item:focus-visible",
		"@media (prefers-reduced-motion:reduce)",
		"@media (prefers-contrast:more)",
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("overview omitted governed removal UI contract %q", required)
		}
	}
}

func sneInstallFixture(t *testing.T) SNEInstallConfig {
	t.Helper()
	root := t.TempDir()
	content := []byte("data")
	digest := sha256.Sum256(content)
	source := snemodels.SourceCatalog{Schema: "pantheon.sne-model-source.v1", CatalogID: "test-catalog", Entries: []snemodels.SourceEntry{{CatalogEntry: "test-entry", Provider: "huggingface", Repository: "owner/repo", Revision: strings.Repeat("d", 40), LicenseID: "terms", Files: []snemodels.SourceFile{{Path: "model.bin", SHA256: fmt.Sprintf("%x", digest), SizeBytes: 4}}}}}
	sourceData, _ := json.Marshal(source)
	sourcePath := filepath.Join(root, "sources.json")
	if err := os.WriteFile(sourcePath, sourceData, 0o600); err != nil {
		t.Fatal(err)
	}
	admission := `{"schema_version":"pantheon.sne-model-admission.v1","catalog_id":"test-catalog","family":"gemma-4","lifecycle_policy":"supervised-restart","entries":[{"catalog_entry":"test-entry","manifest_sha256":"` + strings.Repeat("a", 64) + `","model_id":"test-model","architecture":"gemma4-dense","parameter_class":"12B","adapter":"gemma4-dense-v0","execution_mode":"plain","weight_format":"affine","weight_bits":4,"weight_group_size":64,"memory_bytes":1,"qualification":"candidate","checkpoint_sha256":"` + strings.Repeat("b", 64) + `","artifact_set_sha256":"` + strings.Repeat("c", 64) + `"}]}`
	admissionPath := filepath.Join(root, "admission.json")
	if err := os.WriteFile(admissionPath, []byte(admission), 0o600); err != nil {
		t.Fatal(err)
	}
	checkout := filepath.Join(root, "checkout")
	checkoutScript := "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"result\",\"result\":{\"model_id\":\"test-model\",\"checkpoint_dir\":\"/installed\"}}'\n"
	if err := os.WriteFile(checkout, []byte(checkoutScript), 0o700); err != nil {
		t.Fatal(err)
	}
	return SNEInstallConfig{SourceCatalog: sourcePath, AdmissionRegistry: admissionPath, ModelCatalogRoot: filepath.Join(root, "catalog"), PreparedRoot: filepath.Join(root, "prepared"), StoreRoot: filepath.Join(root, "store"), CheckoutBinary: checkout}
}
