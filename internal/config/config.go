package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// DefaultPath is the standard config file location.
var DefaultPath = filepath.Join(os.Getenv("HOME"), ".config", "moonshine", "config")

// Config provides thread-safe access to KEY=VALUE configuration.
type Config struct {
	mu     sync.RWMutex
	path   string
	values map[string]string
	macros map[string]string // user-defined voice macros (phrase -> replacement)
}

// Load reads the config file at path. Missing file is not an error.
// Path must be within the user's home directory or /tmp for security.
func Load(path string) (*Config, error) {
	// Validate path is within allowed directories
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("invalid config path: %w", err)
	}
	absPath = filepath.Clean(absPath)

	home := os.Getenv("HOME")
	allowedPrefixes := []string{
		filepath.Clean(home),
		"/tmp",
	}

	allowed := false
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(absPath, prefix+string(filepath.Separator)) || absPath == prefix {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, fmt.Errorf("config path must be within home directory or /tmp")
	}

	c := &Config{
		path:   absPath,
		values: make(map[string]string),
	}

	f, err := os.Open(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		c.values[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}

	return c, scanner.Err()
}

// Get returns the value for key, or defaultVal if not set.
func (c *Config) Get(key, defaultVal string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if v, ok := c.values[key]; ok && v != "" {
		return v
	}
	return defaultVal
}

// Set updates a key in memory. Does not write to disk.
func (c *Config) Set(key, val string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = val
}

// Save writes the current config to disk.
func (c *Config) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Ensure directory exists
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// Build config content
	var lines []string
	for k, v := range c.values {
		lines = append(lines, k+"="+v)
	}

	// Write atomically (temp file + rename)
	tmpPath := c.path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmpPath, c.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

// Device returns the configured audio input device substring.
func (c *Config) Device() string {
	return c.Get("DEVICE", "")
}

// Language returns the configured transcription language.
func (c *Config) Language() string {
	return c.Get("LANGUAGE", "en")
}

// AutoPunctuation returns whether to auto-insert punctuation.
func (c *Config) AutoPunctuation() bool {
	return c.Get("AUTO_PUNCTUATION", "on") == "on"
}

// NumberFormat returns "digits" to convert "twenty three" -> "23", or "words" to keep as-is.
func (c *Config) NumberFormat() string {
	return c.Get("NUMBER_FORMAT", "words")
}

// FillerRemoval returns whether to remove filler words (um, uh, etc).
func (c *Config) FillerRemoval() bool {
	return c.Get("FILLER_REMOVAL", "on") == "on"
}

// VoiceCommands returns whether to expand voice commands (new line, period, etc).
func (c *Config) VoiceCommands() bool {
	return c.Get("VOICE_COMMANDS", "on") == "on"
}

// AutoCapitalize returns whether to auto-capitalize sentences.
func (c *Config) AutoCapitalize() bool {
	return c.Get("AUTO_CAPITALIZE", "on") == "on"
}

// SentenceEnd returns the default punctuation for sentence endings.
// Options: "." (period), "" (none), or any single character.
// Default is "." (period). Set to "" or "none" to disable.
func (c *Config) SentenceEnd() string {
	val := c.Get("SENTENCE_END", ".")
	if val == "none" {
		return ""
	}
	return val
}

// SilenceTimeout returns the number of seconds of silence before auto-stopping
// push-to-talk recording. Returns 0 to disable (manual stop only).
func (c *Config) SilenceTimeout() int {
	val := c.Get("SILENCE_TIMEOUT", "3")
	n := 3
	fmt.Sscanf(val, "%d", &n)
	if n < 0 {
		n = 0
	}
	if n > 30 {
		n = 30
	}
	return n
}

// Backend returns the transcription backend ("moonshine" or "whisper").
func (c *Config) Backend() string {
	return c.Get("BACKEND", "moonshine")
}

// WhisperModel returns the path to the Whisper model file.
func (c *Config) WhisperModel() string {
	return c.Get("WHISPER_MODEL", "")
}

// Threads returns the number of threads for transcription (Whisper).
func (c *Config) Threads() int {
	val := c.Get("THREADS", "4")
	n := 4
	fmt.Sscanf(val, "%d", &n)
	if n < 1 {
		n = 1
	}
	if n > 32 {
		n = 32
	}
	return n
}

// GetBool returns a boolean config value.
func (c *Config) GetBool(key string, defaultVal bool) bool {
	def := "off"
	if defaultVal {
		def = "on"
	}
	return c.Get(key, def) == "on"
}

// Mode returns the default output mode ("type" or "clipboard").
func (c *Config) Mode() string {
	return c.Get("MODE", "type")
}

// AutoListen returns whether to start free-speech listening on daemon startup.
func (c *Config) AutoListen() bool {
	return c.Get("AUTO_LISTEN", "off") == "on"
}

// All returns a copy of all config values.
func (c *Config) All() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]string, len(c.values))
	for k, v := range c.values {
		result[k] = v
	}
	return result
}

// Macros returns the user-defined voice macro map.
// Returns nil if no macros are loaded.
func (c *Config) Macros() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.macros
}

// LoadMacros reads user-defined voice macros from the macros file
// next to the config file. Format: "phrase = replacement" per line.
// Lines starting with # are comments. Missing file is not an error.
func (c *Config) LoadMacros() {
	macroPath := filepath.Join(filepath.Dir(c.path), "macros")

	f, err := os.Open(macroPath)
	if err != nil {
		return // File doesn't exist, no macros
	}
	defer f.Close()

	macros := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		phrase := strings.TrimSpace(strings.ToLower(key))
		replacement := strings.TrimSpace(val)
		if phrase != "" && replacement != "" {
			macros[phrase] = replacement
		}
	}

	c.mu.Lock()
	c.macros = macros
	c.mu.Unlock()
}
