package transcriber

import (
	"fmt"

	"moonshine-daemon/internal/moonshine"
)

// New creates a Transcriber based on the configuration.
func New(cfg Config) (Transcriber, error) {
	switch cfg.Backend {
	case BackendMoonshine, "":
		return newMoonshineFromConfig(cfg)
	case BackendWhisper:
		return newWhisperFromConfig(cfg)
	default:
		return nil, fmt.Errorf("unknown backend: %s", cfg.Backend)
	}
}

// newMoonshineFromConfig creates a Moonshine transcriber.
func newMoonshineFromConfig(cfg Config) (Transcriber, error) {
	// Determine architecture based on language
	arch := moonshine.ArchMediumStreaming

	// Moonshine uses different model names per language
	// The model path should already include the language-specific directory
	return NewMoonshineTranscriber(cfg.ModelPath, arch)
}

// newWhisperFromConfig creates a Whisper transcriber.
func newWhisperFromConfig(cfg Config) (Transcriber, error) {
	threads := cfg.Threads
	if threads <= 0 {
		threads = 4
	}

	lang := cfg.Language
	if lang == "" {
		lang = "auto" // Auto-detect language
	}

	return NewWhisperTranscriber(cfg.ModelPath, lang, threads)
}

// RecommendBackend suggests the best backend for a given language.
func RecommendBackend(language string) Backend {
	switch language {
	case "en", "es", "ar", "ja":
		return BackendMoonshine // Native support
	case "de", "fr", "it", "pt", "nl", "pl", "ru", "zh", "ko":
		return BackendWhisper // Whisper supports these
	default:
		return BackendWhisper // Whisper has broader language support
	}
}

// IsLanguageSupported checks if a language is supported by a backend.
func IsLanguageSupported(backend Backend, language string) bool {
	for _, lang := range SupportedLanguages(backend) {
		if lang == language || lang == "auto" {
			return true
		}
	}
	return false
}
