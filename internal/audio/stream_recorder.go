package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os/exec"
	"sync"
	"sync/atomic"
)

// recorderState represents the atomic state of the recorder.
type recorderState int32

const (
	stateIdle recorderState = iota
	stateRunning
	stateStopping
)

// StreamRecorder runs pw-record in raw mode, outputting float32 PCM to stdout.
// Audio is read in chunks for real-time streaming transcription.
// Thread-safe with atomic state machine to prevent race conditions.
type StreamRecorder struct {
	mu     sync.Mutex
	state  atomic.Int32
	cmd    *exec.Cmd
	stdout io.ReadCloser
	target string
}

// NewStreamRecorder creates a recorder for raw PCM streaming.
func NewStreamRecorder(target string) *StreamRecorder {
	r := &StreamRecorder{target: target}
	r.state.Store(int32(stateIdle))
	return r
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

	if recorderState(r.state.Load()) != stateIdle {
		return fmt.Errorf("already recording or stopping")
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
		r.stdout = nil
		return fmt.Errorf("start pw-record: %w", err)
	}

	r.state.Store(int32(stateRunning))
	return nil
}

// ReadChunk reads exactly n float32 samples from the stream.
// Blocks until all samples are available or an error occurs.
// Returns error if recorder is stopped during read.
func (r *StreamRecorder) ReadChunk(n int) ([]float32, error) {
	// Check state atomically without holding lock
	if recorderState(r.state.Load()) != stateRunning {
		return nil, fmt.Errorf("not recording")
	}

	// Get stdout reference while holding lock
	r.mu.Lock()
	stdout := r.stdout
	r.mu.Unlock()

	// Check stdout is valid (may have been cleared by Stop)
	if stdout == nil {
		return nil, fmt.Errorf("recorder stopped")
	}

	buf := make([]byte, n*4)
	_, err := io.ReadFull(stdout, buf)
	if err != nil {
		// Check if we're stopping (expected error)
		if recorderState(r.state.Load()) != stateRunning {
			return nil, fmt.Errorf("recorder stopped")
		}
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
// Safe to call multiple times.
func (r *StreamRecorder) Stop() {
	// Atomically transition to stopping state
	if !r.state.CompareAndSwap(int32(stateRunning), int32(stateStopping)) {
		// Already stopped or stopping
		return
	}

	r.mu.Lock()
	cmd := r.cmd
	stdout := r.stdout
	r.stdout = nil // Clear to signal ReadChunk
	r.mu.Unlock()

	// Kill process first (unblocks any pending reads)
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
	}

	// Close stdout (safe even if already closed by process exit)
	if stdout != nil {
		stdout.Close()
	}

	// Wait for process to exit (don't hold lock)
	if cmd != nil {
		cmd.Wait()
	}

	// Transition to idle
	r.state.Store(int32(stateIdle))
}

// IsRunning returns true if the recorder is currently streaming.
func (r *StreamRecorder) IsRunning() bool {
	return recorderState(r.state.Load()) == stateRunning
}
