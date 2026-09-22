package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestFetchDashboardIdentityRejectsWrongSchema(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema":"legacy-dashboard/v1","version":"0.23.9-beta","commit":"dd0e701b"}`))
	}))
	defer srv.Close()

	if _, err := fetchDashboardIdentity(srv.Client(), srv.URL); err == nil {
		t.Fatal("wrong identity schema was accepted")
	}
}
