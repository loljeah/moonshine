package daemon

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	// commandTimeout prevents hangs from unresponsive external tools
	commandTimeout = 10 * time.Second
)

// CopyToClipboard copies text to the Wayland clipboard via wl-copy.
// Returns error if the operation fails or times out.
func CopyToClipboard(text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "wl-copy")
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("wl-copy timed out")
		}
		return fmt.Errorf("wl-copy: %w", err)
	}
	return nil
}

// validSpecialKeys is the allowlist of valid wtype key names.
// This prevents potential injection via transcribed {KEY} placeholders.
var validSpecialKeys = map[string]bool{
	"Up": true, "Down": true, "Left": true, "Right": true,
	"BackSpace": true, "Delete": true, "Tab": true, "Return": true,
	"Home": true, "End": true, "Page_Up": true, "Page_Down": true,
	"Escape": true, "space": true,
}

// TypeText types text into the focused window via wtype.
// Supports {KEY} placeholders for special keys (e.g., {Up}, {Down}, {Left}, {Right}).
// Only allowlisted key names are accepted to prevent injection.
func TypeText(text string) error {
	// Parse text for {KEY} placeholders and build wtype args
	var args []string
	args = append(args, "-d", "12")

	i := 0
	for i < len(text) {
		// Look for {KEY} pattern
		if text[i] == '{' {
			end := strings.Index(text[i:], "}")
			if end > 1 {
				key := text[i+1 : i+end]
				// Only allow known safe key names
				if validSpecialKeys[key] {
					args = append(args, "-k", key)
				} else {
					// Unknown key - output literal text
					args = append(args, text[i:i+end+1])
				}
				i += end + 1
				continue
			}
		}

		// Find next { or end of string
		next := strings.Index(text[i:], "{")
		if next < 0 {
			// No more special keys, add rest as text
			args = append(args, text[i:])
			break
		} else if next > 0 {
			// Add text before next {
			args = append(args, text[i:i+next])
		}
		i += next
	}

	if len(args) == 2 {
		// Only -d 12, no content
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	if err := exec.CommandContext(ctx, "wtype", args...).Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("wtype timed out")
		}
		return fmt.Errorf("wtype: %w", err)
	}
	return nil
}

// Notify sends a desktop notification via notify-send.
// Non-blocking with timeout to prevent hangs.
func Notify(title, body string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return exec.CommandContext(ctx, "notify-send",
		"-a", "Moonshine",
		"-i", "microphone-sensitivity-high",
		title, body,
		"-t", "3000",
	).Run()
}

// PlaySound plays a WAV file via pw-play (non-blocking).
// Uses a timeout to prevent goroutine leaks if pw-play hangs.
func PlaySound(path string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	cmd := exec.CommandContext(ctx, "pw-play", path)
	if err := cmd.Start(); err != nil {
		cancel()
		return
	}
	go func() {
		defer cancel()
		cmd.Wait()
	}()
}
