// Package transcriber defines the abstract interface for speech-to-text backends.
// Supports multiple implementations: Moonshine (default) and Whisper (for German, etc.)
package transcriber

import "time"

// TranscriptLine represents a single transcription segment with timing.
type TranscriptLine struct {
	Text      string
	StartTime float32 // seconds
	Duration  float32 // seconds
}

// StreamTranscriptLine extends TranscriptLine with streaming state flags.
type StreamTranscriptLine struct {
	Text           string
	StartTime      float32
	Duration       float32
	IsComplete     bool // Sentence is finalized
	IsNew          bool // First occurrence of this segment
	HasTextChanged bool // Text was updated since last call
}

// Stream represents a streaming transcription session.
type Stream interface {
	// Start begins the streaming session.
	Start() error
	// Stop pauses the session (can be restarted).
	Stop() error
	// Close frees resources. Stream becomes unusable.
	Close()
	// AddAudio feeds PCM samples and returns current transcript lines.
	AddAudio(pcm []float32, sampleRate int) ([]StreamTranscriptLine, error)
}

// Transcriber is the abstract interface for speech-to-text backends.
type Transcriber interface {
	// Transcribe performs single-shot transcription on audio samples.
	// Input: float32 PCM samples at the given sample rate (typically 16000 Hz).
	// Returns ordered transcript lines with timing information.
	Transcribe(pcm []float32, sampleRate int) ([]TranscriptLine, error)

	// CreateStream creates a new streaming transcription session.
	CreateStream() (Stream, error)

	// SupportsStreaming returns true if the backend supports real-time streaming.
	// Whisper uses chunked processing (simulated streaming), Moonshine has native streaming.
	SupportsStreaming() bool

	// Close frees all resources.
	Close()
}

// Backend identifies the transcription backend type.
type Backend string

const (
	BackendMoonshine Backend = "moonshine"
	BackendWhisper   Backend = "whisper"
)

// Config holds configuration for creating a transcriber.
type Config struct {
	Backend   Backend
	ModelPath string
	Language  string // "en", "de", "es", etc.
	Threads   int    // Number of threads (Whisper only)
}

// SupportedLanguages returns languages supported by each backend.
func SupportedLanguages(backend Backend) []string {
	switch backend {
	case BackendMoonshine:
		return []string{"en", "es", "ar", "ja"}
	case BackendWhisper:
		// Whisper supports 99 languages including German
		return []string{"en", "de", "es", "fr", "it", "pt", "nl", "pl", "ru", "zh", "ja", "ko", "ar", "auto"}
	default:
		return nil
	}
}

// Duration converts float32 seconds to time.Duration
func Duration(seconds float32) time.Duration {
	return time.Duration(seconds * float32(time.Second))
}
