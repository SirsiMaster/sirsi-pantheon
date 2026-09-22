package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	buildversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

func TestIdentityEndpointReportsExactBuildIdentity(t *testing.T) {
	want := buildversion.Info{Binary: "sirsi", Version: "0.23.9-beta", Commit: "dd0e701b", Date: "2026-09-22T19:00:00Z", Path: "/tmp/sirsi", GoVer: "go1.test"}
	srv := New(Config{BuildIdentity: want})
	ts := testServer(t, srv.cfg)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/identity")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/identity = %d", resp.StatusCode)
	}
	var got IdentityResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Schema != IdentitySchema {
		t.Fatalf("schema = %q, want %q", got.Schema, IdentitySchema)
	}
	if got.Info != want {
		t.Fatalf("identity = %+v, want %+v", got.Info, want)
	}
}

func TestIdentityEndpointIsReadOnly(t *testing.T) {
	srv := New(Config{})
	req := httptest.NewRequest(http.MethodPost, "/api/identity", nil)
	rec := httptest.NewRecorder()
	srv.apiIdentity(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/identity = %d, want 405", rec.Code)
	}
}
