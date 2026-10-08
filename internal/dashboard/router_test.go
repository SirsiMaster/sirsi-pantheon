package dashboard

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// TestRouterSurfaceActiveLaneCardReadsRealVerdictKey guards the 2026-10-08
// defect: the embedded page JS read counts.active, a key that never existed
// in the real producer's Counts map (keyed by the router.Verdict* strings,
// e.g. "LIVE") — the card silently rendered 0 regardless of fleet state.
func TestRouterSurfaceActiveLaneCardReadsRealVerdictKey(t *testing.T) {
	t.Parallel()
	want := "c." + router.VerdictLive
	if !strings.Contains(routerSurfaceHTML, want) {
		t.Fatalf("routerSurfaceHTML must read %q (the real Counts key); not found", want)
	}
	if strings.Contains(routerSurfaceHTML, "c.active") {
		t.Fatalf("routerSurfaceHTML still reads the invented c.active key")
	}
}

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

func TestRouterSurfaceUsesTheHorusProducer(t *testing.T) {
	t.Parallel()
	want := RouterSnapshot{Version: "v-surface", GeneratedAt: "2026-10-08T12:00:00Z", Lanes: RouterLanes{Counts: map[string]int{"active": 2}}}
	ts := testServer(t, Config{RouterFn: func() (RouterSnapshot, error) { return want, nil }})
	defer ts.Close()

	manifest, err := http.Get(ts.URL + "/api/router/v1/manifest")
	if err != nil {
		t.Fatal(err)
	}
	defer manifest.Body.Close()
	if manifest.StatusCode != http.StatusOK || manifest.Header.Get("X-Sirsi-Router-Schema") != "router-surface.v1" {
		t.Fatalf("manifest = %d schema=%q", manifest.StatusCode, manifest.Header.Get("X-Sirsi-Router-Schema"))
	}
	var m map[string]any
	if err := json.NewDecoder(manifest.Body).Decode(&m); err != nil || m["authority"] != "ra" {
		t.Fatalf("manifest = %+v err=%v", m, err)
	}
	build, ok := m["build"].(map[string]any)
	if !ok || build["version"] == nil || build["commit"] == nil {
		t.Fatalf("manifest missing producer build identity: %+v", m)
	}

	snapshot, err := http.Get(ts.URL + "/api/router/v1/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Body.Close()
	var got RouterSnapshot
	if snapshot.StatusCode != http.StatusOK || json.NewDecoder(snapshot.Body).Decode(&got) != nil || got.Version != want.Version {
		t.Fatalf("snapshot = %d %+v", snapshot.StatusCode, got)
	}

	page, err := http.Get(ts.URL + "/router")
	if err != nil {
		t.Fatal(err)
	}
	defer page.Body.Close()
	if page.StatusCode != http.StatusOK {
		t.Fatalf("router page = %d", page.StatusCode)
	}
}

func TestRouterSurfaceFailsClosedWhenProducerIsMissing(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{})
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/router/v1/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("snapshot without producer = %d, want 503", resp.StatusCode)
	}
}

// ADR-077: the versioned surface's failure status is uniformly 503 (service
// unavailable, retry later) whether the producer was never wired or errored
// on this call — unlike the older /api/router contract (502 for a failing
// producer), which the v1 surface does not inherit.
func TestRouterSurfaceSnapshotIs503WhenProducerErrors(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{RouterFn: func() (RouterSnapshot, error) { return RouterSnapshot{}, errors.New("boom") }})
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/router/v1/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("snapshot with failing producer = %d, want 503", resp.StatusCode)
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
	for _, want := range []string{"/api/router", "/api/fleet", "/api/stats", "Operator attention"} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js missing %q", want)
		}
	}
	css, _, code := body("/assets/tokens.css")
	if code != 200 || !strings.Contains(css, "--emerald:") || !strings.Contains(css, "prefers-color-scheme: dark") {
		t.Fatalf("tokens.css not derived from the brand palette: %d %q", code, css)
	}
	if _, _, code2 := body("/assets/nope.css"); code2 != 404 {
		t.Fatalf("unknown asset = %d, want 404", code2)
	}
	if _, _, code3 := body("/missing"); code3 != 404 {
		t.Fatalf("unknown path = %d, want 404 (the home handler must not swallow every route)", code3)
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
