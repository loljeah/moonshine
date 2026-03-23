//go:build !whisper

package transcriber

import "fmt"

// WhisperTranscriber stub when whisper build tag is not set.
type WhisperTranscriber struct{}

// NewWhisperTranscriber returns an error when whisper support is not compiled in.
func NewWhisperTranscriber(modelPath string, language string, threads int) (*WhisperTranscriber, error) {
	return nil, fmt.Errorf("whisper backend not available: rebuild with -tags whisper")
}

// Transcribe is not available.
func (w *WhisperTranscriber) Transcribe(pcm []float32, sampleRate int) ([]TranscriptLine, error) {
	return nil, fmt.Errorf("whisper backend not available")
}

// CreateStream is not available.
func (w *WhisperTranscriber) CreateStream() (Stream, error) {
	return nil, fmt.Errorf("whisper backend not available")
}

// SupportsStreaming returns false.
func (w *WhisperTranscriber) SupportsStreaming() bool {
	return false
}

// Close does nothing.
func (w *WhisperTranscriber) Close() {}
