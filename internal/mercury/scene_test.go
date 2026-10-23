package mercury

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestSceneKeepsIdentityAndCardEdges(t *testing.T) {
	d, err := Decode(fixtureBodies["live"])
	if err != nil {
		t.Fatal(err)
	}
	freshness := fixtureFreshness(d)
	h := projectHeadline(d, freshness)
	s := projectScene(d, h.NodeOrder, freshness)
	if len(s.Cards) != 3 || len(s.Cables) != len(d.Links) {
		t.Fatalf("%+v", s)
	}
	cards := map[string]SceneCard{}
	for _, c := range s.Cards {
		cards[c.ID] = c
	}
	if s.Cards[0].ID != "M5" || s.Cards[1].ID != "M5-Studio" || s.Cards[2].ID != "M1" {
		t.Fatal(s.Cards)
	}
	lanes := map[float64]bool{}
	moving := 0
	for i, c := range s.Cables {
		if c.Letter != cableLetter(i) || (i > 0 && s.Cables[i-1].ID >= c.ID) {
			t.Fatal(c)
		}
		var link DashLink
		for _, l := range d.Links {
			if l.ID == c.ID {
				link = l
			}
		}
		a := cards[link.Ends[0].Node]
		if link.Ends[1] == nil {
			if c.X1 != a.X+a.Width || c.X2-c.X1 != 32 || c.State != "missing" || c.Iface == "" {
				t.Fatal(c)
			}
			if c.Rate.Number != "—" || len(c.Directions) != 0 {
				t.Fatal("missing peer invented rate", c)
			}
			continue
		}
		b := cards[link.Ends[1].Node]
		if a.X > b.X {
			a, b = b, a
		}
		if c.X1 != a.X+a.Width || c.X2 != b.X {
			t.Fatal("not facing edges", c)
		}
		if a.ID == "M5" && b.ID == "M5-Studio" {
			if lanes[c.Lane] {
				t.Fatal("parallel tracks overlap", c)
			}
			lanes[c.Lane] = true
		}
		for _, f := range c.Directions {
			if f.Speed > 0 {
				moving++
			}
			if f.Speed < 0 || f.Speed > 600 {
				t.Fatal(f)
			}
		}
	}
	for a := range lanes {
		for b := range lanes {
			if a != b && a-b < 26 && b-a < 26 {
				t.Fatal("cable badges overlap", lanes)
			}
		}
	}
	if len(lanes) != 3 || moving == 0 {
		t.Fatalf("lanes=%v moving=%d", lanes, moving)
	}
	staleFreshness := assessFreshness(d, time.Unix(1792339210, 0))
	for _, c := range projectScene(d, projectHeadline(d, staleFreshness).NodeOrder, staleFreshness).Cables {
		for _, f := range c.Directions {
			if f.Speed != 0 {
				t.Fatal("stale motion", f)
			}
		}
	}
	// Quiet interface traffic below the headline threshold still moves cables.
	// Sub-1KB/s observations must remain motionless.
	tiny := 0.0000009
	for i := range d.Links {
		for j := range d.Links[i].Directions {
			d.Links[i].Directions[j].WireGBs = &tiny
		}
	}
	freshness = fixtureFreshness(d)
	for _, c := range projectScene(d, projectHeadline(d, freshness).NodeOrder, freshness).Cables {
		for _, f := range c.Directions {
			if f.Speed != 0 {
				t.Fatal("sub-threshold motion", f)
			}
		}
	}
	// Each peer must itself be fresh, even if the serving node is live.
	for i := range d.Nodes {
		d.Nodes[i].State = "stale"
	}
	freshness = fixtureFreshness(d)
	for _, c := range projectScene(d, projectHeadline(d, freshness).NodeOrder, freshness).Cables {
		for _, f := range c.Directions {
			if f.Speed != 0 {
				t.Fatal("stale peer motion", f)
			}
		}
	}
}
func TestSceneUnknownFamiliesAndRates(t *testing.T) {
	unknown := "unrecognized future Mac"
	if family(nil) != "generic" || family(&unknown) != "generic" {
		t.Fatal("invented family")
	}
	for m, want := range map[string]string{"MacBook Pro": "laptop", "Mac Studio": "studio", "Mac16,9": "studio", "Mac17,2": "laptop", "Mac mini": "mini", "iMac": "imac", "Mac Pro": "tower"} {
		if family(&m) != want {
			t.Fatal(m)
		}
	}
	known := "Mac17,2"
	for _, tc := range []struct {
		value *string
		want  string
	}{
		{nil, "laptop"}, {ptrString("studio"), "studio"}, {ptrString("desktop"), "desktop"},
		{ptrString("unknown"), "generic"}, {ptrString("future-family"), "generic"},
	} {
		if got := nodeFamily(DashNode{Model: &known, Family: tc.value}); got != tc.want {
			t.Fatalf("family precedence %v: %s", tc.value, got)
		}
	}
	if cableLetter(25) != "Z" || cableLetter(26) != "AA" {
		t.Fatal("cable labels")
	}
	d, _ := Decode(fixtureBodies["live"])
	for i := range d.Links {
		for j := range d.Links[i].Directions {
			d.Links[i].Directions[j].WireGBs = nil
			d.Links[i].Directions[j].PayloadGBs = nil
		}
	}
	freshness := fixtureFreshness(d)
	for _, c := range projectScene(d, projectHeadline(d, freshness).NodeOrder, freshness).Cables {
		if c.Rate.Number != "—" {
			t.Fatal("unknown became zero", c)
		}
		for _, f := range c.Directions {
			if f.Speed != 0 || f.Rate != "— MB/s" {
				t.Fatal(f)
			}
		}
	}
}

// An explicit output path creates a historical fixture from the actual shared
// reader for native/web render QA. No network request: fixtureClient is injected.
func TestExportRenderFixture(t *testing.T) {
	path := os.Getenv("PANTHEON_MERCURY_SNAPSHOT_JSON")
	if path == "" {
		t.Skip("no explicit fixture output")
	}
	c := fixtureClient(fixtureBodies["live"])
	r := c.Read(context.Background())
	if r.Scene == nil || r.Headline.Wire.Number != "28.2" {
		t.Fatalf("%+v", r)
	}
	r.Message = "Historical producer fixture · live"
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func ptrString(v string) *string { return &v }
