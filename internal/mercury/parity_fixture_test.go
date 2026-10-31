package mercury

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// This is the actual bounded Go reader projection, with injected inert HTTP,
// for the native/web wire fixture. No hand-authored projected JSON is exported.
// Main may supply one fresh absolute output; absent env still runs assertions.
func TestExportMixedFreshnessParityFixture(t *testing.T) {
	at := time.Unix(1792339200, 0)
	d := dashboardForFreshness(t)
	// Fix all timestamps to the same deterministic observation, then expire
	// exactly one direction. Its healthy reverse must survive the partial read.
	stamp := float64(at.Unix())
	d.GeneratedAt = stamp
	for i := range d.Nodes {
		d.Nodes[i].ObservedAt = &stamp
	}
	for i := range d.Links {
		for j := range d.Links[i].Directions {
			d.Links[i].Directions[j].ObservedAt = &stamp
		}
	}
	stale := stamp - 7
	d.Links[0].Directions[0].ObservedAt = &stale
	r := readDashboardAt(t, d, at, at)
	if r.State != "partial" || r.Total != nil || r.Incoming != nil || r.Headline.Wire.Number != "—" {
		t.Fatalf("incomplete aggregate promoted: %+v", r)
	}
	var mixed *SceneCable
	for i := range r.Scene.Cables {
		if r.Scene.Cables[i].ID == d.Links[0].ID {
			mixed = &r.Scene.Cables[i]
		}
	}
	if mixed == nil || mixed.Rate.Number != "—" || mixed.Directions[0].Rate != "— MB/s" || mixed.Directions[0].Speed != 0 || mixed.Directions[1].Rate == "— MB/s" || mixed.Directions[1].Speed <= 0 {
		t.Fatalf("mixed directional eligibility lost: %+v", mixed)
	}
	path := os.Getenv("PANTHEON_TEST_MERCURY_MIXED_JSON")
	if path == "" {
		return
	}
	if !filepath.IsAbs(path) {
		t.Fatal("fixture output must be absolute")
	}
	data, err := json.Marshal(r)
	if err != nil || len(data) > 1024*1024 {
		t.Fatalf("fixture encoding: %v bytes=%d", err, len(data))
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	n, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || n != len(data) || closeErr != nil {
		t.Fatalf("fixture complete write: n=%d want=%d write=%v close=%v", n, len(data), writeErr, closeErr)
	}
}
