package tui

import "unicode"

// Glyph policy (Rules G1–G3 from docs/TUI_DESIGN_PROOF.md §2.3).
//
// The load-bearing decision of the whole console: a single mis-measured glyph
// in a layout-bearing cell shifts every column after it, which is the visible
// "broken grid" that helped make v0.22 unreleasable. Codex's Gate-1 review
// added a precondition that this discipline cover ALL non-ASCII layout glyphs
// — box drawing, ◉, arrows, checkmarks, the timer, the ellipsis — not only the
// Egyptian hieroglyphs. This file is that policy, expressed as data so the
// test can iterate every glyph the renderer is allowed to place.

// hieroglyphBlock is the Egyptian Hieroglyphs Unicode block (U+13000–U+1342F).
// Almost no terminal monospace font renders it; in the reference faces
// (JetBrains Mono, SF Mono, Iosevka) these codepoints are tofu, and tofu for an
// East-Asian-width-ambiguous codepoint breaks alignment.
const (
	hieroglyphLo = 0x13000
	hieroglyphHi = 0x1342F
)

// IsHieroglyph reports whether r is in the Egyptian Hieroglyphs block.
func IsHieroglyph(r rune) bool {
	return r >= hieroglyphLo && r <= hieroglyphHi
}

// Glyph is a non-ASCII codepoint the renderer is permitted to place in a
// layout-bearing cell, together with the contract that keeps the grid intact.
type Glyph struct {
	// Name is a stable identifier used in tests and command wiring.
	Name string
	// R is the preferred codepoint.
	R rune
	// Width is the cell advance the layout engine reserves for R. Every
	// layout-bearing glyph in v1 is single-width; the test enforces it.
	Width int
	// ASCII is the width-1 fallback used when the terminal cannot render R
	// safely (capability probe failed, NO_COLOR linear mode, or unknown TERM).
	ASCII string
}

// layoutGlyphs is the complete, closed set of non-ASCII glyphs the renderer may
// place in layout-bearing positions. Anything not in this set must be ASCII.
// The glyph_test.go suite iterates this table and asserts the G1–G3 invariants
// for every entry, so adding a glyph without a safe width+fallback fails CI.
var layoutGlyphs = []Glyph{
	// Deity / status sigils (§2.3 G2).
	{Name: "horus", R: '◉', Width: 1, ASCII: "o"},
	{Name: "focus-marker", R: '▸', Width: 1, ASCII: ">"},
	{Name: "timer", R: '⏱', Width: 1, ASCII: "t"},
	{Name: "ellipsis", R: '…', Width: 1, ASCII: "."},
	{Name: "check", R: '✓', Width: 1, ASCII: "x"},
	{Name: "bullet", R: '·', Width: 1, ASCII: "-"},
	{Name: "arrow-right", R: '→', Width: 1, ASCII: ">"},
	{Name: "arrow-up", R: '↑', Width: 1, ASCII: "^"},
	{Name: "arrow-down", R: '↓', Width: 1, ASCII: "v"},
	{Name: "spinner-static", R: '⠿', Width: 1, ASCII: "*"},

	// Box-drawing light set (§2.3) — universally present, but still measured
	// here so the renderer never reaches for an unverified codepoint.
	{Name: "box-v", R: '│', Width: 1, ASCII: "|"},
	{Name: "box-h", R: '─', Width: 1, ASCII: "-"},
	{Name: "box-tl", R: '┌', Width: 1, ASCII: "+"},
	{Name: "box-tr", R: '┐', Width: 1, ASCII: "+"},
	{Name: "box-bl", R: '└', Width: 1, ASCII: "+"},
	{Name: "box-br", R: '┘', Width: 1, ASCII: "+"},
	{Name: "box-vr", R: '├', Width: 1, ASCII: "+"},
	{Name: "box-vl", R: '┤', Width: 1, ASCII: "+"},
	{Name: "box-ht", R: '┬', Width: 1, ASCII: "+"},
	{Name: "box-hb", R: '┴', Width: 1, ASCII: "+"},
	{Name: "box-x", R: '┼', Width: 1, ASCII: "+"},
}

// glyphByName indexes layoutGlyphs for O(1) lookup by the renderer.
var glyphByName = func() map[string]Glyph {
	m := make(map[string]Glyph, len(layoutGlyphs))
	for _, g := range layoutGlyphs {
		m[g.Name] = g
	}
	return m
}()

// LayoutGlyphs returns a copy of the closed glyph set (for tests and tooling).
func LayoutGlyphs() []Glyph {
	out := make([]Glyph, len(layoutGlyphs))
	copy(out, layoutGlyphs)
	return out
}

// Sigil renders a layout glyph by name under the given capabilities. When the
// terminal cannot safely render the codepoint, the width-1 ASCII fallback is
// returned instead, so the column never shifts. Unknown names return "?" — a
// width-1 last resort that is still grid-safe.
func Sigil(name string, caps Capabilities) string {
	g, ok := glyphByName[name]
	if !ok {
		return "?"
	}
	if caps.UnicodeLayout {
		return string(g.R)
	}
	return g.ASCII
}

// IsLayoutSafe reports whether r may appear in a layout-bearing cell. ASCII is
// always safe; hieroglyphs (G1) never are; any other non-ASCII rune is safe
// only if it is a declared layout glyph with a verified width.
func IsLayoutSafe(r rune) bool {
	if r < unicode.MaxASCII {
		return true
	}
	if IsHieroglyph(r) {
		return false
	}
	for _, g := range layoutGlyphs {
		if g.R == r {
			return true
		}
	}
	return false
}
