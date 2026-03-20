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
