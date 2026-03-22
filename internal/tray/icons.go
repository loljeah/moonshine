package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// Generated icons — crescent moon shape with color states.
// 48x48 for crisp display in Waybar tray.

var (
	IconIdle       []byte
	IconRecording  []byte
	IconProcessing []byte
	IconListening  []byte
)

func init() {
	IconIdle = generateMoonIcon(color.RGBA{130, 130, 140, 255})      // grey
	IconRecording = generateMoonIcon(color.RGBA{230, 40, 40, 255})   // bright red
	IconProcessing = generateMoonIcon(color.RGBA{230, 180, 30, 255}) // amber/gold
	IconListening = generateMoonIcon(color.RGBA{40, 200, 80, 255})   // green
}

// generateMoonIcon creates a crescent moon icon with the given color.
// The moon shape is created by subtracting a smaller circle from a larger one.
func generateMoonIcon(c color.RGBA) []byte {
	const size = 48
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Main moon circle center and radius
	cx, cy := float64(size)/2, float64(size)/2
	r := float64(size)/2 - 2

	// Cutout circle (offset to the right to create crescent)
	cutX := cx + r*0.55
	cutY := cy
	cutR := r * 0.75

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5

			// Distance from main circle center
			dx1, dy1 := px-cx, py-cy
			distMain := math.Sqrt(dx1*dx1 + dy1*dy1)

			// Distance from cutout circle center
			dx2, dy2 := px-cutX, py-cutY
			distCut := math.Sqrt(dx2*dx2 + dy2*dy2)

			// Inside main circle but outside cutout = moon
			inMain := distMain - r
			inCut := distCut - cutR

			if inMain < -1 && inCut > 1 {
				// Fully inside moon
				img.SetRGBA(x, y, c)
			} else if inMain < 0 && inCut > 0 {
				// Anti-alias edge
				alpha := 1.0
				if inMain > -1 {
					alpha = math.Min(alpha, -inMain)
				}
				if inCut < 1 {
					alpha = math.Min(alpha, inCut)
				}
				a := uint8(float64(c.A) * alpha)
				img.SetRGBA(x, y, color.RGBA{c.R, c.G, c.B, a})
			}
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
