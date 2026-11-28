package stacklab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testRev = "0123456789abcdef0123456789abcdef01234567"
	testSum = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

// fakeHub serves the hub's exact-revision contract: one snapshot, 404 for any other rev.
// answerRev lets a test play a hub that ignores ?rev= (an old hub).
func fakeHub(t *testing.T, answerRev string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/hub/v1/models/mlx-community/Qwen3.8-27B-4bit" {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		rev := r.URL.Query().Get("rev")
		if answerRev == "" && rev != testRev {
			http.Error(w, `{"error":"not_found","message":"not in the library"}`, http.StatusNotFound)
			return
		}
		if answerRev != "" {
			rev = answerRev
		}
		json.NewEncoder(w).Encode(HubModel{Repo: "mlx-community/Qwen3.8-27B-4bit", Revision: rev,
			Files: []HubFile{{Name: "config.json", Size: 10, SHA256: testSum}}})
	}))
}

func TestHubModelResolve(t *testing.T) {
	ctx := context.Background()
	hub := fakeHub(t, "")
	defer hub.Close()
	c := &HubClient{URLs: []string{hub.URL}, Token: "tok", HTTP: hub.Client()}
	known := "hub:mlx-community/Qwen3.8-27B-4bit@" + testRev

	m, err := c.Resolve(ctx, known)
	if err != nil || m.Revision != testRev || len(m.Files) != 1 || m.Source != hub.URL {
		t.Fatalf("known rev: %+v %v", m, err)
	}

	// A dead first URL is skipped (transport failure), the live hub answers.
	dead := httptest.NewServer(nil)
	deadURL := dead.URL
	dead.Close()
	c2 := &HubClient{URLs: []string{deadURL, hub.URL}, Token: "tok", HTTP: hub.Client()}
	if _, err := c2.Resolve(ctx, known); err != nil {
		t.Fatalf("failover: %v", err)
	}

	refused := map[string]struct {
		c   *HubClient
		ref string
	}{
		"unknown rev":     {c, "hub:mlx-community/Qwen3.8-27B-4bit@" + strings.Repeat("f", 40)},
		"unknown repo":    {c, "hub:mlx-community/nope@" + testRev},
		"unpinned":        {c, "hub:mlx-community/Qwen3.8-27B-4bit"},
		"not hub":         {c, "hf:mlx-community/Qwen3.8-27B-4bit@" + testRev},
		"traversal":       {c, "hub:../x@" + testRev},
		"bad token":       {&HubClient{URLs: []string{hub.URL}, Token: "bad", HTTP: hub.Client()}, known},
		"hub unreachable": {&HubClient{URLs: []string{deadURL}, Token: "tok", HTTP: hub.Client()}, known},
		"no account":      {nil, known},
	}
	for name, tc := range refused {
		if _, err := AdmitModelIngredients(ctx, tc.c, []string{known, tc.ref}); err == nil {
			t.Errorf("%s: admitted, want refusal", name)
		}
	}

	// An old hub that ignores ?rev= and answers another revision is refused.
	old := fakeHub(t, strings.Repeat("e", 40))
	defer old.Close()
	if _, err := (&HubClient{URLs: []string{old.URL}, Token: "tok", HTTP: old.Client()}).Resolve(ctx, known); err == nil || !strings.Contains(err.Error(), "not the pinned") {
		t.Fatalf("mismatched rev: %v", err)
	}
}
