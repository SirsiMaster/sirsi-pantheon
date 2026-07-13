// Package dashboard serves the Pantheon local dashboard — a self-contained
// HTML UI at localhost:9119. Menubar clicks open browser pages; CLI stays
// in the terminal. Both surfaces read from the same data stores.
package dashboard

import "github.com/SirsiMaster/sirsi-pantheon/internal/brand"

// Brand colors for HTML templates — sourced from internal/brand (ADR-038), the
// ONE Pantheon palette, so the dashboard can never drift from the CLI/TUI/app.
// Emerald leads (identity · healthy · interactive); gold is the second accent
// (owner-action). Dark scheme — the dashboard renders on a dark ground.
var (
	pal = brand.For(brand.Dark)

	ColorEmerald  = pal.Hex(brand.Emerald) // primary identity accent
	ColorGold     = pal.Hex(brand.Gold)    // second accent · owner-action
	ColorBlack    = pal.Hex(brand.Bg)
	ColorLapis    = pal.Hex(brand.Info) // retired lapis → brand info blue
	ColorWhite    = pal.Hex(brand.Ink)
	ColorDim      = pal.Hex(brand.Dim)
	ColorRed      = pal.Hex(brand.Danger)
	ColorGreen    = pal.Hex(brand.OK)
	ColorYellow   = pal.Hex(brand.Warn)
	ColorBg       = pal.Hex(brand.Bg)
	ColorBgPanel  = "rgba(20,28,23,.90)"   // panel over the emerald-biased ground
	ColorBorder   = "rgba(43,210,155,.14)" // emerald hairline
	ColorBorderHi = "rgba(43,210,155,.38)" // emerald hairline, emphasized
)

// DashboardPort is the fixed port for the dashboard server.
const DashboardPort = 9119
