package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

// ParseFloat32WAV reads a WAV file recorded by pw-record with format float32
// (WAV format tag 3 = IEEE_FLOAT). Returns raw PCM samples as []float32.
func ParseFloat32WAV(path string) ([]float32, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read wav: %w", err)
	}

	if len(data) < 44 {
		return nil, fmt.Errorf("file too small for WAV header")
	}

	// Find the "data" chunk — pw-record may have extra chunks before it
	dataOffset := -1
	for i := 0; i+4 <= len(data); i++ {
		if data[i] == 'd' && data[i+1] == 'a' && data[i+2] == 't' && data[i+3] == 'a' {
			dataOffset = i
			break
		}
	}

	if dataOffset < 0 {
		return nil, fmt.Errorf("no 'data' chunk found in WAV")
	}

	// data chunk: "data" (4 bytes) + size (4 bytes) + PCM samples
	if dataOffset+8 > len(data) {
		return nil, fmt.Errorf("truncated data chunk header")
	}

	chunkSize := int(binary.LittleEndian.Uint32(data[dataOffset+4 : dataOffset+8]))
	pcmStart := dataOffset + 8

	// Clamp to actual file size
	pcmEnd := pcmStart + chunkSize
	if pcmEnd > len(data) {
		pcmEnd = len(data)
	}

	pcmBytes := data[pcmStart:pcmEnd]
	sampleCount := len(pcmBytes) / 4
	if sampleCount == 0 {
		return nil, fmt.Errorf("no audio samples in WAV")
	}

	samples := make([]float32, sampleCount)
	for i := 0; i < sampleCount; i++ {
		bits := binary.LittleEndian.Uint32(pcmBytes[i*4 : i*4+4])
		samples[i] = math.Float32frombits(bits)
	}

	return samples, nil
}

// NormalizeAudio scales PCM audio so the peak absolute value is targetPeak.
// Returns the samples in-place (modifies the slice).
func NormalizeAudio(samples []float32, targetPeak float32) []float32 {
	if len(samples) == 0 {
		return samples
	}

	var maxAbs float32
	for _, s := range samples {
		abs := s
		if abs < 0 {
			abs = -abs
		}
		if abs > maxAbs {
			maxAbs = abs
		}
	}

	if maxAbs == 0 {
		return samples
	}

	scale := targetPeak / maxAbs
	for i := range samples {
		samples[i] *= scale
	}

	return samples
}
