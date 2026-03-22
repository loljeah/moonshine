package config

import (
	"bufio"
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
}

// Load reads the config file at path. Missing file is not an error.
func Load(path string) (*Config, error) {
	c := &Config{
		path:   path,
		values: make(map[string]string),
	}

	f, err := os.Open(path)
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

// GetBool returns a boolean config value.
func (c *Config) GetBool(key string, defaultVal bool) bool {
	def := "off"
	if defaultVal {
		def = "on"
	}
	return c.Get(key, def) == "on"
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
