package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// Generated icons — colored circles with mic silhouette.
// 48x48 for crisp display in Waybar tray.

var (
	IconIdle       []byte
	IconRecording  []byte
	IconProcessing []byte
	IconListening  []byte
)

func init() {
	IconIdle = generateIcon(color.RGBA{130, 130, 140, 255})      // grey
	IconRecording = generateIcon(color.RGBA{230, 40, 40, 255})   // bright red
	IconProcessing = generateIcon(color.RGBA{230, 180, 30, 255}) // amber
	IconListening = generateIcon(color.RGBA{40, 200, 80, 255})   // green
}

func generateIcon(c color.RGBA) []byte {
	const size = 48
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	cx, cy := float64(size)/2, float64(size)/2
	r := float64(size)/2 - 1

	// Filled circle with 1px anti-aliased edge
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)-cx+0.5, float64(y)-cy+0.5
			dist := math.Sqrt(dx*dx+dy*dy) - r
			if dist < -1 {
				img.SetRGBA(x, y, c)
			} else if dist < 0 {
				// Anti-alias edge
				a := uint8(float64(c.A) * (-dist))
				img.SetRGBA(x, y, color.RGBA{c.R, c.G, c.B, a})
			}
		}
	}

	// Mic icon in white, scaled to 48x48
	white := color.RGBA{255, 255, 255, 255}

	// Mic capsule (rounded rect, ~center)
	for y := 10; y <= 26; y++ {
		for x := 19; x <= 28; x++ {
			// Round the top and bottom
			if y <= 12 || y >= 24 {
				dx := float64(x) - 23.5
				dy := float64(y)
				if y <= 12 {
					dy -= 12
				} else {
					dy -= 24
				}
				if dx*dx+dy*dy <= 5*5 {
					img.SetRGBA(x, y, white)
				}
			} else {
				img.SetRGBA(x, y, white)
			}
		}
	}

	// Mic cradle (U-shape)
	for y := 20; y <= 30; y++ {
		for x := 16; x <= 31; x++ {
			dx := float64(x) - 23.5
			dy := float64(y) - 22.0
			outerR := 8.5
			innerR := 6.5
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist >= innerR && dist <= outerR && y >= 24 {
				img.SetRGBA(x, y, white)
			}
		}
	}

	// Stand (vertical bar)
	for y := 30; y <= 34; y++ {
		for x := 22; x <= 25; x++ {
			img.SetRGBA(x, y, white)
		}
	}

	// Base plate
	for x := 18; x <= 29; x++ {
		for y := 34; y <= 36; y++ {
			img.SetRGBA(x, y, white)
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
