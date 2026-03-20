package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os/exec"
	"sync"
)

// StreamRecorder runs pw-record in raw mode, outputting float32 PCM to stdout.
// Audio is read in chunks for real-time streaming transcription.
type StreamRecorder struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdout  io.ReadCloser
	target  string
	running bool
}

// NewStreamRecorder creates a recorder for raw PCM streaming.
func NewStreamRecorder(target string) *StreamRecorder {
	return &StreamRecorder{target: target}
}

// SetTarget changes the recording target device.
func (r *StreamRecorder) SetTarget(target string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.target = target
}

// GetTarget returns the current recording target node name.
func (r *StreamRecorder) GetTarget() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.target
}

// Start begins streaming raw PCM audio from pw-record.
func (r *StreamRecorder) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return fmt.Errorf("already recording")
	}

	args := []string{
		"--raw",
		"--rate", fmt.Sprint(SampleRate),
		"--channels", "1",
		"--format", "f32",
	}
	if r.target != "" {
		args = append(args, "--target", r.target)
	}
	args = append(args, "-") // stdout

	r.cmd = exec.Command("pw-record", args...)
	r.cmd.Stderr = nil

	var err error
	r.stdout, err = r.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("start pw-record: %w", err)
	}

	r.running = true
	return nil
}

// ReadChunk reads exactly n float32 samples from the stream.
// Blocks until all samples are available or an error occurs.
func (r *StreamRecorder) ReadChunk(n int) ([]float32, error) {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return nil, fmt.Errorf("not recording")
	}
	stdout := r.stdout
	r.mu.Unlock()

	buf := make([]byte, n*4)
	_, err := io.ReadFull(stdout, buf)
	if err != nil {
		return nil, fmt.Errorf("read audio: %w", err)
	}

	samples := make([]float32, n)
	for i := 0; i < n; i++ {
		bits := binary.LittleEndian.Uint32(buf[i*4 : i*4+4])
		samples[i] = math.Float32frombits(bits)
	}

	return samples, nil
}

// Stop ends the recording stream and cleans up.
func (r *StreamRecorder) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return
	}

	r.running = false
	if r.cmd.Process != nil {
		r.cmd.Process.Kill()
	}
	if r.stdout != nil {
		r.stdout.Close()
	}
	r.cmd.Wait()
}
