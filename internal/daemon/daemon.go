package daemon

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"moonshine-daemon/internal/audio"
	"moonshine-daemon/internal/config"
	"moonshine-daemon/internal/transcriber"
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

// isWordBoundary reports whether position i in text is a word boundary.
// A word boundary exists at the start/end of string, or adjacent to a non-letter/non-digit character.
func isWordBoundary(text string, i int) bool {
	if i <= 0 || i >= len(text) {
		return true
	}
	r := rune(text[i])
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

// expandVoiceCommands replaces voice commands with their character equivalents.
// Case-insensitive matching with word boundary checks to prevent matching inside words
// (e.g., "star" inside "restart"). Special keys use {KEY} placeholder format.
// User-defined macros are applied after built-in commands.
func expandVoiceCommands(text string, macros map[string]string) string {
	// Order matters — longer phrases first to avoid partial matches
	replacements := []struct {
		phrase string
		char   string
	}{
		// Multi-word phrases first
		{"new paragraph", "\n\n"},
		{"new line", "\n"},
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
		{"euro sign", "€"},
		{"pound sign", "£"},
		{"yen sign", "¥"},
		{"cent sign", "¢"},
		{"bitcoin sign", "₿"},
		{"question mark", "?"},
		{"exclamation point", "!"},
		{"exclamation mark", "!"},
		{"ellipsis", "..."},
		{"em dash", "—"},
		// Single words — use precise technical terms only
		// Currency symbols
		{"dollar", "$"},
		{"euro", "€"},
		{"pound", "£"},
		{"yen", "¥"},
		{"cent", "¢"},
		{"bitcoin", "₿"},
		{"backspace", "{BackSpace}"},
		{"enter", "\n"},
		{"tab", "\t"},
		{"space", " "},
		{"equals", "="},
		{"colon", ":"},
		{"semicolon", ";"},
		{"comma", ", "},
		{"period", "."},
		{"quote", "'"},
		{"underscore", "_"},
		{"plus", "+"},
		{"minus", "-"},
		{"hyphen", "-"},
		{"dash", "-"},
		{"asterisk", "*"},
		{"slash", "/"},
		{"backslash", "\\"},
		{"ampersand", "&"},
		{"pipe", "|"},
		{"hash", "#"},
	}

	result := text

	for _, r := range replacements {
		idx := 0
		for {
			lower := strings.ToLower(result[idx:])
			pos := strings.Index(lower, r.phrase)
			if pos < 0 {
				break
			}
			absPos := pos + idx
			endPos := absPos + len(r.phrase)

			// Only replace if both boundaries are word boundaries
			if isWordBoundary(result, absPos) && isWordBoundary(result, endPos) {
				result = result[:absPos] + r.char + result[endPos:]
				idx = absPos + len(r.char)
			} else {
				// Skip past this match, not a word boundary
				idx = absPos + len(r.phrase)
			}
		}
	}

	// Apply user-defined macros (same word-boundary logic)
	for phrase, replacement := range macros {
		idx := 0
		for {
			lower := strings.ToLower(result[idx:])
			pos := strings.Index(lower, phrase)
			if pos < 0 {
				break
			}
			absPos := pos + idx
			endPos := absPos + len(phrase)

			if isWordBoundary(result, absPos) && isWordBoundary(result, endPos) {
				result = result[:absPos] + replacement + result[endPos:]
				idx = absPos + len(replacement)
			} else {
				idx = absPos + len(phrase)
			}
		}
	}

	return result
}

// Pre-compiled regexes for filler removal (avoid recompilation on every call)
// Organized by category for maintainability and potential future config options.
var (
	// Hesitation sounds - safe to always remove, these are pure disfluencies
	// Uses + quantifier to handle extended sounds (e.g., "uhhhhh", "ummmm")
	hesitationFillers = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bu+h+\b`),     // uh, uhh, uhhh
		regexp.MustCompile(`(?i)\bu+m+\b`),     // um, umm, ummm
		regexp.MustCompile(`(?i)\be+r+m?\b`),   // er, err, erm, errm
		regexp.MustCompile(`(?i)\ba+h+\b`),     // ah, ahh, ahhh
		regexp.MustCompile(`(?i)\bo+h+\b`),     // oh, ohh (standalone hesitation)
		regexp.MustCompile(`(?i)\be+h+\b`),     // eh, ehh
		regexp.MustCompile(`(?i)\bh+m+\b`),     // hm, hmm, hmmm
		regexp.MustCompile(`(?i)\bm+h+m+\b`),   // mhm, mhmm (listening sound)
	}

	// Agreement/acknowledgment sounds - also safe to remove in dictation context
	agreementFillers = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\buh[- ]?huh\b`), // uh-huh, uh huh
		regexp.MustCompile(`(?i)\buh[- ]?uh\b`),  // uh-uh (negative)
		regexp.MustCompile(`(?i)\bnuh[- ]?uh\b`), // nuh-uh (emphatic negative)
		regexp.MustCompile(`(?i)\bmm[- ]?hm+\b`), // mm-hm, mm-hmm
	}

	// Discourse markers - common verbal fillers that add little meaning
	discourseFillers = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\byou know\b`),
		regexp.MustCompile(`(?i)\bi mean\b`),
		regexp.MustCompile(`(?i)\bi guess\b`),
		regexp.MustCompile(`(?i)\byou see\b`),
		regexp.MustCompile(`(?i)\bsort of\b`),
		regexp.MustCompile(`(?i)\bkind of\b`),
	}

	// Combined list for standard filler removal (initialized in init())
	fillerRegexes []*regexp.Regexp

	multiSpaceRegex = regexp.MustCompile(`\s+`)

	// Filler + punctuation patterns (removes trailing comma/period after filler removal)
	trailingPunctRegex = regexp.MustCompile(`\s+([,.])\s*`)
	leadingPunctRegex  = regexp.MustCompile(`^\s*[,.]\s+`)
)

func init() {
	// Combine all filler categories into the main list
	// Order matters: longer/compound patterns first to prevent partial matches
	// e.g., "uh-huh" must be matched before "uh"
	fillerRegexes = append(fillerRegexes, agreementFillers...)  // compound patterns first
	fillerRegexes = append(fillerRegexes, discourseFillers...)  // multi-word phrases
	fillerRegexes = append(fillerRegexes, hesitationFillers...) // simple sounds last
}

// removeFillers removes common filler words and phrases from transcribed text.
// Uses word boundaries to avoid matching fillers inside valid words.
// Handles cleanup of orphaned punctuation after filler removal.
func removeFillers(text string) string {
	result := text

	// Remove all filler patterns
	for _, re := range fillerRegexes {
		result = re.ReplaceAllString(result, "")
	}

	// Clean up orphaned punctuation (e.g., "I, , think" -> "I, think")
	// Remove double punctuation
	result = regexp.MustCompile(`[,]+`).ReplaceAllString(result, ",")
	result = regexp.MustCompile(`[.]+`).ReplaceAllString(result, ".")

	// Remove leading punctuation at sentence start
	result = leadingPunctRegex.ReplaceAllString(result, "")

	// Collapse multiple spaces into single space
	result = multiSpaceRegex.ReplaceAllString(result, " ")

	// Trim leading/trailing whitespace
	return strings.TrimSpace(result)
}

// autoPunctuation adds punctuation based on speech patterns.
// Adds question marks when the sentence ends with interrogative words/patterns.
// Uses sentenceEnd as the default punctuation (typically "." but configurable).
func autoPunctuation(text string, sentenceEnd string) string {
	if text == "" {
		return text
	}

	// If text already ends with punctuation, leave it alone
	lastChar := text[len(text)-1]
	if lastChar == '.' || lastChar == '?' || lastChar == '!' || lastChar == ',' {
		return text
	}

	// Check for question patterns (case-insensitive)
	lower := strings.ToLower(text)

	// Question word at start
	questionStarters := []string{
		"what ", "where ", "when ", "why ", "who ", "whom ", "whose ",
		"which ", "how ", "is ", "are ", "was ", "were ", "do ", "does ",
		"did ", "can ", "could ", "will ", "would ", "should ", "shall ",
		"may ", "might ", "have ", "has ", "had ", "am ",
	}
	for _, q := range questionStarters {
		if strings.HasPrefix(lower, q) {
			return text + "? "
		}
	}

	// "... or not" pattern suggests a question
	if strings.HasSuffix(lower, " or not") {
		return text + "? "
	}

	// Tag questions at the end
	tagPatterns := []string{
		"right", "isn't it", "aren't they", "don't you", "doesn't it",
		"won't it", "can't you", "couldn't it", "shouldn't we", "huh",
	}
	for _, tag := range tagPatterns {
		if strings.HasSuffix(lower, " "+tag) || lower == tag {
			return text + "? "
		}
	}

	// Default: add configured sentence end punctuation with trailing space
	return text + sentenceEnd + " "
}

// numberWords maps spoken number words to their digit equivalents.
var numberWords = map[string]string{
	"zero": "0", "one": "1", "two": "2", "three": "3", "four": "4",
	"five": "5", "six": "6", "seven": "7", "eight": "8", "nine": "9",
	"ten": "10", "eleven": "11", "twelve": "12", "thirteen": "13",
	"fourteen": "14", "fifteen": "15", "sixteen": "16", "seventeen": "17",
	"eighteen": "18", "nineteen": "19", "twenty": "20", "thirty": "30",
	"forty": "40", "fifty": "50", "sixty": "60", "seventy": "70",
	"eighty": "80", "ninety": "90", "hundred": "100", "thousand": "1000",
	"million": "1000000", "billion": "1000000000",
}

// convertNumbersToDigits converts spelled-out numbers to digits.
// "twenty three" -> "23", "one hundred" -> "100"
func convertNumbersToDigits(text string) string {
	words := strings.Fields(text)
	result := make([]string, 0, len(words))

	i := 0
	for i < len(words) {
		word := strings.ToLower(strings.Trim(words[i], ".,!?;:"))
		originalWord := words[i]

		// Check if this word is a number word
		if _, isNum := numberWords[word]; isNum {
			// Collect consecutive number words
			numWords := []string{word}
			j := i + 1
			for j < len(words) {
				nextWord := strings.ToLower(strings.Trim(words[j], ".,!?;:"))
				if _, ok := numberWords[nextWord]; ok {
					numWords = append(numWords, nextWord)
					j++
				} else {
					break
				}
			}

			// Convert the number words to a single number
			num := parseNumberWords(numWords)
			result = append(result, fmt.Sprintf("%d", num))
			i = j
		} else {
			result = append(result, originalWord)
			i++
		}
	}

	return strings.Join(result, " ")
}

// parseNumberWords converts a sequence of number words to an integer.
func parseNumberWords(words []string) int {
	if len(words) == 0 {
		return 0
	}

	total := 0
	current := 0

	for _, word := range words {
		val, _ := numberWords[word]
		n := 0
		fmt.Sscanf(val, "%d", &n)

		switch {
		case n == 100:
			if current == 0 {
				current = 1
			}
			current *= 100
		case n == 1000:
			if current == 0 {
				current = 1
			}
			total += current * 1000
			current = 0
		case n >= 1000000:
			if current == 0 {
				current = 1
			}
			total += current * n
			current = 0
		default:
			current += n
		}
	}

	return total + current
}

// Pre-compiled regex for capitalizing standalone "i"
var standaloneIRegex = regexp.MustCompile(`\bi\b`)

// autoCapitalize capitalizes the first character and after sentence-ending punctuation.
// Also capitalizes standalone "i" to "I".
func autoCapitalize(text string) string {
	if text == "" {
		return text
	}

	// Capitalize standalone "i" -> "I"
	text = standaloneIRegex.ReplaceAllString(text, "I")

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

// processText applies the full text processing pipeline based on config settings.
func (d *Daemon) processText(text string) string {
	// 1. Remove filler words (um, uh, etc.)
	if d.cfg.FillerRemoval() {
		text = removeFillers(text)
	}

	// 2. Expand voice commands (new line, period, etc.) + user macros
	if d.cfg.VoiceCommands() {
		text = expandVoiceCommands(text, d.cfg.Macros())
	}

	// 3. Convert spoken numbers to digits
	if d.cfg.NumberFormat() == "digits" {
		text = convertNumbersToDigits(text)
	}

	// 4. Add automatic punctuation
	if d.cfg.AutoPunctuation() {
		text = autoPunctuation(text, d.cfg.SentenceEnd())
	}

	// 5. Auto-capitalize (after punctuation is added)
	if d.cfg.AutoCapitalize() {
		text = autoCapitalize(text)
	}

	return text
}

// logTranscription appends a timestamped transcription to the history file
// and to the in-memory history buffer.
func (d *Daemon) logTranscription(mode OutputMode, text string) {
	now := time.Now()

	// Append to file (restricted permissions - may contain sensitive transcriptions)
	path := historyPath()
	os.MkdirAll(filepath.Dir(path), 0o700)
	if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
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
	transcriber transcriber.Transcriber
	recorder    *audio.Recorder
	cfg         *config.Config
	soundDir    string
	verbose     bool

	// Free Speech toggle (independent of output mode)
	freeSpeech     bool // true = always-listening enabled
	streamRecorder *audio.StreamRecorder
	stream         transcriber.Stream
	listenCancel   context.CancelFunc
	listening      bool

	// Push-to-talk streaming (for silence auto-stop)
	pttRecorder *audio.StreamRecorder // streaming recorder for push-to-talk
	pttSamples  []float32             // accumulated PCM samples
	pttMu       sync.Mutex            // protects pttSamples
	pttCancel   context.CancelFunc    // cancels silence monitor goroutine

	// Keep-alive for USB headset
	keepAliveCancel context.CancelFunc

	// Transcription history (most recent last, capped at maxHistoryEntries).
	history []HistoryEntry

	// Last output tracking for "scratch that" undo
	lastOutputLen  int        // rune count of last typed/copied text
	lastOutputMode OutputMode // mode used for last output
	lastOutputText string     // raw text of last output (for clipboard re-copy)

	// StateCh broadcasts state changes (buffered, non-blocking send).
	StateCh chan StateChange
}

// New creates a Daemon with a loaded transcriber and config.
func New(trans transcriber.Transcriber, cfg *config.Config, soundDir string, verbose bool) *Daemon {
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
		transcriber: trans,
		recorder:    audio.NewRecorder(target),
		cfg:         cfg,
		soundDir:    soundDir,
		verbose:     verbose,
		StateCh:     make(chan StateChange, 4),
	}

	d.loadHistory()
	cfg.LoadMacros()

	os.MkdirAll(stateDir, 0o700) // Restrict to owner only (contains status info)
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

		silenceTimeout := d.cfg.SilenceTimeout()
		if silenceTimeout > 0 {
			Notify("Recording", fmt.Sprintf("Auto-stops after %ds silence", silenceTimeout))
		} else {
			Notify("Recording", "Press again to stop")
		}

		// Use streaming recorder for silence detection
		target := d.recorder.GetTarget()
		rec := audio.NewStreamRecorder(target)
		if err := rec.Start(); err != nil {
			d.mu.Lock()
			d.state = StateIdle
			d.writeStatus()
			d.notify(StateIdle)
			d.mu.Unlock()
			return "", fmt.Errorf("start recording: %w", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		d.pttMu.Lock()
		d.pttRecorder = rec
		d.pttSamples = nil
		d.pttCancel = cancel
		d.pttMu.Unlock()

		go d.pttSilenceMonitor(ctx)
		return "recording", nil

	case StateRecording:
		// Manual stop — finishPTTRecording handles everything
		d.mu.Unlock()
		d.finishPTTRecording()
		return "stopped", nil

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

// writeStatus atomically writes the current state to the status file.
// Uses write-to-temp-then-rename for atomic updates.
func (d *Daemon) writeStatus() {
	path := filepath.Join(stateDir, "status")
	tmpPath := path + ".tmp"

	// Write to temp file first
	if err := os.WriteFile(tmpPath, []byte(d.state.String()+"\n"), 0o600); err != nil {
		return
	}

	// Atomic rename
	os.Rename(tmpPath, path)
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

// GetBackend returns the current transcription backend ("moonshine" or "whisper").
func (d *Daemon) GetBackend() string {
	return d.cfg.Backend()
}

// GetLanguage returns the current transcription language.
func (d *Daemon) GetLanguage() string {
	return d.cfg.Language()
}

// SetBackendConfig updates the backend and language config and saves to disk.
// Returns error if save fails. Daemon restart required for changes to take effect.
func (d *Daemon) SetBackendConfig(backend, language string) error {
	d.cfg.Set("BACKEND", backend)
	d.cfg.Set("LANGUAGE", language)
	return d.cfg.Save()
}

// Config returns the daemon's config (for tray access).
func (d *Daemon) Config() interface{ Save() error } {
	return d.cfg
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
				// Apply text processing pipeline based on config
				text := d.processText(line.Text)

				// Check for "scratch that" / "undo that" control commands
				lowerText := strings.ToLower(strings.TrimSpace(text))
				if lowerText == "scratch that" || lowerText == "undo that" || lowerText == "scratch that." || lowerText == "undo that." {
					if _, err := d.ScratchThat(); err != nil {
						log.Printf("free-speech scratch error: %s", err)
					}
					// Reset stream and continue
					stream.Stop()
					stream.Start()
					d.mu.Lock()
					d.state = StateListening
					d.writeStatus()
					d.notify(StateListening)
					d.mu.Unlock()
					break
				}

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

				d.trackOutput(text, currentMode)

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

// ScratchThat undoes the last typed/copied transcription.
// In Type mode, sends backspaces to delete the text. In Clipboard mode, clears clipboard.
func (d *Daemon) ScratchThat() (string, error) {
	d.mu.Lock()
	outputLen := d.lastOutputLen
	outputMode := d.lastOutputMode
	d.lastOutputLen = 0 // Prevent double-scratch
	d.mu.Unlock()

	if outputLen == 0 {
		return "", fmt.Errorf("nothing to undo")
	}

	if outputMode == ModeType {
		if err := DeleteChars(outputLen); err != nil {
			return "", fmt.Errorf("delete: %w", err)
		}
	} else {
		CopyToClipboard("")
	}

	Notify("Scratch", fmt.Sprintf("Removed %d characters", outputLen))
	return fmt.Sprintf("scratched %d chars", outputLen), nil
}

// trackOutput records the last output for scratch-that undo.
func (d *Daemon) trackOutput(text string, mode OutputMode) {
	d.mu.Lock()
	d.lastOutputLen = len([]rune(text))
	d.lastOutputMode = mode
	d.lastOutputText = text
	d.mu.Unlock()
}

// rmsLevel calculates the root mean square of a float32 audio buffer.
func rmsLevel(samples []float32) float32 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range samples {
		sum += float64(s) * float64(s)
	}
	return float32(math.Sqrt(sum / float64(len(samples))))
}

// pttSilenceMonitor reads audio chunks, accumulates them, and auto-stops
// when silence is detected for the configured duration.
func (d *Daemon) pttSilenceMonitor(ctx context.Context) {
	const chunkSize = 4800 // 300ms at 16kHz
	const silenceThreshold = 0.005

	silenceTimeout := d.cfg.SilenceTimeout()
	if silenceTimeout <= 0 {
		// No silence detection, just accumulate samples until manual stop
		silenceTimeout = 0
	}
	maxSilenceChunks := (silenceTimeout * audio.SampleRate) / chunkSize // chunks of silence before auto-stop
	silentChunks := 0
	hadSpeech := false

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		d.pttMu.Lock()
		rec := d.pttRecorder
		d.pttMu.Unlock()

		if rec == nil {
			return
		}

		chunk, err := rec.ReadChunk(chunkSize)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if d.verbose {
				log.Printf("ptt monitor read error: %s", err)
			}
			return
		}

		// Accumulate samples
		d.pttMu.Lock()
		d.pttSamples = append(d.pttSamples, chunk...)
		d.pttMu.Unlock()

		// Check silence
		if silenceTimeout > 0 {
			level := rmsLevel(chunk)
			if level < silenceThreshold {
				silentChunks++
			} else {
				silentChunks = 0
				hadSpeech = true
			}

			// Auto-stop after configured silence duration (only if we had speech first)
			if hadSpeech && silentChunks >= maxSilenceChunks {
				if d.verbose {
					log.Printf("silence detected (%ds), auto-stopping", silenceTimeout)
				}
				go d.finishPTTRecording()
				return
			}
		}
	}
}

// finishPTTRecording stops the push-to-talk recording and processes the audio.
// Called either by manual Toggle() or by silence auto-stop.
func (d *Daemon) finishPTTRecording() {
	d.mu.Lock()
	if d.state != StateRecording {
		d.mu.Unlock()
		return
	}
	d.state = StateProcessing
	d.writeStatus()
	d.notify(StateProcessing)
	currentMode := d.mode
	d.mu.Unlock()

	d.playSound("stop.wav")

	// Cancel the silence monitor
	d.pttMu.Lock()
	if d.pttCancel != nil {
		d.pttCancel()
		d.pttCancel = nil
	}
	rec := d.pttRecorder
	d.pttRecorder = nil
	samples := d.pttSamples
	d.pttSamples = nil
	d.pttMu.Unlock()

	// Stop the stream recorder
	if rec != nil {
		rec.Stop()
	}

	if len(samples) < 1600 {
		d.mu.Lock()
		d.state = StateIdle
		d.writeStatus()
		d.notify(StateIdle)
		d.mu.Unlock()
		d.playSound("error.wav")
		Notify("No Audio", "Recording was too short")
		return
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
		return
	}

	// Collect text from lines
	var parts []string
	for _, l := range lines {
		if l.Text != "" {
			parts = append(parts, l.Text)
		}
	}
	text := strings.Join(parts, " ")

	// Apply text processing pipeline based on config
	text = d.processText(text)

	d.mu.Lock()
	d.state = StateIdle
	d.writeStatus()
	d.notify(StateIdle)
	d.mu.Unlock()

	if text == "" {
		d.playSound("error.wav")
		Notify("No Speech", "Couldn't detect any words")
		return
	}

	// Check for "scratch that" / "undo that" control commands
	lowerText := strings.ToLower(strings.TrimSpace(text))
	if lowerText == "scratch that" || lowerText == "undo that" || lowerText == "scratch that." || lowerText == "undo that." || lowerText == "scratch that. " || lowerText == "undo that. " {
		result, err := d.ScratchThat()
		if err != nil {
			d.playSound("error.wav")
			Notify("Scratch", err.Error())
			return
		}
		d.playSound("success.wav")
		_ = result
		return
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

	d.trackOutput(text, currentMode)
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
