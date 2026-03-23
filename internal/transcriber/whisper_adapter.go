//go:build whisper

package transcriber

import (
	"fmt"
	"io"
	"sync"
	"time"

	whisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

// Audio processing constants
const (
	// minSampleRate is the minimum valid sample rate (8kHz)
	minSampleRate = 8000
	// maxSampleRate is the maximum valid sample rate (192kHz)
	maxSampleRate = 192000
	// maxChunkSamples limits individual AddAudio() calls (5 seconds at 192kHz)
	maxChunkSamples = maxSampleRate * 5
	// maxBufferSeconds limits accumulated audio buffer
	maxBufferSeconds = 30
	// transcribeTimeout limits how long a single transcription can take
	transcribeTimeout = 60 * time.Second
)

// validWhisperLanguages is the whitelist of supported language codes.
var validWhisperLanguages = map[string]bool{
	"auto": true, "en": true, "de": true, "es": true, "fr": true,
	"it": true, "pt": true, "nl": true, "pl": true, "ru": true,
	"zh": true, "ja": true, "ko": true, "ar": true, "hi": true,
	"tr": true, "vi": true, "th": true, "id": true, "ms": true,
	"sv": true, "da": true, "no": true, "fi": true, "cs": true,
	"sk": true, "ro": true, "hu": true, "el": true, "he": true,
	"uk": true, "bg": true, "hr": true, "lt": true, "lv": true,
	"sl": true, "et": true, "mt": true, "sq": true, "mk": true,
	"sr": true, "bs": true, "ca": true, "gl": true, "eu": true,
	"cy": true, "ga": true, "is": true, "af": true, "sw": true,
}

// WhisperTranscriber implements Transcriber using whisper.cpp.
type WhisperTranscriber struct {
	model    whisper.Model
	language string
	threads  int
	mu       sync.Mutex
}

// whisperStream implements Stream using chunked processing.
// Whisper doesn't have native streaming, so we simulate it.
type whisperStream struct {
	transcriber *WhisperTranscriber
	ctx         whisper.Context
	buffer      []float32    // Accumulated audio
	lastText    string       // Last transcription for change detection
	mu          sync.Mutex
	started     bool
}

// validateSampleRate checks that sample rate is within valid range.
func validateSampleRate(sampleRate int) error {
	if sampleRate < minSampleRate || sampleRate > maxSampleRate {
		return fmt.Errorf("invalid sample rate: %d (must be %d-%d)", sampleRate, minSampleRate, maxSampleRate)
	}
	return nil
}

// validateLanguage checks that language code is in the whitelist.
func validateLanguage(language string) error {
	if language == "" {
		return nil // Empty means auto-detect
	}
	if !validWhisperLanguages[language] {
		return fmt.Errorf("unsupported language: %s", language)
	}
	return nil
}

// NewWhisperTranscriber creates a transcriber using the Whisper backend.
func NewWhisperTranscriber(modelPath string, language string, threads int) (*WhisperTranscriber, error) {
	// Validate language before loading model
	if err := validateLanguage(language); err != nil {
		return nil, err
	}

	model, err := whisper.New(modelPath)
	if err != nil {
		return nil, fmt.Errorf("load whisper model: %w", err)
	}

	if threads <= 0 {
		threads = 4 // Default
	}
	if threads > 32 {
		threads = 32 // Cap at reasonable maximum
	}

	return &WhisperTranscriber{
		model:    model,
		language: language,
		threads:  threads,
	}, nil
}

// Transcribe performs single-shot transcription with timeout protection.
func (w *WhisperTranscriber) Transcribe(pcm []float32, sampleRate int) ([]TranscriptLine, error) {
	// Validate input
	if err := validateSampleRate(sampleRate); err != nil {
		return nil, err
	}
	if len(pcm) == 0 {
		return nil, fmt.Errorf("empty audio data")
	}
	// Limit input size (10 minutes at max sample rate)
	maxSamples := maxSampleRate * 600
	if len(pcm) > maxSamples {
		return nil, fmt.Errorf("audio too long: %d samples (max %d)", len(pcm), maxSamples)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	ctx, err := w.model.NewContext()
	if err != nil {
		return nil, fmt.Errorf("create context: %w", err)
	}

	// Configure context
	if w.language != "" {
		ctx.SetLanguage(w.language)
	}
	ctx.SetThreads(uint(w.threads))

	// Run transcription with timeout
	type result struct {
		lines []TranscriptLine
		err   error
	}
	resultCh := make(chan result, 1)

	go func() {
		// Process audio
		if err := ctx.Process(pcm, nil, nil, nil); err != nil {
			resultCh <- result{err: fmt.Errorf("process audio: %w", err)}
			return
		}

		// Collect segments
		var lines []TranscriptLine
		for {
			segment, err := ctx.NextSegment()
			if err == io.EOF {
				break
			}
			if err != nil {
				resultCh <- result{err: fmt.Errorf("read segment: %w", err)}
				return
			}

			lines = append(lines, TranscriptLine{
				Text:      segment.Text,
				StartTime: float32(segment.Start.Seconds()),
				Duration:  float32((segment.End - segment.Start).Seconds()),
			})
		}
		resultCh <- result{lines: lines}
	}()

	// Wait with timeout
	select {
	case res := <-resultCh:
		return res.lines, res.err
	case <-time.After(transcribeTimeout):
		return nil, fmt.Errorf("transcription timeout after %v", transcribeTimeout)
	}
}

// CreateStream creates a streaming session (simulated via chunked processing).
func (w *WhisperTranscriber) CreateStream() (Stream, error) {
	ctx, err := w.model.NewContext()
	if err != nil {
		return nil, fmt.Errorf("create context: %w", err)
	}

	if w.language != "" {
		ctx.SetLanguage(w.language)
	}
	ctx.SetThreads(uint(w.threads))

	return &whisperStream{
		transcriber: w,
		ctx:         ctx,
		buffer:      make([]float32, 0, 16000*30), // 30 seconds buffer
	}, nil
}

// SupportsStreaming returns false - Whisper uses chunked processing.
func (w *WhisperTranscriber) SupportsStreaming() bool {
	return false // Simulated streaming via chunks
}

// Close frees resources.
func (w *WhisperTranscriber) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.model.Close()
}

// Start begins the streaming session.
func (s *whisperStream) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = true
	s.buffer = s.buffer[:0]
	s.lastText = ""
	return nil
}

// Stop pauses the session.
func (s *whisperStream) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = false
	return nil
}

// Close frees resources.
func (s *whisperStream) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = false
	s.buffer = nil
}

// AddAudio feeds samples and returns transcript lines.
// Whisper processes in chunks, so we accumulate audio and periodically transcribe.
func (s *whisperStream) AddAudio(pcm []float32, sampleRate int) ([]StreamTranscriptLine, error) {
	// Validate input before acquiring lock
	if err := validateSampleRate(sampleRate); err != nil {
		return nil, err
	}
	// Reject oversized chunks to prevent memory exhaustion
	if len(pcm) > maxChunkSamples {
		return nil, fmt.Errorf("audio chunk too large: %d samples (max %d)", len(pcm), maxChunkSamples)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil, fmt.Errorf("stream not started")
	}

	// Calculate max buffer size BEFORE appending to prevent overflow
	maxSamples := sampleRate * maxBufferSeconds
	if len(s.buffer)+len(pcm) > maxSamples {
		// Trim buffer to make room for new samples
		excess := len(s.buffer) + len(pcm) - maxSamples
		if excess >= len(s.buffer) {
			// New chunk alone is larger than max; keep only end of new chunk
			s.buffer = s.buffer[:0]
			pcm = pcm[len(pcm)-maxSamples:]
		} else {
			s.buffer = s.buffer[excess:]
		}
	}

	// Accumulate audio
	s.buffer = append(s.buffer, pcm...)

	// Process when we have enough audio (at least 1 second)
	minSamples := sampleRate // 1 second
	if len(s.buffer) < minSamples {
		return nil, nil // Not enough audio yet
	}

	// Process accumulated audio
	if err := s.ctx.Process(s.buffer, nil, nil, nil); err != nil {
		return nil, fmt.Errorf("process audio: %w", err)
	}

	// Collect segments
	var lines []StreamTranscriptLine
	for {
		segment, err := s.ctx.NextSegment()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read segment: %w", err)
		}

		// Determine state flags
		isNew := s.lastText == ""
		hasChanged := segment.Text != s.lastText
		// Consider complete if we have significant silence or buffer is filling up
		isComplete := len(s.buffer) > sampleRate*5 // 5+ seconds accumulated

		lines = append(lines, StreamTranscriptLine{
			Text:           segment.Text,
			StartTime:      float32(segment.Start.Seconds()),
			Duration:       float32((segment.End - segment.Start).Seconds()),
			IsComplete:     isComplete,
			IsNew:          isNew,
			HasTextChanged: hasChanged,
		})

		s.lastText = segment.Text
	}

	// If we marked something as complete, clear the buffer for next utterance
	for _, l := range lines {
		if l.IsComplete {
			s.buffer = s.buffer[:0]
			s.lastText = ""
			break
		}
	}

	return lines, nil
}
