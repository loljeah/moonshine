package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// Microphone icon - simple and clear for speech recognition
// Procedurally generated since Twemoji wasn't rendering correctly

// Icon variants for different states.
var (
	IconIdle       []byte
	IconRecording  []byte
	IconProcessing []byte
	IconListening  []byte
	IconDisabled   []byte // Microphone with red X overlay
)

func init() {
	// Create microphone icon procedurally
	micIcon := createMicrophoneIcon()

	IconIdle = micIcon
	IconRecording = micIcon
	IconProcessing = micIcon
	IconListening = micIcon

	// Create disabled icon with red X overlay
	IconDisabled = createDisabledMicIcon()
}

// createMicrophoneIcon creates a 64x64 microphone icon.
func createMicrophoneIcon() []byte {
	const size = 64
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Colors
	micBody := color.RGBA{80, 80, 90, 255}    // Dark gray mic body
	micGrill := color.RGBA{60, 60, 70, 255}   // Slightly darker grill
	micStand := color.RGBA{100, 100, 110, 255} // Stand/base

	cx := size / 2 // center x

	// Draw microphone body (rounded rectangle/capsule shape)
	// Top capsule part (the mic head)
	for y := 8; y < 38; y++ {
		for x := cx - 12; x <= cx+12; x++ {
			// Rounded top
			if y < 18 {
				dy := float64(y - 18)
				dx := float64(x - cx)
				if dx*dx+dy*dy <= 12*12 {
					img.Set(x, y, micBody)
				}
			} else {
				// Rectangular body
				img.Set(x, y, micBody)
			}
		}
	}

	// Microphone grill lines (horizontal lines on mic head)
	for y := 12; y < 32; y += 4 {
		for x := cx - 10; x <= cx+10; x++ {
			if y < 18 {
				dy := float64(y - 18)
				dx := float64(x - cx)
				if dx*dx+dy*dy <= 10*10 {
					img.Set(x, y, micGrill)
				}
			} else {
				img.Set(x, y, micGrill)
			}
		}
	}

	// Draw the U-shaped holder/stand around mic
	for y := 24; y < 44; y++ {
		// Left arm of U
		for x := cx - 18; x <= cx-14; x++ {
			img.Set(x, y, micStand)
		}
		// Right arm of U
		for x := cx + 14; x <= cx+18; x++ {
			img.Set(x, y, micStand)
		}
	}
	// Bottom of U
	for x := cx - 18; x <= cx+18; x++ {
		for y := 42; y <= 46; y++ {
			img.Set(x, y, micStand)
		}
	}

	// Vertical stand
	for y := 46; y < 54; y++ {
		for x := cx - 3; x <= cx+3; x++ {
			img.Set(x, y, micStand)
		}
	}

	// Base
	for x := cx - 14; x <= cx+14; x++ {
		for y := 54; y <= 58; y++ {
			img.Set(x, y, micStand)
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

// createDisabledMicIcon creates the microphone icon with a red X overlay.
func createDisabledMicIcon() []byte {
	const size = 64
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// First draw the microphone
	micBody := color.RGBA{80, 80, 90, 255}
	micGrill := color.RGBA{60, 60, 70, 255}
	micStand := color.RGBA{100, 100, 110, 255}
	cx := size / 2

	// Mic body
	for y := 8; y < 38; y++ {
		for x := cx - 12; x <= cx+12; x++ {
			if y < 18 {
				dy := float64(y - 18)
				dx := float64(x - cx)
				if dx*dx+dy*dy <= 12*12 {
					img.Set(x, y, micBody)
				}
			} else {
				img.Set(x, y, micBody)
			}
		}
	}

	// Grill lines
	for y := 12; y < 32; y += 4 {
		for x := cx - 10; x <= cx+10; x++ {
			if y < 18 {
				dy := float64(y - 18)
				dx := float64(x - cx)
				if dx*dx+dy*dy <= 10*10 {
					img.Set(x, y, micGrill)
				}
			} else {
				img.Set(x, y, micGrill)
			}
		}
	}

	// U holder
	for y := 24; y < 44; y++ {
		for x := cx - 18; x <= cx-14; x++ {
			img.Set(x, y, micStand)
		}
		for x := cx + 14; x <= cx+18; x++ {
			img.Set(x, y, micStand)
		}
	}
	for x := cx - 18; x <= cx+18; x++ {
		for y := 42; y <= 46; y++ {
			img.Set(x, y, micStand)
		}
	}

	// Stand
	for y := 46; y < 54; y++ {
		for x := cx - 3; x <= cx+3; x++ {
			img.Set(x, y, micStand)
		}
	}
	for x := cx - 14; x <= cx+14; x++ {
		for y := 54; y <= 58; y++ {
			img.Set(x, y, micStand)
		}
	}

	// Draw red X overlay
	red := color.RGBA{220, 50, 50, 255}
	thickness := 5
	pad := 6

	// Draw X diagonals
	for i := pad; i < size-pad; i++ {
		progress := float64(i-pad) / float64(size-2*pad-1)
		x1 := pad + int(progress*float64(size-2*pad-1))
		x2 := size - pad - 1 - int(progress*float64(size-2*pad-1))
		y := i

		for dx := -thickness / 2; dx <= thickness/2; dx++ {
			for dy := -thickness / 2; dy <= thickness/2; dy++ {
				px1, py1 := x1+dx, y+dy
				if px1 >= 0 && px1 < size && py1 >= 0 && py1 < size {
					img.Set(px1, py1, red)
				}
				px2, py2 := x2+dx, y+dy
				if px2 >= 0 && px2 < size && py2 >= 0 && py2 < size {
					img.Set(px2, py2, red)
				}
			}
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

