package audio

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

const (
	// stopTimeout is the maximum time to wait for graceful shutdown
	// before forcing kill
	stopTimeout = 3 * time.Second
)

const (
	SampleRate = 16000
	tmpDir     = "/tmp/moonshine"
	tmpWAV     = "/tmp/moonshine/audio_tmp.wav"
)

// Recorder manages a pw-record subprocess for audio capture.
type Recorder struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	target   string // PipeWire node name, or empty for default
	running  bool
}

// NewRecorder creates a recorder targeting a specific PipeWire node.
// Pass empty string for system default device.
func NewRecorder(target string) *Recorder {
	return &Recorder{target: target}
}

// SetTarget changes the recording target device.
func (r *Recorder) SetTarget(target string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.target = target
}

// GetTarget returns the current recording target node name.
func (r *Recorder) GetTarget() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.target
}

// Start begins recording. Non-blocking — recording runs in the background.
func (r *Recorder) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return fmt.Errorf("already recording")
	}

	os.MkdirAll(tmpDir, 0o700) // Restrict to owner (audio may be sensitive)
	os.Remove(tmpWAV)

	args := []string{
		"--rate", fmt.Sprint(SampleRate),
		"--channels", "1",
		"--format", "f32",
	}
	if r.target != "" {
		args = append(args, "--target", r.target)
	}
	args = append(args, tmpWAV)

	r.cmd = exec.Command("pw-record", args...)
	r.cmd.Stderr = nil // silence stderr

	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("start pw-record: %w", err)
	}

	r.running = true
	return nil
}

// Stop ends recording, waits for pw-record to flush, and returns the
// parsed PCM float32 audio. The temporary WAV file is cleaned up.
// Uses timeout to prevent hanging if pw-record doesn't respond to SIGINT.
func (r *Recorder) Stop() ([]float32, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return nil, fmt.Errorf("not recording")
	}

	r.running = false

	// Send SIGINT for graceful flush
	if r.cmd.Process != nil {
		r.cmd.Process.Signal(os.Interrupt)
	}

	// Wait with timeout, force kill if needed
	done := make(chan error, 1)
	go func() { done <- r.cmd.Wait() }()

	select {
	case <-done:
		// Process exited normally
	case <-time.After(stopTimeout):
		// Force kill if graceful shutdown didn't work
		if r.cmd.Process != nil {
			r.cmd.Process.Kill()
		}
		<-done // Wait for kill to complete
	}

	// Parse the WAV file pw-record wrote
	wavPath := filepath.Join(tmpDir, "audio_tmp.wav")
	samples, err := ParseFloat32WAV(wavPath)
	os.Remove(wavPath)

	if err != nil {
		return nil, fmt.Errorf("parse recording: %w", err)
	}

	// Need at least 0.1 seconds of audio (1600 samples at 16kHz)
	if len(samples) < 1600 {
		return nil, fmt.Errorf("recording too short (%d samples)", len(samples))
	}

	return samples, nil
}

// IsRecording returns whether recording is active.
func (r *Recorder) IsRecording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}
