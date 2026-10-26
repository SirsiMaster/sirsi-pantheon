package ledger

import (
	"github.com/SirsiMaster/sirsi-pantheon/internal/brand"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

type FabricSwatch struct {
	Dark  string `json:"dark"`
	Light string `json:"light"`
}

// Roles come from the producer; consumers resolve the same existing brand
// tokens, never derive runtime classification or invent a color from task count.
type FabricPresentation struct {
	Headlines map[string]string       `json:"headlines"`
	Palette   map[string]FabricSwatch `json:"palette"`
}

func fabricPresentation() FabricPresentation {
	p := FabricPresentation{Headlines: map[string]string{
		"completed": brand.OK.Name(), "progress": brand.Gold.Name(),
		"stalled": brand.Warn.Name(), "messages": brand.Ink2.Name(),
	}, Palette: map[string]FabricSwatch{}}
	for _, role := range []brand.Role{brand.OK, brand.Gold, brand.Warn, brand.Ink2, brand.Dim} {
		p.Palette[role.Name()] = FabricSwatch{Dark: brand.For(brand.Dark).Hex(role), Light: brand.For(brand.Light).Hex(role)}
	}
	return p
}

func fabricLaneRole(state string) string {
	switch state {
	case routerstore.LaneWorking, routerstore.LaneComplete:
		return brand.OK.Name()
	case routerstore.LaneAssigned:
		return brand.Gold.Name()
	case routerstore.LaneIdleWithWork, routerstore.LaneBlocked, routerstore.LaneUnroutable:
		return brand.Warn.Name()
	default:
		return brand.Dim.Name()
	}
}
