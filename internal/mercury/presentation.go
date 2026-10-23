package mercury

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Headline follows Mercury edb9046a DASHBOARD-RENDER-MAPPING-v1. Surfaces
// render these values; they do not independently infer activity or gain.
type Headline struct {
	Wire       Cell     `json:"wire"`
	Compressed Cell     `json:"compressed"`
	Gain       Cell     `json:"gain"`
	Context    string   `json:"context"`
	Lamps      []Lamp   `json:"lamps"`
	Cables     int      `json:"cables"`
	NodeOrder  []string `json:"node_order"`
	RunStatus  string   `json:"run_status"`
}
type Cell struct {
	Number string `json:"number"`
	Unit   string `json:"unit"`
	Note   string `json:"note"`
}
type Lamp struct {
	Label string `json:"label"`
	On    bool   `json:"on"`
}

func rateCell(value float64) Cell {
	mb := value * 1000
	switch {
	case mb >= 10000:
		return Cell{Number: fmt.Sprintf("%.1f", value), Unit: "GB/s"}
	case mb >= 1000:
		return Cell{Number: fmt.Sprintf("%.2f", value), Unit: "GB/s"}
	case mb >= 10:
		return Cell{Number: fmt.Sprintf("%.0f", mb), Unit: "MB/s"}
	default:
		return Cell{Number: fmt.Sprintf("%.1f", mb), Unit: "MB/s"}
	}
}
func (c Cell) text() string { return strings.TrimSpace(c.Number + " " + c.Unit) }
func clockText(seconds float64) string {
	n := int(math.Round(math.Max(0, seconds)))
	return fmt.Sprintf("%d:%02d", n/60, n%60)
}
func runText(r DashRun) string {
	if !r.Configured {
		return ""
	}
	path := ""
	if r.Leg > 0 && r.Leg <= len(r.Legs) {
		path = strings.Join(r.Legs[r.Leg-1].Path, " → ")
	}
	where := fmt.Sprintf("Leg %d of %d · %s", r.Leg, len(r.Legs), path)
	switch r.Phase {
	case "running", "queued":
		s := fmt.Sprintf("%s · %d cables · %s / %s", where, len(r.Live), clockText(r.Elapsed), clockText(float64(r.Seconds)))
		if r.Note != "" {
			s += " · " + r.Note
		}
		return s
	case "done":
		var parts []string
		for _, v := range r.Results {
			recoveries := 0
			for _, c := range v.Cables {
				recoveries += c.Recoveries
			}
			s := fmt.Sprintf("%s: %.2f GB/s · %d cables · %d recoveries", strings.ReplaceAll(v.Path, " -> ", " → "), v.Aggregate, len(v.Cables), recoveries)
			if strings.Contains(v.Verdict, "FAIL") {
				s += " · not quiet-certified"
			}
			parts = append(parts, s)
		}
		return "Done · " + strings.Join(parts, " | ")
	case "stopped":
		return "Stopped · " + where
	case "stop-failed":
		return "Stop not confirmed · " + r.Error + " · retrying"
	case "failed":
		if r.Error == "" {
			return "Failed · unknown error"
		}
		return "Failed · " + r.Error
	case "idle":
		return ""
	default:
		return r.Phase
	}
}
func projectHeadline(d Dashboard, freshness freshnessAssessment) *Headline {
	unknown := Cell{Number: "—"}
	h := &Headline{Wire: unknown, Compressed: unknown, Gain: unknown, RunStatus: runText(d.Run)}
	degrees := map[string]int{}
	sourceLive := freshness.nodeEligible(d.Source.Host)
	decode := ""
	for _, n := range d.Nodes {
		degrees[n.ID] = 0
		if n.ID == d.Source.Host {
			if n.Decode.Mode != nil {
				decode = *n.Decode.Mode
			}
		}
	}
	up := 0
	maxWire := 0.0
	for _, l := range d.Links {
		for _, e := range l.Ends {
			if e != nil {
				degrees[e.Node]++
			}
		}
		if l.State == "up" {
			up++
		}
		if l.Ends[1] != nil && l.State == "up" {
			for _, v := range l.Directions {
				if v.WireGBs != nil {
					maxWire = math.Max(maxWire, *v.WireGBs)
				}
			}
		}
	}
	var ids []string
	for _, n := range d.Nodes {
		ids = append(ids, n.ID)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := ids[i], ids[j]
		if a == b {
			return false
		}
		if degrees[a] != degrees[b] {
			return degrees[a] > degrees[b]
		}
		if a == d.Source.Host {
			return true
		}
		if b == d.Source.Host {
			return false
		}
		return a > b
	})
	if len(ids) > 0 {
		center := ids[0]
		others := append([]string(nil), ids[1:]...)
		sort.Sort(sort.Reverse(sort.StringSlice(others)))
		var left, right []string
		for i, id := range others {
			if i%2 == 0 {
				left = append([]string{id}, left...)
			} else {
				right = append(right, id)
			}
		}
		h.NodeOrder = append(append(left, center), right...)
	}
	if !freshness.envelopeFresh || !sourceLive {
		h.Lamps = []Lamp{{"TCP", false}, {"RDMA", false}, {"MERCURY", false}, {"CPU DECODE", false}, {"GPU DECODE", false}}
		h.Context = "Rates paused · the samples are not fresh"
		return h
	}
	if !freshness.completeForHeadline(d) {
		h.Lamps = []Lamp{{"TCP", false}, {"RDMA", false}, {"MERCURY", false}, {"CPU DECODE", false}, {"GPU DECODE", false}}
		h.Context = "Rates unavailable · one or more link samples are not fresh"
		return h
	}
	wire, payload := 0.0, 0.0
	published, rdma, tcp := false, false, false
	directions := map[string]int{}
	for linkIndex, l := range d.Links {
		if l.Ends[1] == nil || l.State != "up" {
			continue
		}
		active := false
		for directionIndex, v := range l.Directions {
			if !freshness.directionEligible(linkIndex, directionIndex) {
				continue
			}
			if v.WireGBs == nil || *v.WireGBs < 0.05 || *v.WireGBs < maxWire*0.01 {
				continue
			}
			active = true
			wire += *v.WireGBs
			p := *v.WireGBs
			if v.PayloadGBs != nil {
				p = *v.PayloadGBs
				published = true
			}
			payload += p
			directions[v.From+" → "+v.To]++
			if v.Provenance != nil {
				switch *v.Provenance {
				case "rdma-publisher":
					rdma = true
				case "tcp-publisher", "interface-counters":
					tcp = true
				}
			}
		}
		if active {
			h.Cables++
		}
	}
	if wire == 0 {
		if d.Run.Phase == "running" {
			h.Cables = len(d.Run.Live)
			h.Lamps = []Lamp{{"TCP", d.Run.Transport == "tcp"}, {"RDMA", d.Run.Transport == "rdma"}, {"MERCURY", d.Run.Engine == "mercury"}, {"CPU DECODE", d.Run.Decode == "cpu"}, {"GPU DECODE", d.Run.Decode == "gpu"}}
		}
		h.Context = fmt.Sprintf("Idle · nothing moving on %d Thunderbolt links", up)
		return h
	}
	var names []string
	for name := range directions {
		names = append(names, name)
	}
	sort.Strings(names)
	h.Context = strings.Join(names, " · ")
	h.Wire = rateCell(wire)
	compressed := published && payload > 1.005*wire
	if compressed {
		h.Compressed = rateCell(payload)
		h.Compressed.Note = "Delivered · verified"
		h.Gain = Cell{Number: fmt.Sprintf("%.2f", payload/wire), Unit: "×", Note: "+" + rateCell(payload-wire).text() + " over the wire"}
	}
	if len(directions) == 1 && h.Cables > 1 {
		h.Wire.Note = fmt.Sprintf("TOTAL · %d × %s", h.Cables, rateCell(wire/float64(h.Cables)).text())
		if compressed {
			h.Compressed.Note = "TOTAL · " + rateCell(payload/float64(h.Cables)).text() + " per cable\nDelivered · verified"
		}
	}
	mercury, cpu, gpu := compressed, decode == "cpu", decode == "gpu"
	if d.Run.Phase == "running" {
		rdma = d.Run.Transport == "rdma"
		tcp = d.Run.Transport == "tcp"
		mercury = d.Run.Engine == "mercury"
		cpu = d.Run.Decode == "cpu"
		gpu = d.Run.Decode == "gpu"
		h.Cables = len(d.Run.Live)
	} else {
		tcp = tcp || !rdma
	}
	h.Lamps = []Lamp{{"TCP", tcp}, {"RDMA", rdma}, {"MERCURY", mercury}, {"CPU DECODE", cpu}, {"GPU DECODE", gpu}}
	return h
}
