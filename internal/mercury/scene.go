package mercury

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Scene is shared presentation geometry, not physical port geometry. Cables end
// at facing card edges. Consumers scale this coordinate space without deriving
// topology, hardware families, rate formatting or animation speeds themselves.
type Scene struct {
	Width  float64      `json:"width"`
	Height float64      `json:"height"`
	Cards  []SceneCard  `json:"cards"`
	Cables []SceneCable `json:"cables"`
}
type SceneCard struct {
	ID     string  `json:"id"`
	Family string  `json:"family"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}
type SceneCable struct {
	ID     string `json:"id"`
	Letter string `json:"letter"`
	State  string `json:"state"`
	Iface  string `json:"iface"`
	// X1/Y1 and X2/Y2 are card edges, not plugs or case-port locations.
	X1         float64          `json:"x1"`
	Y1         float64          `json:"y1"`
	X2         float64          `json:"x2"`
	Y2         float64          `json:"y2"`
	Lane       float64          `json:"lane"`
	Rate       Cell             `json:"rate"`
	Directions []SceneDirection `json:"directions"`
}
type SceneDirection struct {
	From    string  `json:"from"`
	To      string  `json:"to"`
	Rate    string  `json:"rate"`
	Color   string  `json:"color"`
	Speed   float64 `json:"speed"`
	Forward bool    `json:"forward"`
}

// Producer decision 20261018-204657 supplies the current opaque model table
// and optional family contract; absent remains valid, unknown stays generic.
func nodeFamily(n DashNode) string {
	if n.Family != nil {
		switch *n.Family {
		case "laptop", "studio", "desktop", "mini":
			return *n.Family
		default:
			return "generic"
		}
	}
	return family(n.Model)
}
func family(model *string) string {
	if model == nil {
		return "generic"
	}
	m := strings.ToLower(*model)
	// Producer kindOf(model) fallback, e99b021c. v1 omits its richer
	// hardware-profile family. Unknown identifiers stay generic.
	switch m {
	case "mac13,1", "mac13,2", "mac14,13", "mac14,14", "mac15,14", "mac16,9", "mac17,14":
		return "studio"
	case "mac14,8":
		return "tower"
	case "mac17,2", "mac14,6", "mac14,10", "mac15,7", "mac15,9", "mac15,11", "mac16,5", "mac16,7", "mac17,6", "mac17,8":
		return "laptop"
	}
	for _, f := range []struct{ match, name string }{
		{"macbook", "laptop"}, {"studio", "studio"}, {"mini", "mini"}, {"imac", "imac"}, {"mac pro", "tower"}, {"macpro", "tower"},
	} {
		if strings.Contains(m, f.match) {
			return f.name
		}
	}
	return "generic"
}
func cableLetter(n int) string {
	var s string
	for n++; n > 0; n = (n - 1) / 26 {
		s = string(rune('A'+(n-1)%26)) + s
	}
	return s
}
func projectScene(d Dashboard, order []string, freshness freshnessAssessment) *Scene {
	const width, gap, cardWidth, cardY, cardHeight = 1120.0, 104.0, 280.0, 180.0, 300.0
	scene := &Scene{Width: math.Max(width, float64(len(order))*(cardWidth+gap)-gap+72), Height: 540, Cards: []SceneCard{}, Cables: []SceneCable{}}
	cards := map[string]SceneCard{}
	live := map[string]bool{}
	start := (scene.Width - (float64(len(order))*(cardWidth+gap) - gap)) / 2
	for i, id := range order {
		for _, node := range d.Nodes {
			if node.ID != id {
				continue
			}
			card := SceneCard{ID: id, Family: nodeFamily(node), X: start + float64(i)*(cardWidth+gap), Y: cardY, Width: cardWidth, Height: cardHeight}
			scene.Cards = append(scene.Cards, card)
			cards[id] = card
			live[id] = freshness.nodeEligible(id)
			break
		}
	}
	links := append([]DashLink(nil), d.Links...)
	sort.Slice(links, func(i, j int) bool { return links[i].ID < links[j].ID })
	linkIndexes := make(map[string]int, len(d.Links))
	for i, link := range d.Links {
		linkIndexes[link.ID] = i
	}
	// Group parallel tracks by unordered endpoint pair, independent of endpoint order.
	pair := func(l DashLink) string {
		if l.Ends[0] == nil || l.Ends[1] == nil {
			return ""
		}
		a, b := l.Ends[0].Node, l.Ends[1].Node
		if a > b {
			a, b = b, a
		}
		return a + "\x00" + b
	}
	counts, seen := map[string]int{}, map[string]int{}
	for _, l := range links {
		if k := pair(l); k != "" {
			counts[k]++
		}
	}
	for i, l := range links {
		if l.Ends[0] == nil {
			continue
		}
		a, ok := cards[l.Ends[0].Node]
		if !ok {
			continue
		}
		c := SceneCable{ID: l.ID, Letter: cableLetter(i), State: l.State, Iface: l.Ends[0].Iface, Rate: Cell{Number: "—"}, Directions: []SceneDirection{}}
		c.X1, c.Y1 = a.X+a.Width, a.Y+a.Height/2
		c.X2, c.Y2, c.Lane = c.X1+32, c.Y1, c.Y1
		if l.Ends[1] != nil {
			b, found := cards[l.Ends[1].Node]
			if !found {
				continue
			}
			k := pair(l)
			track := seen[k]
			seen[k]++
			offset := (float64(track) - float64(counts[k]-1)/2) * 32
			if b.X < a.X {
				a, b = b, a
			}
			c.X1, c.X2 = a.X+a.Width, b.X
			c.Y1, c.Y2 = a.Y+a.Height*0.52+offset, b.Y+b.Height*0.52+offset
			c.Lane = 100 + offset
		}
		var busiest *float64
		allDirectionsEligible := len(l.Directions) > 0
		for directionIndex, v := range l.Directions {
			eligible := freshness.directionEligible(linkIndexes[l.ID], directionIndex)
			if !eligible {
				allDirectionsEligible = false
			}
			direction := SceneDirection{From: v.From, To: v.To, Rate: "— MB/s", Color: "neutral", Forward: cards[v.From].X <= cards[v.To].X}
			if eligible && v.WireGBs != nil {
				direction.Rate = fmt.Sprintf("%.1f MB/s", *v.WireGBs*1000)
				r := v.PayloadGBs
				if r == nil {
					r = v.WireGBs
				}
				if r != nil && (busiest == nil || *r > *busiest) {
					busiest = r
				}
				if v.From == d.Source.Host {
					direction.Color = "out"
				} else if v.To == d.Source.Host {
					direction.Color = "in"
				}
				if *v.WireGBs >= 0.000001 && l.State == "up" && live[v.From] && live[v.To] {
					u := math.Max(0, math.Min(1, math.Log(*v.WireGBs/0.000001)/math.Log(12.6/0.000001)))
					direction.Speed = 2 * math.Pow(300, u)
				}
			}
			c.Directions = append(c.Directions, direction)
		}
		if allDirectionsEligible && busiest != nil {
			c.Rate = rateCell(*busiest)
		}
		scene.Cables = append(scene.Cables, c)
	}
	return scene
}
