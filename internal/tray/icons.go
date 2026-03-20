package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// Generated icons — simple colored circles with mic silhouette.
// These are generated at init time to avoid embedding binary assets.

var (
	IconIdle       []byte
	IconRecording  []byte
	IconProcessing []byte
)

func init() {
	IconIdle = generateIcon(color.RGBA{150, 150, 150, 255})       // grey
	IconRecording = generateIcon(color.RGBA{220, 50, 50, 255})    // red
	IconProcessing = generateIcon(color.RGBA{220, 180, 50, 255})  // amber
}

func generateIcon(c color.RGBA) []byte {
	const size = 22
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Draw a filled circle
	cx, cy, r := size/2, size/2, size/2-1
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r {
				img.SetRGBA(x, y, c)
			}
		}
	}

	// Draw a simple mic shape (vertical bar + base) in white
	white := color.RGBA{255, 255, 255, 255}
	// Mic body: vertical rectangle in center
	for y := 4; y <= 12; y++ {
		for x := 9; x <= 12; x++ {
			img.SetRGBA(x, y, white)
		}
	}
	// Mic base: horizontal bar
	for x := 8; x <= 13; x++ {
		img.SetRGBA(x, 14, white)
	}
	// Stand
	img.SetRGBA(10, 15, white)
	img.SetRGBA(11, 15, white)
	// Base plate
	for x := 8; x <= 13; x++ {
		img.SetRGBA(x, 16, white)
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
