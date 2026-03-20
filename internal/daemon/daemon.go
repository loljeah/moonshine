package daemon

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"moonshine-daemon/internal/audio"
	"moonshine-daemon/internal/config"
	"moonshine-daemon/internal/moonshine"
)

// State represents the daemon's current processing state.
type State int

const (
	StateIdle       State = iota
	StateRecording
	StateProcessing
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateRecording:
		return "recording"
	case StateProcessing:
		return "processing"
	default:
		return "unknown"
	}
}

// OutputMode determines where transcribed text goes.
type OutputMode int

const (
	ModeClipboard OutputMode = iota
	ModeType
)

func (m OutputMode) String() string {
	if m == ModeType {
		return "type"
	}
	return "clipboard"
}

// ParseOutputMode converts a string to OutputMode.
func ParseOutputMode(s string) OutputMode {
	if s == "type" {
		return ModeType
	}
	return ModeClipboard
}

// StateChange is sent to listeners (e.g. the tray) when state changes.
type StateChange struct {
	State State
	Mode  OutputMode
}

const (
	stateDir  = "/tmp/moonshine"
	soundsDir = "/home" // overridden by actual path at runtime
)

// Daemon is the core state machine: idle -> recording -> processing -> idle.
type Daemon struct {
	mu          sync.Mutex
	state       State
	mode        OutputMode
	transcriber *moonshine.Transcriber
	recorder    *audio.Recorder
	cfg         *config.Config
	soundDir    string
	verbose     bool

	// StateCh broadcasts state changes (buffered, non-blocking send).
	StateCh chan StateChange
}

// New creates a Daemon with a loaded transcriber and config.
func New(transcriber *moonshine.Transcriber, cfg *config.Config, soundDir string, verbose bool) *Daemon {
	// Resolve target device from config
	target := ""
	deviceSearch := cfg.Device()
	if deviceSearch != "" {
		devices, err := audio.ListDevices()
		if err == nil {
			target = audio.FindDevice(devices, deviceSearch)
			if target != "" && verbose {
				log.Printf("matched device %q -> %s", deviceSearch, target)
			}
		}
	}

	d := &Daemon{
		state:       StateIdle,
		mode:        ModeClipboard,
		transcriber: transcriber,
		recorder:    audio.NewRecorder(target),
		cfg:         cfg,
		soundDir:    soundDir,
		verbose:     verbose,
		StateCh:     make(chan StateChange, 4),
	}

	os.MkdirAll(stateDir, 0o755)
	d.writeStatus()
	return d
}

// Toggle starts or stops recording. Returns transcribed text on stop, or
// "recording" if recording just started.
func (d *Daemon) Toggle(mode OutputMode) (string, error) {
	d.mu.Lock()

	switch d.state {
	case StateIdle:
		d.mode = mode
		d.state = StateRecording
		d.writeStatus()
		d.notify(StateRecording)
		d.mu.Unlock()

		d.playSound("start.wav")
		Notify("Recording", "Press again to stop")

		if err := d.recorder.Start(); err != nil {
			d.mu.Lock()
			d.state = StateIdle
			d.writeStatus()
			d.notify(StateIdle)
			d.mu.Unlock()
			return "", fmt.Errorf("start recording: %w", err)
		}
		return "recording", nil

	case StateRecording:
		d.state = StateProcessing
		d.writeStatus()
		d.notify(StateProcessing)
		currentMode := d.mode
		d.mu.Unlock()

		d.playSound("stop.wav")

		// Stop recording and get PCM
		samples, err := d.recorder.Stop()
		if err != nil {
			d.mu.Lock()
			d.state = StateIdle
			d.writeStatus()
			d.notify(StateIdle)
			d.mu.Unlock()
			d.playSound("error.wav")
			Notify("No Audio", "Recording was empty")
			return "", fmt.Errorf("stop recording: %w", err)
		}

		// Normalize audio
		audio.NormalizeAudio(samples, 0.95)

		if d.verbose {
			log.Printf("transcribing %d samples (%.1fs)", len(samples), float64(len(samples))/float64(audio.SampleRate))
		}

		// Transcribe
		lines, err := d.transcriber.Transcribe(samples, audio.SampleRate)
		if err != nil {
			d.mu.Lock()
			d.state = StateIdle
			d.writeStatus()
			d.notify(StateIdle)
			d.mu.Unlock()
			d.playSound("error.wav")
			Notify("Error", err.Error())
			return "", fmt.Errorf("transcribe: %w", err)
		}

		// Collect text from lines
		var parts []string
		for _, l := range lines {
			if l.Text != "" {
				parts = append(parts, l.Text)
			}
		}
		text := strings.Join(parts, " ")

		d.mu.Lock()
		d.state = StateIdle
		d.writeStatus()
		d.notify(StateIdle)
		d.mu.Unlock()

		if text == "" {
			d.playSound("error.wav")
			Notify("No Speech", "Couldn't detect any words")
			return "", nil
		}

		// Output
		CopyToClipboard(text)
		d.playSound("success.wav")

		if currentMode == ModeType {
			Notify("Typing...", text)
			if err := TypeText(text); err != nil {
				Notify("Copied", text+" (wtype failed, use Ctrl+V)")
			}
		} else {
			Notify("Copied", text)
		}

		return text, nil

	case StateProcessing:
		d.mu.Unlock()
		return "", fmt.Errorf("busy processing")

	default:
		d.mu.Unlock()
		return "", fmt.Errorf("unknown state")
	}
}

// SwitchDevice updates the recording target by substring match.
func (d *Daemon) SwitchDevice(search string) (string, error) {
	devices, err := audio.ListDevices()
	if err != nil {
		return "", fmt.Errorf("list devices: %w", err)
	}

	target := audio.FindDevice(devices, search)
	if target == "" {
		return "", fmt.Errorf("device %q not found", search)
	}

	d.recorder.SetTarget(target)
	d.cfg.Set("DEVICE", search)
	return target, nil
}

// Devices returns all available audio input devices.
func (d *Daemon) Devices() ([]audio.AudioDevice, error) {
	return audio.ListDevices()
}

// State returns the current state.
func (d *Daemon) GetState() State {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state
}

// GetMode returns the current output mode.
func (d *Daemon) GetMode() OutputMode {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.mode
}

// SetMode changes the output mode.
func (d *Daemon) SetMode(m OutputMode) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.mode = m
}

func (d *Daemon) writeStatus() {
	path := filepath.Join(stateDir, "status")
	os.WriteFile(path, []byte(d.state.String()), 0o644)
}

func (d *Daemon) notify(s State) {
	select {
	case d.StateCh <- StateChange{State: s, Mode: d.mode}:
	default:
	}
}

func (d *Daemon) playSound(name string) {
	path := filepath.Join(d.soundDir, name)
	if _, err := os.Stat(path); err == nil {
		PlaySound(path)
	}
}

// GetCurrentDeviceTarget returns the current pw-record target node name.
func (d *Daemon) GetCurrentDeviceTarget() string {
	return d.recorder.GetTarget()
}

// Close cleans up the daemon.
func (d *Daemon) Close() {
	d.transcriber.Close()
	os.Remove(filepath.Join(stateDir, "status"))
}
