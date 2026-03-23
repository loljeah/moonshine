package transcriber

import (
	"fmt"

	"moonshine-daemon/internal/moonshine"
)

// Audio processing constants (shared with whisper_adapter.go)
const (
	minSampleRateMoonshine = 8000
	maxSampleRateMoonshine = 192000
)

// MoonshineTranscriber wraps the existing Moonshine implementation.
type MoonshineTranscriber struct {
	inner *moonshine.Transcriber
}

// moonshineStream wraps the Moonshine stream.
type moonshineStream struct {
	inner *moonshine.Stream
}

// NewMoonshineTranscriber creates a transcriber using the Moonshine backend.
func NewMoonshineTranscriber(modelPath string, arch moonshine.ModelArch) (*MoonshineTranscriber, error) {
	t, err := moonshine.NewTranscriber(modelPath, arch)
	if err != nil {
		return nil, err
	}
	return &MoonshineTranscriber{inner: t}, nil
}

// Transcribe performs single-shot transcription.
func (m *MoonshineTranscriber) Transcribe(pcm []float32, sampleRate int) ([]TranscriptLine, error) {
	// Validate sample rate
	if sampleRate < minSampleRateMoonshine || sampleRate > maxSampleRateMoonshine {
		return nil, fmt.Errorf("invalid sample rate: %d (must be %d-%d)", sampleRate, minSampleRateMoonshine, maxSampleRateMoonshine)
	}
	if len(pcm) == 0 {
		return nil, fmt.Errorf("empty audio data")
	}

	lines, err := m.inner.Transcribe(pcm, sampleRate)
	if err != nil {
		return nil, err
	}

	result := make([]TranscriptLine, len(lines))
	for i, l := range lines {
		result[i] = TranscriptLine{
			Text:      l.Text,
			StartTime: l.StartTime,
			Duration:  l.Duration,
		}
	}
	return result, nil
}

// CreateStream creates a streaming session.
func (m *MoonshineTranscriber) CreateStream() (Stream, error) {
	s, err := m.inner.CreateStream()
	if err != nil {
		return nil, err
	}
	return &moonshineStream{inner: s}, nil
}

// SupportsStreaming returns true - Moonshine has native streaming.
func (m *MoonshineTranscriber) SupportsStreaming() bool {
	return true
}

// Close frees resources.
func (m *MoonshineTranscriber) Close() {
	m.inner.Close()
}

// Start begins the streaming session.
func (s *moonshineStream) Start() error {
	return s.inner.Start()
}

// Stop pauses the session.
func (s *moonshineStream) Stop() error {
	return s.inner.Stop()
}

// Close frees resources.
func (s *moonshineStream) Close() {
	s.inner.Close()
}

// AddAudio feeds samples and returns transcript lines.
func (s *moonshineStream) AddAudio(pcm []float32, sampleRate int) ([]StreamTranscriptLine, error) {
	// Validate sample rate
	if sampleRate < minSampleRateMoonshine || sampleRate > maxSampleRateMoonshine {
		return nil, fmt.Errorf("invalid sample rate: %d (must be %d-%d)", sampleRate, minSampleRateMoonshine, maxSampleRateMoonshine)
	}

	lines, err := s.inner.AddAudio(pcm, sampleRate)
	if err != nil {
		return nil, err
	}

	result := make([]StreamTranscriptLine, len(lines))
	for i, l := range lines {
		result[i] = StreamTranscriptLine{
			Text:           l.Text,
			StartTime:      l.StartTime,
			Duration:       l.Duration,
			IsComplete:     l.IsComplete,
			IsNew:          l.IsNew,
			HasTextChanged: l.HasTextChanged,
		}
	}
	return result, nil
}
