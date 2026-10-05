package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	buildversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

func TestFetchDashboardIdentityAcceptsExactReadOnlyContract(t *testing.T) {
	want := dashboard.IdentityResponse{
		Schema: dashboard.IdentitySchema,
		Info:   buildversion.Info{Version: "0.23.9-beta", Commit: "dd0e701b", Path: "/tmp/sirsi"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/identity" {
			t.Fatalf("request = %s %s, want GET /api/identity", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	got, err := fetchDashboardIdentity(srv.Client(), srv.URL+"/api/identity")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("identity = %+v, want %+v", got, want)
	}
}

func TestDashboardIdentityMustMatchCurrentBuildAndExecutable(t *testing.T) {
	dir := t.TempDir()
	selfPath := filepath.Join(dir, "sirsi")
	otherPath := filepath.Join(dir, "other-sirsi")
	if err := os.WriteFile(selfPath, []byte("same executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(otherPath, []byte("different executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	self := buildversion.Info{Binary: "sirsi", Version: "1.2.3", Commit: "abc123", Date: "2026-09-23", Path: selfPath, Dirty: true}
	running := dashboard.IdentityResponse{Schema: dashboard.IdentitySchema, Info: self}
	if err := validateDashboardIdentityMatchesSelf(self, running); err != nil {
		t.Fatalf("same executable/build rejected: %v", err)
	}

	wrongBuild := running
	wrongBuild.Commit = "def456"
	if err := validateDashboardIdentityMatchesSelf(self, wrongBuild); err == nil || !strings.Contains(err.Error(), "commit") {
		t.Fatalf("different build accepted or misreported: %v", err)
	}
	wrongExecutable := running
	wrongExecutable.Path = otherPath
	if err := validateDashboardIdentityMatchesSelf(self, wrongExecutable); err == nil || !strings.Contains(err.Error(), "reported executable names do not match their identity") {
		t.Fatalf("different executable accepted or misreported: %v", err)
	}
}

func TestDashboardIdentityAcceptsCanonicalAppSiblingBinaries(t *testing.T) {
	makeApp := func(name string) (string, string) {
		t.Helper()
		macOSDir := filepath.Join(t.TempDir(), name+".app", "Contents", "MacOS")
		if err := os.MkdirAll(macOSDir, 0o700); err != nil {
			t.Fatal(err)
		}
		cliPath := filepath.Join(macOSDir, "sirsi")
		menubarPath := filepath.Join(macOSDir, "sirsi-menubar")
		for _, path := range []string{cliPath, menubarPath} {
			if err := os.WriteFile(path, []byte(filepath.Base(path)), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		return cliPath, menubarPath
	}

	cliPath, menubarPath := makeApp("Pantheon")
	self := buildversion.Info{Binary: "sirsi", Version: "1.2.3", Commit: "abc123", Date: "2026-09-23", Path: cliPath}
	running := dashboard.IdentityResponse{Schema: dashboard.IdentitySchema, Info: buildversion.Info{
		Binary: "sirsi-menubar", Version: self.Version, Commit: self.Commit, Date: self.Date, Path: menubarPath,
	}}
	if err := validateDashboardIdentityMatchesSelf(self, running); err != nil {
		t.Fatalf("same-app sibling executable/build rejected: %v", err)
	}

	_, otherMenubarPath := makeApp("OtherPantheon")
	running.Path = otherMenubarPath
	if err := validateDashboardIdentityMatchesSelf(self, running); err == nil || !strings.Contains(err.Error(), "same Pantheon app") {
		t.Fatalf("sibling from a different app bundle accepted or misreported: %v", err)
	}
}

func TestFetchDashboardIdentityRejectsWrongSchema(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema":"legacy-dashboard/v1","version":"0.23.9-beta","commit":"dd0e701b"}`))
	}))
	defer srv.Close()

	if _, err := fetchDashboardIdentity(srv.Client(), srv.URL); err == nil {
		t.Fatal("wrong identity schema was accepted")
	}
}

func TestFetchDashboardIdentityDiagnosesLegacyListener(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	if _, err := fetchDashboardIdentity(srv.Client(), srv.URL); err == nil || !strings.Contains(err.Error(), "listener may be an older dashboard") {
		t.Fatalf("legacy dashboard response = %v, want actionable older-dashboard diagnosis", err)
	}
}

func TestFetchDashboardIdentityRejectsAmbiguousAndOversizedJSON(t *testing.T) {
	valid := `{"schema":"pantheon.dashboard-identity/v1","binary":"sirsi","version":"0.23.9-beta","commit":"dd0e701b"}`
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "duplicate schema", body: `{"schema":"pantheon.dashboard-identity/v1","schema":"legacy","version":"0.23.9-beta","commit":"dd0e701b"}`},
		{name: "trailing value", body: valid + ` {}`},
		{name: "unknown field", body: `{"schema":"pantheon.dashboard-identity/v1","version":"0.23.9-beta","commit":"dd0e701b","unexpected":true}`},
		{name: "oversized", body: valid + strings.Repeat(" ", dashboardIdentityBodyLimit)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			if _, err := fetchDashboardIdentity(srv.Client(), srv.URL); err == nil {
				t.Fatal("ambiguous or oversized identity response was accepted")
			}
		})
	}
}

func TestFetchDashboardIdentityDoesNotFollowRedirects(t *testing.T) {
	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetRequests.Add(1)
		_, _ = w.Write([]byte(`{"schema":"pantheon.dashboard-identity/v1","version":"0.23.9-beta","commit":"dd0e701b"}`))
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()

	if _, err := fetchDashboardIdentity(redirect.Client(), redirect.URL); err == nil {
		t.Fatal("redirected identity response was accepted")
	}
	if got := targetRequests.Load(); got != 0 {
		t.Fatalf("redirect target received %d requests, want none", got)
	}
}
