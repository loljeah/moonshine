package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"moonshine-daemon/internal/audio"
	"moonshine-daemon/internal/config"
	"moonshine-daemon/internal/moonshine"
)

// State represents the daemon's current processing state.
type State int

const (
	StateIdle           State = iota
	StateRecording
	StateProcessing
	StateListening      // Free Speech: mic open, waiting for speech
	StateSpeechDetected // Free Speech: speech detected, transcribing
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateRecording:
		return "recording"
	case StateProcessing:
		return "processing"
	case StateListening:
		return "listening"
	case StateSpeechDetected:
		return "speech"
	default:
		return "unknown"
	}
}

// OutputMode determines where transcribed text goes.
type OutputMode int

const (
	ModeClipboard   OutputMode = iota
	ModeType
	ModeFreeSpeech
)

func (m OutputMode) String() string {
	switch m {
	case ModeType:
		return "type"
	case ModeFreeSpeech:
		return "free-speech"
	default:
		return "clipboard"
	}
}

// ParseOutputMode converts a string to OutputMode.
func ParseOutputMode(s string) OutputMode {
	switch s {
	case "type":
		return ModeType
	case "free-speech":
		return ModeFreeSpeech
	default:
		return ModeClipboard
	}
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

// historyPath returns the path to the transcription history file.
func historyPath() string {
	return filepath.Join(os.Getenv("HOME"), ".local", "share", "moonshine", "history.log")
}

// logTranscription appends a timestamped transcription to the history file.
func logTranscription(mode OutputMode, text string) {
	path := historyPath()
	os.MkdirAll(filepath.Dir(path), 0o755)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(f, "[%s] [%s] %s\n", ts, mode, text)
}

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

	// Free Speech mode
	streamRecorder *audio.StreamRecorder
	stream         *moonshine.Stream
	listenCancel   context.CancelFunc
	listening      bool

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

// Toggle starts or stops recording using the daemon's current output mode
// (set via SetMode or tray menu). Returns transcribed text on stop, or
// "recording" if recording just started. In Free Speech mode, toggles
// listening on/off.
func (d *Daemon) Toggle() (string, error) {
	d.mu.Lock()

	// Free Speech mode: toggle listening
	if d.mode == ModeFreeSpeech {
		if d.listening {
			d.mu.Unlock()
			d.StopListening()
			return "stopped", nil
		}
		d.mu.Unlock()
		if err := d.StartListening(); err != nil {
			return "", err
		}
		return "listening", nil
	}

	switch d.state {
	case StateIdle:
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
		logTranscription(currentMode, text)

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

// SetMode changes the output mode and notifies listeners (tray).
// Stops listening if leaving Free Speech mode.
func (d *Daemon) SetMode(m OutputMode) {
	d.mu.Lock()
	oldMode := d.mode
	d.mode = m
	d.mu.Unlock()

	// Stop listening if leaving Free Speech mode
	if oldMode == ModeFreeSpeech && m != ModeFreeSpeech {
		d.StopListening()
	}

	d.mu.Lock()
	state := d.state
	d.mu.Unlock()
	d.notify(state)
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

// StartListening begins the free-speech streaming loop.
func (d *Daemon) StartListening() error {
	d.mu.Lock()
	if d.listening {
		d.mu.Unlock()
		return nil
	}

	target := d.recorder.GetTarget()
	d.streamRecorder = audio.NewStreamRecorder(target)
	d.mu.Unlock()

	if err := d.streamRecorder.Start(); err != nil {
		return fmt.Errorf("start stream recorder: %w", err)
	}

	stream, err := d.transcriber.CreateStream()
	if err != nil {
		d.streamRecorder.Stop()
		return fmt.Errorf("create stream: %w", err)
	}

	if err := stream.Start(); err != nil {
		stream.Close()
		d.streamRecorder.Stop()
		return fmt.Errorf("start stream: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	d.mu.Lock()
	d.stream = stream
	d.listenCancel = cancel
	d.listening = true
	d.state = StateListening
	d.writeStatus()
	d.notify(StateListening)
	d.mu.Unlock()

	go d.streamingLoop(ctx)
	return nil
}

// StopListening stops the free-speech streaming loop.
func (d *Daemon) StopListening() {
	d.mu.Lock()
	if !d.listening {
		d.mu.Unlock()
		return
	}

	d.listening = false
	if d.listenCancel != nil {
		d.listenCancel()
		d.listenCancel = nil
	}
	d.mu.Unlock()

	// Stop recorder (unblocks ReadChunk)
	d.mu.Lock()
	sr := d.streamRecorder
	stream := d.stream
	d.streamRecorder = nil
	d.stream = nil
	d.mu.Unlock()

	if sr != nil {
		sr.Stop()
	}
	if stream != nil {
		stream.Stop()
		stream.Close()
	}

	d.mu.Lock()
	d.state = StateIdle
	d.writeStatus()
	d.notify(StateIdle)
	d.mu.Unlock()
}

// streamingLoop continuously reads audio and feeds it to the streaming transcriber.
func (d *Daemon) streamingLoop(ctx context.Context) {
	const chunkSize = 4800 // 300ms at 16kHz

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		d.mu.Lock()
		sr := d.streamRecorder
		stream := d.stream
		d.mu.Unlock()

		if sr == nil || stream == nil {
			return
		}

		chunk, err := sr.ReadChunk(chunkSize)
		if err != nil {
			// Recorder was stopped or pipe broken
			if d.verbose {
				log.Printf("streaming read: %s", err)
			}
			return
		}

		lines, err := stream.AddAudio(chunk, audio.SampleRate)
		if err != nil {
			if d.verbose {
				log.Printf("streaming transcribe: %s", err)
			}
			continue
		}

		for _, line := range lines {
			if line.IsNew {
				d.mu.Lock()
				if d.state != StateSpeechDetected {
					d.state = StateSpeechDetected
					d.writeStatus()
					d.notify(StateSpeechDetected)
				}
				d.mu.Unlock()
			}

			if line.IsComplete && line.Text != "" {
				if d.verbose {
					log.Printf("free-speech: %q", line.Text)
				}

				// Log and type text into focused window
				logTranscription(ModeFreeSpeech, line.Text)
				if err := TypeText(line.Text); err != nil {
					if d.verbose {
						log.Printf("free-speech type: %s", err)
					}
				}

				// Reset stream to clear completed transcript and prepare for next utterance
				stream.Stop()
				stream.Start()

				// Return to listening state
				d.mu.Lock()
				d.state = StateListening
				d.writeStatus()
				d.notify(StateListening)
				d.mu.Unlock()

				break // Exit line loop since stream was reset
			}
		}
	}
}

// Close cleans up the daemon.
func (d *Daemon) Close() {
	d.StopListening()
	d.transcriber.Close()
	os.Remove(filepath.Join(stateDir, "status"))
}
