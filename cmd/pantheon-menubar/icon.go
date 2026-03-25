// Package main — pantheon-menubar
//
// icon.go — Embedded icon for the macOS menu bar.
//
// The icon is a small 22x22 PNG ankh symbol (𓋹) rendered in gold on transparent.
// For the menu bar, macOS requires a template image (monochrome).
// We embed the raw PNG bytes and pass them to systray.SetIcon().
//
// To regenerate: use the generate_icon tool or manually create a 22x22 PNG.
package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// generateAnkhIcon creates a procedural 22x22 ankh icon for the menu bar.
// This is a monochrome icon suitable for macOS template rendering.
// The ankh symbol (☥) represents life and is the Pantheon emblem.
func generateAnkhIcon() []byte {
	const size = 22

	// Create a monochrome image
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Draw ankh symbol procedurally.
	// The ankh consists of:
	// - An oval/loop at top (the handle)
	// - A horizontal crossbar
	// - A vertical stem below

	white := color.RGBA{0, 0, 0, 255} // Black for template icon (macOS inverts)

	// Loop (oval at top): rows 2-9
	loopPixels := map[[2]int]bool{
		// Top of oval
		{9, 2}: true, {10, 2}: true, {11, 2}: true, {12, 2}: true,
		// Upper sides
		{8, 3}: true, {13, 3}: true,
		{7, 4}: true, {14, 4}: true,
		{7, 5}: true, {14, 5}: true,
		{7, 6}: true, {14, 6}: true,
		{8, 7}: true, {13, 7}: true,
		// Bottom of oval (connects to stem)
		{9, 8}: true, {12, 8}: true,
		{10, 9}: true, {11, 9}: true,
	}

	// Crossbar: row 10-11
	for x := 5; x <= 16; x++ {
		loopPixels[[2]int{x, 10}] = true
		loopPixels[[2]int{x, 11}] = true
	}

	// Stem: rows 12-19
	for y := 12; y <= 19; y++ {
		loopPixels[[2]int{10, y}] = true
		loopPixels[[2]int{11, y}] = true
	}

	// Draw
	for coord := range loopPixels {
		x, y := coord[0], coord[1]
		if x >= 0 && x < size && y >= 0 && y < size {
			img.Set(x, y, white)
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// getIcon returns the menu bar icon bytes.
func getIcon() []byte {
	return generateAnkhIcon()
}
