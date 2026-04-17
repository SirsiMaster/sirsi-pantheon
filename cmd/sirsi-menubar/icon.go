// Package main — sirsi-menubar
//
// icon.go — Embedded icon for the macOS menu bar.
//
// macOS menu bar icons should be "template images" — monochrome with alpha.
// The system tints them white on dark backgrounds and black on light.
// Size: 22×22 pixels is the standard for @1x, 44×44 for @2x.
//
// This uses @2x (44×44) for Retina displays with HEAVYWEIGHT strokes.
package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// generateAnkhIcon creates a 44x44 ankh symbol as a macOS template icon (@2x Retina).
// Uses HEAVYWEIGHT strokes — thick filled shapes for maximum visibility in the menu bar.
// Solid black pixels on transparent — macOS handles the tinting.
func generateAnkhIcon() []byte {
	const size = 44
	img := image.NewNRGBA(image.Rect(0, 0, size, size))

	black := color.NRGBA{0, 0, 0, 255}

	// Helper: fill a circle (used for the loop at top of the ankh)
	fillCircle := func(cx, cy, r int) {
		for y := cy - r; y <= cy+r; y++ {
			for x := cx - r; x <= cx+r; x++ {
				dx := x - cx
				dy := y - cy
				if dx*dx+dy*dy <= r*r {
					if x >= 0 && x < size && y >= 0 && y < size {
						img.Set(x, y, black)
					}
				}
			}
		}
	}

	// Helper: fill a rectangle
	fillRect := func(x1, y1, x2, y2 int) {
		for y := y1; y <= y2; y++ {
			for x := x1; x <= x2; x++ {
				if x >= 0 && x < size && y >= 0 && y < size {
					img.Set(x, y, black)
				}
			}
		}
	}

	// Helper: clear circle (cut out from filled circle to make a ring)
	clearCircle := func(cx, cy, r int) {
		trans := color.NRGBA{0, 0, 0, 0}
		for y := cy - r; y <= cy+r; y++ {
			for x := cx - r; x <= cx+r; x++ {
				dx := x - cx
				dy := y - cy
				if dx*dx+dy*dy <= r*r {
					if x >= 0 && x < size && y >= 0 && y < size {
						img.Set(x, y, trans)
					}
				}
			}
		}
	}

	// ── Draw the Ankh (☥) — HEAVYWEIGHT ────────────────────────────
	//
	// Structure:
	//   1. Oval loop at top (filled circle minus inner circle = thick ring)
	//   2. Wide crossbar through the center
	//   3. Thick vertical stem below crossbar

	centerX := size / 2 // 22

	// 1. LOOP: outer circle at top, thick ring
	loopCenterY := 12
	fillCircle(centerX, loopCenterY, 11) // Outer radius 11
	clearCircle(centerX, loopCenterY, 6) // Inner radius 6 → 5px thick ring

	// 2. CROSSBAR: wide and thick, positioned at the bottom of the loop
	crossbarY := 21
	fillRect(5, crossbarY, 38, crossbarY+4) // Full width, 5px tall

	// 3. STEM: thick vertical below crossbar
	stemWidth := 6
	stemX1 := centerX - stemWidth/2
	stemX2 := centerX + stemWidth/2 - 1
	fillRect(stemX1, crossbarY+5, stemX2, 42) // Down to near bottom

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// getIcon returns the menu bar icon bytes.
func getIcon() []byte {
	return generateAnkhIcon()
}
