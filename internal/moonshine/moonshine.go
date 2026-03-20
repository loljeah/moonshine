package moonshine

/*
#cgo LDFLAGS: -lmoonshine

#include <stdlib.h>
#include <stdint.h>

// transcript_line_t matches the C struct from moonshine-c-api.h
typedef struct {
	char *text;
	float *audio_data;
	size_t audio_data_count;
	float start_time;
	float duration;
	uint64_t id;
	int8_t is_complete;
	int8_t is_updated;
	int8_t is_new;
	int8_t has_text_changed;
	int8_t has_speaker_id;
	uint64_t speaker_id;
	uint32_t speaker_index;
	uint32_t last_transcription_latency_ms;
} transcript_line_t;

// transcript_t matches the C struct
typedef struct {
	transcript_line_t *lines;
	uint64_t line_count;
} transcript_t;

// transcriber_option_t matches the C struct
typedef struct {
	const char *name;
	const char *value;
} transcriber_option_t;

// Model architectures
enum model_arch {
	MODEL_ARCH_TINY = 0,
	MODEL_ARCH_BASE = 1,
	MODEL_ARCH_TINY_STREAMING = 2,
	MODEL_ARCH_BASE_STREAMING = 3,
	MODEL_ARCH_SMALL_STREAMING = 4,
	MODEL_ARCH_MEDIUM_STREAMING = 5
};

// C API functions
extern int32_t moonshine_get_version();
extern const char* moonshine_error_to_string(int32_t error_code);
extern int32_t moonshine_load_transcriber_from_files(
	const char *path,
	uint32_t model_arch,
	transcriber_option_t *options,
	uint64_t options_count,
	int32_t moonshine_version
);
extern void moonshine_free_transcriber(int32_t handle);
extern int32_t moonshine_transcribe_without_streaming(
	int32_t transcriber_handle,
	float *audio_data,
	uint64_t audio_length,
	int32_t sample_rate,
	uint32_t flags,
	transcript_t **out_transcript
);

// Streaming API
extern int32_t moonshine_create_stream(int32_t transcriber_handle, uint32_t flags);
extern int32_t moonshine_start_stream(int32_t transcriber_handle, int32_t stream_handle);
extern int32_t moonshine_stop_stream(int32_t transcriber_handle, int32_t stream_handle);
extern int32_t moonshine_transcribe_add_audio_to_stream(
	int32_t transcriber_handle,
	int32_t stream_handle,
	float *audio_data,
	uint64_t audio_length,
	int32_t sample_rate,
	uint32_t flags
);
extern int32_t moonshine_transcribe_stream(
	int32_t transcriber_handle,
	int32_t stream_handle,
	uint32_t flags,
	transcript_t **out_transcript
);
extern void moonshine_free_stream(int32_t transcriber_handle, int32_t stream_handle);
*/
import "C"

import (
	"fmt"
	"strings"
	"unsafe"
)

const headerVersion = 20000

type ModelArch uint32

const (
	ArchTiny            ModelArch = C.MODEL_ARCH_TINY
	ArchBase            ModelArch = C.MODEL_ARCH_BASE
	ArchTinyStreaming    ModelArch = C.MODEL_ARCH_TINY_STREAMING
	ArchBaseStreaming    ModelArch = C.MODEL_ARCH_BASE_STREAMING
	ArchSmallStreaming   ModelArch = C.MODEL_ARCH_SMALL_STREAMING
	ArchMediumStreaming  ModelArch = C.MODEL_ARCH_MEDIUM_STREAMING
)

// ParseModelArch converts a string like "medium-streaming" to a ModelArch.
func ParseModelArch(s string) (ModelArch, error) {
	switch s {
	case "tiny":
		return ArchTiny, nil
	case "base":
		return ArchBase, nil
	case "tiny-streaming":
		return ArchTinyStreaming, nil
	case "base-streaming":
		return ArchBaseStreaming, nil
	case "small-streaming":
		return ArchSmallStreaming, nil
	case "medium-streaming":
		return ArchMediumStreaming, nil
	default:
		return 0, fmt.Errorf("unknown model arch: %s", s)
	}
}

// TranscriptLine is a single line of transcription output.
type TranscriptLine struct {
	Text      string
	StartTime float32
	Duration  float32
}

// StreamTranscriptLine extends TranscriptLine with streaming flags.
type StreamTranscriptLine struct {
	Text           string
	StartTime      float32
	Duration       float32
	IsComplete     bool
	IsNew          bool
	HasTextChanged bool
}

// Transcriber wraps the Moonshine C library. All C calls are serialized
// through a single goroutine to ensure thread safety.
type Transcriber struct {
	handle C.int32_t
	funcCh chan func()
	done   chan struct{}
}

// NewTranscriber loads a model from the given path with the specified architecture.
func NewTranscriber(modelPath string, arch ModelArch) (*Transcriber, error) {
	cPath := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cPath))

	handle := C.moonshine_load_transcriber_from_files(
		cPath,
		C.uint32_t(arch),
		nil, // no options
		0,
		C.int32_t(headerVersion),
	)

	if handle < 0 {
		errStr := C.moonshine_error_to_string(handle)
		return nil, fmt.Errorf("failed to load transcriber: %s", C.GoString(errStr))
	}

	t := &Transcriber{
		handle: handle,
		funcCh: make(chan func()),
		done:   make(chan struct{}),
	}

	// Single goroutine for all C calls (thread safety)
	go t.run()

	return t, nil
}

func (t *Transcriber) run() {
	defer close(t.done)
	for fn := range t.funcCh {
		fn()
	}
}

func (t *Transcriber) doTranscribe(pcm []float32, sampleRate int) ([]TranscriptLine, error) {
	if len(pcm) == 0 {
		return nil, fmt.Errorf("empty audio data")
	}

	var outTranscript *C.transcript_t

	rc := C.moonshine_transcribe_without_streaming(
		t.handle,
		(*C.float)(unsafe.Pointer(&pcm[0])),
		C.uint64_t(len(pcm)),
		C.int32_t(sampleRate),
		0, // flags
		&outTranscript,
	)

	if rc < 0 {
		errStr := C.moonshine_error_to_string(rc)
		return nil, fmt.Errorf("transcription failed: %s", C.GoString(errStr))
	}

	return parseTranscript(outTranscript), nil
}

// parseTranscript converts a C transcript_t to Go TranscriptLines.
func parseTranscript(t *C.transcript_t) []TranscriptLine {
	if t == nil || t.line_count == 0 {
		return nil
	}

	count := int(t.line_count)
	cLines := unsafe.Slice(t.lines, count)

	lines := make([]TranscriptLine, 0, count)
	for _, cl := range cLines {
		text := C.GoString(cl.text)
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		lines = append(lines, TranscriptLine{
			Text:      text,
			StartTime: float32(cl.start_time),
			Duration:  float32(cl.duration),
		})
	}

	return lines
}

// parseStreamTranscript converts a C transcript_t to StreamTranscriptLines.
func parseStreamTranscript(t *C.transcript_t) []StreamTranscriptLine {
	if t == nil || t.line_count == 0 {
		return nil
	}

	count := int(t.line_count)
	cLines := unsafe.Slice(t.lines, count)

	lines := make([]StreamTranscriptLine, 0, count)
	for _, cl := range cLines {
		text := C.GoString(cl.text)
		text = strings.TrimSpace(text)
		lines = append(lines, StreamTranscriptLine{
			Text:           text,
			StartTime:      float32(cl.start_time),
			Duration:       float32(cl.duration),
			IsComplete:     cl.is_complete != 0,
			IsNew:          cl.is_new != 0,
			HasTextChanged: cl.has_text_changed != 0,
		})
	}

	return lines
}

// Transcribe sends audio PCM data for transcription. Thread-safe.
func (t *Transcriber) Transcribe(pcm []float32, sampleRate int) ([]TranscriptLine, error) {
	type result struct {
		lines []TranscriptLine
		err   error
	}
	ch := make(chan result, 1)
	t.funcCh <- func() {
		lines, err := t.doTranscribe(pcm, sampleRate)
		ch <- result{lines, err}
	}
	r := <-ch
	return r.lines, r.err
}

// Stream wraps a Moonshine streaming session. All methods are thread-safe
// (they dispatch to the Transcriber's single C goroutine).
type Stream struct {
	t      *Transcriber
	handle C.int32_t
}

// CreateStream creates a new streaming transcription session.
func (t *Transcriber) CreateStream() (*Stream, error) {
	type result struct {
		handle C.int32_t
		err    error
	}
	ch := make(chan result, 1)
	t.funcCh <- func() {
		h := C.moonshine_create_stream(t.handle, 0)
		if h < 0 {
			errStr := C.moonshine_error_to_string(h)
			ch <- result{err: fmt.Errorf("create stream: %s", C.GoString(errStr))}
		} else {
			ch <- result{handle: h}
		}
	}
	r := <-ch
	if r.err != nil {
		return nil, r.err
	}
	return &Stream{t: t, handle: r.handle}, nil
}

// Start begins the streaming session.
func (s *Stream) Start() error {
	ch := make(chan error, 1)
	s.t.funcCh <- func() {
		rc := C.moonshine_start_stream(s.t.handle, s.handle)
		if rc < 0 {
			errStr := C.moonshine_error_to_string(rc)
			ch <- fmt.Errorf("start stream: %s", C.GoString(errStr))
		} else {
			ch <- nil
		}
	}
	return <-ch
}

// Stop ends the streaming session (can be restarted).
func (s *Stream) Stop() error {
	ch := make(chan error, 1)
	s.t.funcCh <- func() {
		rc := C.moonshine_stop_stream(s.t.handle, s.handle)
		if rc < 0 {
			errStr := C.moonshine_error_to_string(rc)
			ch <- fmt.Errorf("stop stream: %s", C.GoString(errStr))
		} else {
			ch <- nil
		}
	}
	return <-ch
}

// Close frees the stream. Must not be used after Close.
func (s *Stream) Close() {
	done := make(chan struct{}, 1)
	s.t.funcCh <- func() {
		C.moonshine_free_stream(s.t.handle, s.handle)
		done <- struct{}{}
	}
	<-done
}

// AddAudio feeds PCM audio to the stream and returns updated transcript lines.
func (s *Stream) AddAudio(pcm []float32, sampleRate int) ([]StreamTranscriptLine, error) {
	if len(pcm) == 0 {
		return nil, nil
	}

	type result struct {
		lines []StreamTranscriptLine
		err   error
	}
	ch := make(chan result, 1)
	s.t.funcCh <- func() {
		// Add audio to stream
		rc := C.moonshine_transcribe_add_audio_to_stream(
			s.t.handle,
			s.handle,
			(*C.float)(unsafe.Pointer(&pcm[0])),
			C.uint64_t(len(pcm)),
			C.int32_t(sampleRate),
			0,
		)
		if rc < 0 {
			errStr := C.moonshine_error_to_string(rc)
			ch <- result{err: fmt.Errorf("add audio: %s", C.GoString(errStr))}
			return
		}

		// Get transcript
		var outTranscript *C.transcript_t
		rc = C.moonshine_transcribe_stream(
			s.t.handle,
			s.handle,
			0,
			&outTranscript,
		)
		if rc < 0 {
			errStr := C.moonshine_error_to_string(rc)
			ch <- result{err: fmt.Errorf("transcribe stream: %s", C.GoString(errStr))}
			return
		}

		ch <- result{lines: parseStreamTranscript(outTranscript)}
	}
	r := <-ch
	return r.lines, r.err
}

// Close frees the transcriber resources.
func (t *Transcriber) Close() {
	close(t.funcCh)
	<-t.done // wait for goroutine to exit
	C.moonshine_free_transcriber(t.handle)
}
