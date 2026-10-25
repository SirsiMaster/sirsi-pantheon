package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/updater"
)

func TestInstallAppReleaseRefusesBrokenVersionBeforeAssets(t *testing.T) {
	// No assets: without the compatibility check this returns the distinct
	// missing-payload error. No download or external open is possible. Do not
	// mutate the process-wide version stamp just to test this entrypoint.
	err := installAppRelease(&updater.Release{TagName: "v0.24.114"})
	if !errors.Is(err, updater.ErrIncompatibleUpdate) {
		t.Fatalf("expected early compatibility refusal, got %v", err)
	}
}

func TestRunUpdateRollbackRequiresExplicitConfirmationBeforeStoreOrNetwork(t *testing.T) {
	oldVersion, oldConfirm := updateRollbackVersion, updateConfirmRollback
	oldClient := newUpdateClient
	updateRollbackVersion, updateConfirmRollback = "0.24.136", false
	newUpdateClient = func() *updater.Client { t.Fatal("unconfirmed rollback reached release client"); return nil }
	defer func() {
		updateRollbackVersion, updateConfirmRollback, newUpdateClient = oldVersion, oldConfirm, oldClient
	}()
	if err := runUpdate(updateCmd, nil); err == nil || !strings.Contains(err.Error(), "--confirm-rollback") {
		t.Fatalf("unconfirmed exact rollback was not refused before external work: %v", err)
	}
}

func TestUpdateCobraParsesExactRollbackFlagAndRequiresConfirmation(t *testing.T) {
	oldVersion, oldConfirm := updateRollbackVersion, updateConfirmRollback
	oldCLI, oldApp := updateInstallCLI, updateInstallApp
	oldReconcile, oldCapability := updateReconcileAppAttempt, updateAppUpdateRecoveryCapability
	oldClient := newUpdateClient
	flagNames := []string{"cli", "app", "rollback-version", "confirm-rollback", "reconcile-app-attempt", "app-update-recovery-capability"}
	changedBefore := make(map[string]bool, len(flagNames))
	for _, name := range flagNames {
		flag := updateCmd.Flags().Lookup(name)
		if flag == nil {
			t.Fatalf("update flag %q is not registered", name)
		}
		changedBefore[name] = flag.Changed
	}
	defer func() {
		updateRollbackVersion, updateConfirmRollback = oldVersion, oldConfirm
		updateInstallCLI, updateInstallApp, newUpdateClient = oldCLI, oldApp, oldClient
		updateReconcileAppAttempt, updateAppUpdateRecoveryCapability = oldReconcile, oldCapability
		for name, changed := range changedBefore {
			updateCmd.Flags().Lookup(name).Changed = changed
		}
	}()
	updateInstallCLI, updateInstallApp = false, false
	updateReconcileAppAttempt, updateAppUpdateRecoveryCapability = false, false
	updateRollbackVersion, updateConfirmRollback = "", false
	newUpdateClient = func() *updater.Client { t.Fatal("unconfirmed Cobra rollback reached release client"); return nil }
	for _, name := range flagNames {
		updateCmd.Flags().Lookup(name).Changed = false
	}
	if err := updateCmd.ParseFlags([]string{"--rollback-version", "0.24.136"}); err != nil {
		t.Fatalf("Cobra rejected exact rollback flag: %v", err)
	}
	if updateRollbackVersion != "0.24.136" || !updateCmd.Flags().Lookup("rollback-version").Changed || updateConfirmRollback || updateCmd.Flags().Lookup("confirm-rollback").Changed {
		t.Fatalf("Cobra parsed unexpected rollback state: version=%q confirm=%v", updateRollbackVersion, updateConfirmRollback)
	}
	if err := runUpdate(updateCmd, nil); err == nil || !strings.Contains(err.Error(), "--confirm-rollback") {
		t.Fatalf("Cobra did not preserve explicit rollback confirmation requirement: %v", err)
	}
}

func TestInstallAppRollbackReleaseRejectsTagMismatch(t *testing.T) {
	oldVersion := updateRollbackVersion
	updateRollbackVersion = "0.24.136"
	defer func() { updateRollbackVersion = oldVersion }()
	if err := installAppRollbackRelease(&updater.Release{TagName: "v0.24.137"}); err == nil || !strings.Contains(err.Error(), "tag changed") {
		t.Fatalf("different release tag passed exact rollback selection: %v", err)
	}
}

func TestRunUpdate_NoCompleteCommercialReleaseIsRecoverableForInstallRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"tag_name":"v0.24.33","assets":[]}]`)
	}))
	defer srv.Close()

	previousClient := newUpdateClient
	newUpdateClient = func() *updater.Client {
		return &updater.Client{ReleasesURL: srv.URL, AdvisoryURL: srv.URL, HTTPClient: srv.Client()}
	}
	defer func() { newUpdateClient = previousClient }()

	previousCLI, previousApp := updateInstallCLI, updateInstallApp
	defer func() { updateInstallCLI, updateInstallApp = previousCLI, previousApp }()

	for _, request := range []struct {
		name string
		cli  bool
		app  bool
	}{
		{name: "app", app: true},
		{name: "cli compatibility alias", cli: true},
	} {
		t.Run(request.name, func(t *testing.T) {
			updateInstallCLI, updateInstallApp = request.cli, request.app
			output := captureUpdateStdout(t, func() error { return runUpdate(updateCmd, nil) })
			if !strings.Contains(output, "installed version remains active") || !strings.Contains(output, "Recovery: recheck later") {
				t.Fatalf("recovery output = %q", output)
			}
		})
	}
}

func captureUpdateStdout(t *testing.T, action func() error) string {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = writer
	err = action()
	_ = writer.Close()
	os.Stdout = previous
	if err != nil {
		t.Fatalf("run update: %v", err)
	}
	var captured bytes.Buffer
	if _, copyErr := io.Copy(&captured, reader); copyErr != nil {
		t.Fatalf("read stdout: %v", copyErr)
	}
	_ = reader.Close()
	return captured.String()
}
