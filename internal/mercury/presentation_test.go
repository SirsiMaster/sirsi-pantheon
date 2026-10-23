package mercury

import (
	"reflect"
	"testing"
)

func TestApprovedHeadlineProjection(t *testing.T) {
	d, err := Decode(fixtureBodies["live"])
	if err != nil {
		t.Fatal(err)
	}
	h := Project(d, false)
	if h.Wire.Number != "28.2" || h.Compressed.Number != "42.4" || h.Gain.Number != "1.51" {
		t.Fatalf("%+v", h)
	}
	if h.Cables != 3 || h.Context != "M5 → M5-Studio" || !reflect.DeepEqual(h.NodeOrder, []string{"M5", "M5-Studio", "M1"}) {
		t.Fatalf("%+v", h)
	}
	if h.Wire.Note != "TOTAL · 3 × 9.38 GB/s" {
		t.Fatal(h.Wire.Note)
	}
	for _, name := range []string{"idle", "offline-peer"} {
		d, _ = Decode(fixtureBodies[name])
		h = Project(d, false)
		if h.Wire.Number != "—" || h.Compressed.Number != "—" || h.Gain.Number != "—" {
			t.Fatalf("%s %+v", name, h)
		}
	}
	h = Project(d, true)
	if h.Wire.Number != "—" || h.Context != "Rates paused · the samples are not fresh" {
		t.Fatal(h)
	}
	d, _ = Decode(fixtureBodies["run-in-progress"])
	h = Project(d, false)
	if h.RunStatus != "Leg 1 of 1 · M5 → M5-Studio · 3 cables · 0:22 / 1:00 · leg 1 of 1" {
		t.Fatal(h.RunStatus)
	}
}
func TestApprovedRateUnits(t *testing.T) {
	for _, tc := range []struct {
		v    float64
		n, u string
	}{{42.4, "42.4", "GB/s"}, {9.384, "9.38", "GB/s"}, {0.05, "50", "MB/s"}, {0.0001, "0.1", "MB/s"}} {
		c := rateCell(tc.v)
		if c.Number != tc.n || c.Unit != tc.u {
			t.Fatal(c)
		}
	}
}

func TestQuietRunningJobStillHasAuthoritativeRunLamps(t *testing.T) {
	d, _ := Decode(fixtureBodies["idle"])
	d.Run.Phase = "running"
	d.Run.Transport = "rdma"
	d.Run.Engine = "mercury"
	d.Run.Decode = "gpu"
	d.Run.Live = []Cable{{N: 1}, {N: 2}}
	h := Project(d, false)
	if h.Wire.Number != "—" || h.Cables != 2 || len(h.Lamps) != 5 || !h.Lamps[1].On || !h.Lamps[4].On {
		t.Fatalf("%+v", h)
	}
}
