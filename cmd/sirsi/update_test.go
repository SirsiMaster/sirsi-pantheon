package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/updater"
)

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
