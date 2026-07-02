package tui

import (
	"os"

	"charm.land/lipgloss/v2"
)

// Color & capability model (docs/TUI_DESIGN_PROOF.md §2.4, §5).
//
// Color is semantic, never decorative: every token means exactly one thing and
// degrades down a truecolor → 256 → 16 → attribute-only ladder. NO_COLOR
// collapses to attribute-only so meaning is never carried by color alone.

// ColorDepth is a rung on the degrade ladder.
type ColorDepth int

const (
	// ColorNone is attribute-only (bold/reverse/underline); meaning must still
	// survive. This is the NO_COLOR and linear-renderer default.
	ColorNone ColorDepth = iota
	// Color16 is the ANSI base palette.
	Color16
	// Color256 is the xterm 256-color cube.
	Color256
	// ColorTrue is 24-bit truecolor — full brand fidelity.
	ColorTrue
)

// Token is a semantic color role. The render layer maps a token to an actual
// color (or attribute) at the active ColorDepth; primitives only ever name the
// role, never a hex value, so the ladder is honored everywhere by construction.
type Token int

const (
	TokBrand  Token = iota // gold #C8A951 — identity, headers, selected counts
	TokAccent              // lapis #1A1A5E — mode chips, active focus border
	TokOK                  // green — pass, safe, healthy
	TokWarn                // amber — needs attention, reclaimable
	TokDanger              // red — destructive, error, protected-path block
	TokDim                 // gray — chrome, hints, truncation
)

// SeverityLabel is the text token a severity carries IN ADDITION to color, so
// colorblind and NO_COLOR operators lose no information (§5). A severity is
// never communicated by color alone.
func (t Token) SeverityLabel() string {
	switch t {
	case TokOK:
		return "PASS"
	case TokWarn:
		return "WARN"
	case TokDanger:
		return "BLOCK"
	case TokDim:
		return "INFO"
	default:
		return ""
	}
}

// Capabilities is the resolved terminal/environment profile that the renderer
// reads. It is intentionally a plain value so tests can construct any profile
// directly without touching the real environment.
type Capabilities struct {
	// Color is the deepest color rung this surface may use.
	Color ColorDepth
	// UnicodeLayout permits declared layout glyphs (glyph.go) to render as
	// their preferred codepoint instead of the ASCII fallback.
	UnicodeLayout bool
	// ReducedMotion disables spinners/slides/pulses (§5); progress and spinners
	// degrade to determinate, static forms.
	ReducedMotion bool
	// AltScreen indicates the fullscreen alternate buffer is in use. When
	// false, the linear renderer (the accessible, screen-reader-traversable
	// path) is selected — a first-class renderer, not an adapter.
	AltScreen bool
}

// DetectCapabilities resolves capabilities from the environment. It is
// deliberately conservative: anything it cannot positively confirm degrades to
// the safe rung. The capability probe for layout glyphs is modeled here as a
// TERM allowlist rather than a live cursor-advance measurement so the scaffold
// stays deterministic; the probe (§2.3 G3) is a Gate-3 concern.
func DetectCapabilities(env func(string) string) Capabilities {
	if env == nil {
		env = os.Getenv
	}
	caps := Capabilities{
		Color:         Color16,
		UnicodeLayout: true,
		ReducedMotion: false,
		AltScreen:     true,
	}

	// NO_COLOR (https://no-color.org) collapses to attribute-only.
	if env("NO_COLOR") != "" {
		caps.Color = ColorNone
		caps.ReducedMotion = true
	} else {
		switch env("COLORTERM") {
		case "truecolor", "24bit":
			caps.Color = ColorTrue
		}
	}

	switch term := env("TERM"); term {
	case "", "dumb":
		// No positive terminal signal: take the safest path, including the
		// linear renderer and ASCII glyphs.
		caps.Color = ColorNone
		caps.UnicodeLayout = false
		caps.AltScreen = false
		caps.ReducedMotion = true
	}

	// An explicit opt-out of the alternate screen selects the linear renderer
	// regardless of terminal (accessibility / screen-reader pairing).
	if env("SIRSI_TUI_NO_ALTSCREEN") != "" {
		caps.AltScreen = false
	}
	if env("SIRSI_TUI_REDUCE_MOTION") != "" {
		caps.ReducedMotion = true
	}

	return caps
}

// Brand hex values (§2.4). Truecolor path only; lower rungs degrade below.
const (
	hexBrand  = "#C8A951" // gold
	hexAccent = "#1A1A5E" // deep lapis
	hexOK     = "#3FB950" // green
	hexWarn   = "#D2A24C" // amber
	hexDanger = "#F04747" // red
	hexDim    = "#888888" // gray
	hexBlack  = "#0F0F0F" // near-black background
	hexWhite  = "#FAFAFA" // body text
)

// hex maps a token to its truecolor hex.
func (t Token) hex() string {
	switch t {
	case TokBrand:
		return hexBrand
	case TokAccent:
		return hexAccent
	case TokOK:
		return hexOK
	case TokWarn:
		return hexWarn
	case TokDanger:
		return hexDanger
	default:
		return hexDim
	}
}

// ansi256 maps a token to its xterm-256 fallback (§2.4 ladder).
func (t Token) ansi256() string {
	switch t {
	case TokBrand:
		return "179"
	case TokAccent:
		return "17"
	case TokOK:
		return "35"
	case TokWarn:
		return "214"
	case TokDanger:
		return "196"
	default:
		return "244"
	}
}

// ansi16 maps a token to a base ANSI color name (§2.4 ladder).
func (t Token) ansi16() string {
	switch t {
	case TokBrand, TokWarn:
		return "3" // yellow
	case TokAccent:
		return "4" // blue
	case TokOK:
		return "2" // green
	case TokDanger:
		return "1" // red
	default:
		return "7" // default/gray
	}
}

// Paint applies the semantic token to s at the surface's color depth. At
// ColorNone it degrades to an attribute (bold) so meaning survives without color
// (§2.4, §5); color is never the sole carrier of meaning — severities always
// also carry a text token (SeverityLabel).
func Paint(s string, tok Token, caps Capabilities) string {
	switch caps.Color {
	case ColorTrue:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(tok.hex())).Render(s)
	case Color256:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(tok.ansi256())).Render(s)
	case Color16:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(tok.ansi16())).Render(s)
	default:
		// Attribute-only: brand/headers bold, danger reverse, everything else plain.
		switch tok {
		case TokBrand:
			return lipgloss.NewStyle().Bold(true).Render(s)
		case TokDanger:
			return lipgloss.NewStyle().Reverse(true).Render(s)
		default:
			return s
		}
	}
}

// Chip renders a lapis-background mode chip with white text (§2.4: lapis is a
// background, never text-on-black). At ColorNone it degrades to reverse video.
func Chip(s string, caps Capabilities) string {
	label := " " + s + " "
	switch caps.Color {
	case ColorTrue:
		return lipgloss.NewStyle().
			Background(lipgloss.Color(hexAccent)).
			Foreground(lipgloss.Color(hexWhite)).
			Bold(true).Render(label)
	case Color256:
		return lipgloss.NewStyle().
			Background(lipgloss.Color("17")).
			Foreground(lipgloss.Color("231")).Render(label)
	case Color16:
		return lipgloss.NewStyle().
			Background(lipgloss.Color("4")).
			Foreground(lipgloss.Color("15")).Render(label)
	default:
		return lipgloss.NewStyle().Reverse(true).Render(label)
	}
}
