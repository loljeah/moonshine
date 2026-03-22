package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

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
// FreeSpeech is now a separate toggle, not an output mode.
type OutputMode int

const (
	ModeClipboard OutputMode = iota
	ModeType
)

func (m OutputMode) String() string {
	switch m {
	case ModeType:
		return "type"
	default:
		return "clipboard"
	}
}

// ParseOutputMode converts a string to OutputMode.
func ParseOutputMode(s string) OutputMode {
	switch s {
	case "type":
		return ModeType
	default:
		return ModeClipboard
	}
}

// StateChange is sent to listeners (e.g. the tray) when state changes.
type StateChange struct {
	State      State
	Mode       OutputMode
	FreeSpeech bool // true if FreeSpeech toggle is enabled
	Enabled    bool // true if daemon is enabled (master toggle)
}

const (
	stateDir          = "/tmp/moonshine"
	soundsDir         = "/home" // overridden by actual path at runtime
	maxHistoryEntries = 50
)

// HistoryEntry represents a single transcription record.
type HistoryEntry struct {
	Time time.Time
	Mode OutputMode
	Text string
}

// historyPath returns the path to the transcription history file.
func historyPath() string {
	return filepath.Join(os.Getenv("HOME"), ".local", "share", "moonshine", "history.log")
}

// expandVoiceCommands replaces voice commands with their character equivalents.
// Case-insensitive matching. Special keys use {KEY} placeholder format.
func expandVoiceCommands(text string) string {
	// Order matters — longer phrases first to avoid partial matches
	replacements := []struct {
		phrase string
		char   string
	}{
		// Multi-word phrases first
		{"new paragraph", "\n\n"},
		{"newparagraph", "\n\n"},
		{"new line", "\n"},
		{"newline", "\n"},
		{"arrow down", "{Down}"},
		{"arrow up", "{Up}"},
		{"arrow left", "{Left}"},
		{"arrow right", "{Right}"},
		{"double ampersand", "&&"},
		{"double pipe", "||"},
		{"double equals", "=="},
		{"triple equals", "==="},
		{"not equals", "!="},
		{"open paren", "("},
		{"left paren", "("},
		{"close paren", ")"},
		{"right paren", ")"},
		{"open bracket", "["},
		{"left bracket", "["},
		{"close bracket", "]"},
		{"right bracket", "]"},
		{"open brace", "{"},
		{"left brace", "{"},
		{"close brace", "}"},
		{"right brace", "}"},
		{"double quote", "\""},
		{"single quote", "'"},
		{"dollar sign", "$"},
		{"question mark", "?"},
		{"exclamation point", "!"},
		{"exclamation mark", "!"},
		{"ellipsis", "..."},
		{"em dash", "—"},
		// Single words
		{"backspace", "{BackSpace}"},
		{"enter", "\n"},
		{"tab", "\t"},
		{"space", " "},
		{"equals", "="},
		{"colon", ":"},
		{"semicolon", ";"},
		{"comma", ","},
		{"period", "."},
		{"dot", "."},
		{"quote", "'"},
		{"underscore", "_"},
		{"plus", "+"},
		{"minus", "-"},
		{"hyphen", "-"},
		{"dash", "-"},
		{"asterisk", "*"},
		{"star", "*"},
		{"slash", "/"},
		{"backslash", "\\"},
		{"ampersand", "&"},
		{"pipe", "|"},
		{"hash", "#"},
		{"pound", "#"},
	}

	result := text

	for _, r := range replacements {
		// Find all occurrences (case-insensitive)
		idx := 0
		for {
			pos := strings.Index(strings.ToLower(result[idx:]), r.phrase)
			if pos < 0 {
				break
			}
			pos += idx
			// Replace preserving position
			result = result[:pos] + r.char + result[pos+len(r.phrase):]
			idx = pos + len(r.char)
		}
	}

	return result
}

// removeFillers removes common filler words and phrases from transcribed text.
// Uses word boundaries to avoid matching fillers inside valid words.
func removeFillers(text string) string {
	// Compile patterns for filler words/phrases (case-insensitive, whole words only)
	fillers := []string{
		`\bum\b`,
		`\buh\b`,
		`\ber\b`,
		`\bah\b`,
		`\byou know\b`,
		`\bi mean\b`,
	}

	result := text
	for _, pattern := range fillers {
		re := regexp.MustCompile(`(?i)` + pattern)
		result = re.ReplaceAllString(result, "")
	}

	// Collapse multiple spaces into single space
	spaceRe := regexp.MustCompile(`\s+`)
	result = spaceRe.ReplaceAllString(result, " ")

	// Trim leading/trailing whitespace
	return strings.TrimSpace(result)
}

// autoCapitalize capitalizes the first character and after sentence-ending punctuation.
// Also capitalizes standalone "i" to "I".
func autoCapitalize(text string) string {
	if text == "" {
		return text
	}

	// Capitalize standalone "i" -> "I"
	iRe := regexp.MustCompile(`\bi\b`)
	text = iRe.ReplaceAllString(text, "I")

	// Convert to runes for proper Unicode handling
	runes := []rune(text)

	// Capitalize first character
	if len(runes) > 0 && unicode.IsLetter(runes[0]) {
		runes[0] = unicode.ToUpper(runes[0])
	}

	// Capitalize after sentence-ending punctuation followed by space
	for i := 0; i < len(runes)-2; i++ {
		if (runes[i] == '.' || runes[i] == '?' || runes[i] == '!') &&
			runes[i+1] == ' ' && unicode.IsLetter(runes[i+2]) {
			runes[i+2] = unicode.ToUpper(runes[i+2])
		}
	}

	return string(runes)
}

// logTranscription appends a timestamped transcription to the history file
// and to the in-memory history buffer.
func (d *Daemon) logTranscription(mode OutputMode, text string) {
	now := time.Now()

	// Append to file
	path := historyPath()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintf(f, "[%s] [%s] %s\n", now.Format("2006-01-02 15:04:05"), mode, text)
		f.Close()
	}

	// Append to in-memory history
	d.mu.Lock()
	d.history = append(d.history, HistoryEntry{Time: now, Mode: mode, Text: text})
	if len(d.history) > maxHistoryEntries {
		d.history = d.history[len(d.history)-maxHistoryEntries:]
	}
	d.mu.Unlock()
}

// loadHistory reads existing entries from the history log file into memory.
func (d *Daemon) loadHistory() {
	data, err := os.ReadFile(historyPath())
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		// Format: [2006-01-02 15:04:05] [mode] text
		if len(line) < 24 || line[0] != '[' {
			continue
		}
		closeBracket := strings.Index(line, "]")
		if closeBracket < 0 {
			continue
		}
		ts, err := time.Parse("2006-01-02 15:04:05", line[1:closeBracket])
		if err != nil {
			continue
		}
		rest := line[closeBracket+2:] // skip "] "
		if len(rest) < 3 || rest[0] != '[' {
			continue
		}
		modeEnd := strings.Index(rest, "]")
		if modeEnd < 0 {
			continue
		}
		mode := ParseOutputMode(rest[1:modeEnd])
		text := ""
		if modeEnd+2 < len(rest) {
			text = rest[modeEnd+2:]
		}
		if text == "" {
			continue
		}
		d.history = append(d.history, HistoryEntry{Time: ts, Mode: mode, Text: text})
	}

	// Keep only the last maxHistoryEntries
	if len(d.history) > maxHistoryEntries {
		d.history = d.history[len(d.history)-maxHistoryEntries:]
	}
}

// History returns the transcription history, most recent first.
func (d *Daemon) History() []HistoryEntry {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]HistoryEntry, len(d.history))
	for i, e := range d.history {
		result[len(d.history)-1-i] = e
	}
	return result
}

// Daemon is the core state machine: idle -> recording -> processing -> idle.
type Daemon struct {
	mu          sync.Mutex
	state       State
	mode        OutputMode
	enabled     bool // master enable/disable toggle
	transcriber *moonshine.Transcriber
	recorder    *audio.Recorder
	cfg         *config.Config
	soundDir    string
	verbose     bool

	// Free Speech toggle (independent of output mode)
	freeSpeech     bool // true = always-listening enabled
	streamRecorder *audio.StreamRecorder
	stream         *moonshine.Stream
	listenCancel   context.CancelFunc
	listening      bool

	// Keep-alive for USB headset
	keepAliveCancel context.CancelFunc

	// Transcription history (most recent last, capped at maxHistoryEntries).
	history []HistoryEntry

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
		mode:        ModeType, // Default to Type mode (not Clipboard)
		enabled:     true,     // enabled by default
		transcriber: transcriber,
		recorder:    audio.NewRecorder(target),
		cfg:         cfg,
		soundDir:    soundDir,
		verbose:     verbose,
		StateCh:     make(chan StateChange, 4),
	}

	d.loadHistory()

	os.MkdirAll(stateDir, 0o755)
	d.writeStatus()

	// Start USB headset keep-alive loop
	d.startKeepAlive()

	return d
}

// Toggle starts or stops recording using the daemon's current output mode
// (set via SetMode or tray menu). Returns transcribed text on stop, or
// "recording" if recording just started. When FreeSpeech is enabled,
// toggle is ignored (use ToggleFreeSpeech instead).
func (d *Daemon) Toggle() (string, error) {
	d.mu.Lock()

	// Check if disabled
	if !d.enabled {
		d.mu.Unlock()
		return "", fmt.Errorf("disabled")
	}

	// When FreeSpeech is active, toggle controls listening not recording
	if d.freeSpeech {
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

		// Apply text processing pipeline
		text = removeFillers(text)
		text = expandVoiceCommands(text)
		text = autoCapitalize(text)

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
		d.logTranscription(currentMode, text)

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
// FreeSpeech is now a separate toggle, so mode changes don't affect it.
func (d *Daemon) SetMode(m OutputMode) {
	d.mu.Lock()
	d.mode = m
	state := d.state
	d.mu.Unlock()

	d.notify(state)
}

// SetFreeSpeech enables or disables the FreeSpeech toggle.
// When enabled, starts listening. When disabled, stops listening.
// Ignored if daemon is disabled.
func (d *Daemon) SetFreeSpeech(enabled bool) {
	d.mu.Lock()
	if !d.enabled && enabled {
		// Can't enable FreeSpeech while daemon is disabled
		d.mu.Unlock()
		return
	}
	if d.freeSpeech == enabled {
		d.mu.Unlock()
		return
	}
	d.freeSpeech = enabled
	d.mu.Unlock()

	if enabled {
		d.StartListening()
	} else {
		d.StopListening()
	}
}

// GetFreeSpeech returns the current FreeSpeech toggle state.
func (d *Daemon) GetFreeSpeech() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.freeSpeech
}

// SetEnabled enables or disables the entire daemon.
// When disabled, ignores all recording triggers and stops FreeSpeech.
func (d *Daemon) SetEnabled(enabled bool) {
	d.mu.Lock()
	if d.enabled == enabled {
		d.mu.Unlock()
		return
	}
	d.enabled = enabled
	d.mu.Unlock()

	if !enabled {
		// Stop FreeSpeech if running
		d.SetFreeSpeech(false)
	}

	d.mu.Lock()
	state := d.state
	d.mu.Unlock()
	d.notify(state)
}

// GetEnabled returns the master enable state.
func (d *Daemon) GetEnabled() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.enabled
}

func (d *Daemon) writeStatus() {
	path := filepath.Join(stateDir, "status")
	os.WriteFile(path, []byte(d.state.String()), 0o644)
}

func (d *Daemon) notify(s State) {
	select {
	case d.StateCh <- StateChange{State: s, Mode: d.mode, FreeSpeech: d.freeSpeech, Enabled: d.enabled}:
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
	d.freeSpeech = true // Ensure toggle is set when starting
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
	d.freeSpeech = false // Clear the toggle when stopping
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
// Handles device disconnects gracefully and attempts auto-recovery with exponential backoff.
// Never gives up - keeps trying until explicitly stopped via context cancellation.
func (d *Daemon) streamingLoop(ctx context.Context) {
	const chunkSize = 4800 // 300ms at 16kHz
	consecutiveErrors := 0

	defer func() {
		// Ensure state is properly cleaned up when loop exits
		d.mu.Lock()
		if d.listening {
			d.listening = false
			d.freeSpeech = false
			d.state = StateIdle
			d.writeStatus()
			d.notify(StateIdle)
			log.Printf("streaming loop exited, state reset to idle")
		}
		d.mu.Unlock()
	}()

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
			// Resources not ready, try to restart
			log.Printf("streaming resources nil, attempting restart...")
			if err := d.restartStreaming(); err != nil {
				log.Printf("restart failed: %s, will retry...", err)
				consecutiveErrors++
				d.waitWithBackoff(ctx, consecutiveErrors)
				continue
			}
			consecutiveErrors = 0
			continue
		}

		chunk, err := sr.ReadChunk(chunkSize)
		if err != nil {
			// Check if this was an intentional stop
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Recorder was stopped or pipe broken
			log.Printf("streaming read error: %s (device may have disconnected)", err)
			consecutiveErrors++

			// Notify user on first error
			if consecutiveErrors == 1 {
				Notify("Moonshine", "Microphone issue - reconnecting...")
			}

			// Clean up current resources
			d.cleanupStreamingResources()

			// Wait with exponential backoff
			d.waitWithBackoff(ctx, consecutiveErrors)

			// Try to restart
			if err := d.restartStreaming(); err != nil {
				log.Printf("restart failed (attempt %d): %s", consecutiveErrors, err)
				continue // Keep trying
			}

			// Success - reset error count and notify
			log.Printf("streaming restarted successfully after %d attempts", consecutiveErrors)
			if consecutiveErrors > 1 {
				Notify("Moonshine", "Microphone reconnected")
			}
			consecutiveErrors = 0
			continue
		}

		// Reset error count on successful read
		consecutiveErrors = 0

		lines, err := stream.AddAudio(chunk, audio.SampleRate)
		if err != nil {
			log.Printf("streaming transcribe error: %s", err)
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
				// Apply text processing pipeline
				text := removeFillers(line.Text)
				text = expandVoiceCommands(text)
				text = autoCapitalize(text)

				// Get current output mode
				d.mu.Lock()
				currentMode := d.mode
				d.mu.Unlock()

				if d.verbose {
					log.Printf("free-speech [%s]: %q -> %q", currentMode, line.Text, text)
				}

				// Log transcription
				d.logTranscription(currentMode, line.Text)

				// Output based on current mode
				switch currentMode {
				case ModeType:
					if err := TypeText(text); err != nil {
						log.Printf("free-speech type error: %s", err)
					}
				case ModeClipboard:
					if err := CopyToClipboard(text); err != nil {
						log.Printf("free-speech clipboard error: %s", err)
					}
					d.playSound("success.wav")
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

// waitWithBackoff waits with exponential backoff based on error count.
// Caps at 30 seconds. Can be cancelled via context.
func (d *Daemon) waitWithBackoff(ctx context.Context, errorCount int) {
	// Exponential backoff: 2s, 4s, 8s, 16s, 30s (capped)
	delay := time.Duration(1<<uint(errorCount)) * time.Second
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	if delay < 2*time.Second {
		delay = 2 * time.Second
	}

	log.Printf("waiting %v before retry...", delay)
	select {
	case <-ctx.Done():
	case <-time.After(delay):
	}
}

// cleanupStreamingResources stops current streaming without changing freeSpeech state.
func (d *Daemon) cleanupStreamingResources() {
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
}

// restartStreaming reinitializes the streaming components.
func (d *Daemon) restartStreaming() error {
	target := d.recorder.GetTarget()
	sr := audio.NewStreamRecorder(target)

	if err := sr.Start(); err != nil {
		return fmt.Errorf("start stream recorder: %w", err)
	}

	stream, err := d.transcriber.CreateStream()
	if err != nil {
		sr.Stop()
		return fmt.Errorf("create stream: %w", err)
	}

	if err := stream.Start(); err != nil {
		stream.Close()
		sr.Stop()
		return fmt.Errorf("start stream: %w", err)
	}

	d.mu.Lock()
	d.streamRecorder = sr
	d.stream = stream
	d.state = StateListening
	d.writeStatus()
	d.notify(StateListening)
	d.mu.Unlock()

	return nil
}

// Close cleans up the daemon.
func (d *Daemon) Close() {
	d.stopKeepAlive()
	d.StopListening()
	d.transcriber.Close()
	os.Remove(filepath.Join(stateDir, "status"))
}

// startKeepAlive starts a background goroutine that periodically pings
// the audio device to prevent USB headsets from going into power-save mode.
func (d *Daemon) startKeepAlive() {
	ctx, cancel := context.WithCancel(context.Background())
	d.mu.Lock()
	d.keepAliveCancel = cancel
	d.mu.Unlock()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		log.Printf("keep-alive: started (30s interval)")

		for {
			select {
			case <-ctx.Done():
				log.Printf("keep-alive: stopped")
				return
			case <-ticker.C:
				d.pingAudioDevice()
			}
		}
	}()
}

// stopKeepAlive stops the keep-alive goroutine.
func (d *Daemon) stopKeepAlive() {
	d.mu.Lock()
	if d.keepAliveCancel != nil {
		d.keepAliveCancel()
		d.keepAliveCancel = nil
	}
	d.mu.Unlock()
}

// pingAudioDevice sends a brief silent audio probe to keep the USB device awake.
// Also verifies the device is still accessible and logs any issues.
func (d *Daemon) pingAudioDevice() {
	target := d.recorder.GetTarget()
	if target == "" {
		return
	}

	// Check if device still exists
	devices, err := audio.ListDevices()
	if err != nil {
		log.Printf("keep-alive: failed to list devices: %s", err)
		return
	}

	found := false
	for _, dev := range devices {
		if dev.NodeName == target {
			found = true
			break
		}
	}

	if !found {
		log.Printf("keep-alive: WARNING - device %q not found! Available devices:", target)
		for _, dev := range devices {
			log.Printf("  - %s (%s)", dev.NodeName, dev.Description)
		}
		// Try to find a similar device
		newTarget := audio.FindDevice(devices, d.cfg.Device())
		if newTarget != "" && newTarget != target {
			log.Printf("keep-alive: switching to %s", newTarget)
			d.recorder.SetTarget(newTarget)
		}
		return
	}

	if d.verbose {
		log.Printf("keep-alive: device %s OK", target)
	}
}
