package dashboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// An unwired producer is a 503 (never an empty panel), a failing one a 502, and a
// working one is served verbatim (all three directions).
func TestAPIRouterNilFailingAndWired(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{})
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/router")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unwired = %d, want 503", resp.StatusCode)
	}

	bad := testServer(t, Config{RouterFn: func() (RouterSnapshot, error) { return RouterSnapshot{}, errors.New("boom") }})
	defer bad.Close()
	resp, _ = http.Get(bad.URL + "/api/router")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("failing producer = %d, want 502", resp.StatusCode)
	}

	want := RouterSnapshot{Version: "v9", Lanes: RouterLanes{Counts: map[string]int{"WAKEABLE": 2}}, KnownFailures: []RouterKnownFail{{ID: "x", Status: "resolved", FixedIn: "0.24.68"}}}
	ok := testServer(t, Config{RouterFn: func() (RouterSnapshot, error) { return want, nil }})
	defer ok.Close()
	resp, err = http.Get(ok.URL + "/api/router")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got RouterSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil || got.Version != "v9" || got.Lanes.Counts["WAKEABLE"] != 2 || got.KnownFailures[0].FixedIn != "0.24.68" {
		t.Fatalf("wired snapshot not served verbatim: %+v err=%v", got, err)
	}
}

// The page must carry the Router view and its nav entry: a panel with no way to
// reach it would ship invisible.
func TestRouterViewIsReachableFromTheSPA(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{})
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b := new(strings.Builder)
	buf := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(buf)
		b.Write(buf[:n])
		if rerr != nil {
			break
		}
	}
	page := b.String()
	for _, want := range []string{`data-view="router"`, "function viewRouter", "router:viewRouter", "/api/router", "loading the fleet board"} {
		if !strings.Contains(page, want) {
			t.Errorf("SPA missing %q", want)
		}
	}
}
