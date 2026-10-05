package dashboard

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
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

// The home page is the new dashboard: it loads the brand-token stylesheet and the app,
// and the app reads the router API. A page that cannot reach its data ships blank.
func TestHomeServesTheDashboardAndItsAssets(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{})
	defer ts.Close()
	body := func(path string) (string, string, int) {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b), resp.Header.Get("Content-Type"), resp.StatusCode
	}
	home, ct, code := body("/")
	if code != 200 || !strings.Contains(ct, "text/html") || !strings.Contains(home, "/assets/tokens.css") || !strings.Contains(home, "/assets/app.js") {
		t.Fatalf("home page wrong: %d %s", code, ct)
	}
	js, _, _ := body("/assets/app.js")
	for _, want := range []string{"/api/router", "/api/fleet", "/api/stats", "Needs attention"} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js missing %q", want)
		}
	}
	css, _, code := body("/assets/tokens.css")
	if code != 200 || !strings.Contains(css, "--emerald:") || !strings.Contains(css, "prefers-color-scheme: dark") {
		t.Fatalf("tokens.css not derived from the brand palette: %d %q", code, css)
	}
	if _, _, code := body("/assets/nope.css"); code != 404 {
		t.Fatalf("unknown asset = %d, want 404", code)
	}
	if _, _, code := body("/missing"); code != 404 {
		t.Fatalf("unknown path = %d, want 404 (the home handler must not swallow every route)", code)
	}
	classic, _, code := body("/classic")
	if code != 200 || !strings.Contains(classic, "Horus") {
		t.Fatalf("classic tools page missing: %d", code)
	}
}

// ADR-038: every color comes from internal/brand. A hex literal in the UI files is
// a surface that can drift from the CLI, menubar and Swift app.
func TestUIFilesCarryNoHexColors(t *testing.T) {
	t.Parallel()
	re := regexp.MustCompile(`#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{3}\b`)
	for _, f := range []string{"ui/index.html", "ui/app.css", "ui/app.js"} {
		b, err := uiFS.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllString(string(b), -1) {
			if m == "#main" || m == "#nav" || m == "#title" || m == "#view" || m == "#live" || m == "#refresh" || m == "#foot" {
				continue
			}
			t.Errorf("%s contains a hex color literal %q: use a brand token", f, m)
		}
	}
}
