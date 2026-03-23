package audio

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	// deviceListTimeout prevents hangs when PipeWire is unresponsive
	deviceListTimeout = 5 * time.Second
)

// AudioDevice represents a PipeWire audio source node.
type AudioDevice struct {
	NodeName    string // e.g. "alsa_input.usb-Logitech_PRO_X_Wireless..."
	Description string // e.g. "PRO X Wireless Gaming Headset"
}

// ListDevices enumerates PipeWire Audio/Source nodes by parsing pw-cli output.
// Returns error if PipeWire is unavailable or times out.
func ListDevices() ([]AudioDevice, error) {
	ctx, cancel := context.WithTimeout(context.Background(), deviceListTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "pw-cli", "list-objects", "Node").Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("pw-cli timed out (PipeWire may be unresponsive)")
		}
		return nil, fmt.Errorf("pw-cli: %w", err)
	}

	return parseDevices(string(out)), nil
}

func parseDevices(output string) []AudioDevice {
	var devices []AudioDevice

	// Parse pw-cli output into blocks delimited by "id " or "\tid"
	blocks := splitBlocks(output)

	for _, block := range blocks {
		var name, desc, class string
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "media.class") {
				class = extractValue(line)
			} else if strings.Contains(line, "node.name") {
				name = extractValue(line)
			} else if strings.Contains(line, "node.description") {
				desc = extractValue(line)
			}
		}
		if class == "Audio/Source" && name != "" {
			devices = append(devices, AudioDevice{NodeName: name, Description: desc})
		}
	}

	return devices
}

func splitBlocks(output string) []string {
	var blocks []string
	var current strings.Builder

	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "id ") || strings.HasPrefix(line, "\tid") {
			if current.Len() > 0 {
				blocks = append(blocks, current.String())
				current.Reset()
			}
		}
		current.WriteString(line)
		current.WriteByte('\n')
	}
	if current.Len() > 0 {
		blocks = append(blocks, current.String())
	}

	return blocks
}

func extractValue(line string) string {
	// Handles: key = "value" or key = value
	idx := strings.Index(line, "=")
	if idx < 0 {
		return ""
	}
	val := strings.TrimSpace(line[idx+1:])
	val = strings.Trim(val, `"`)
	return val
}

// FindDevice finds a device whose NodeName or Description contains the
// given substring (case-insensitive). Returns empty string if not found.
func FindDevice(devices []AudioDevice, search string) string {
	if search == "" {
		return ""
	}
	lower := strings.ToLower(search)
	for _, d := range devices {
		if strings.Contains(strings.ToLower(d.NodeName), lower) ||
			strings.Contains(strings.ToLower(d.Description), lower) {
			return d.NodeName
		}
	}
	return ""
}
