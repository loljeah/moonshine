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

// Transcriber wraps the Moonshine C library. All C calls are serialized
// through a single goroutine to ensure thread safety.
type Transcriber struct {
	handle C.int32_t
	reqCh  chan request
	done   chan struct{}
}

type request struct {
	pcm        []float32
	sampleRate int
	result     chan transcribeResult
}

type transcribeResult struct {
	lines []TranscriptLine
	err   error
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
		reqCh:  make(chan request),
		done:   make(chan struct{}),
	}

	// Single goroutine for all C calls (thread safety)
	go t.run()

	return t, nil
}

func (t *Transcriber) run() {
	defer close(t.done)
	for req := range t.reqCh {
		lines, err := t.doTranscribe(req.pcm, req.sampleRate)
		req.result <- transcribeResult{lines: lines, err: err}
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

	if outTranscript == nil || outTranscript.line_count == 0 {
		return nil, nil
	}

	count := int(outTranscript.line_count)
	cLines := unsafe.Slice(outTranscript.lines, count)

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

	return lines, nil
}

// Transcribe sends audio PCM data for transcription. Thread-safe.
func (t *Transcriber) Transcribe(pcm []float32, sampleRate int) ([]TranscriptLine, error) {
	result := make(chan transcribeResult, 1)
	t.reqCh <- request{pcm: pcm, sampleRate: sampleRate, result: result}
	r := <-result
	return r.lines, r.err
}

// Close frees the transcriber resources.
func (t *Transcriber) Close() {
	close(t.reqCh)
	<-t.done // wait for goroutine to exit
	C.moonshine_free_transcriber(t.handle)
}
