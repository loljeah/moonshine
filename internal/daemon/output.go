package daemon

import (
	"os/exec"
	"strings"
)

// CopyToClipboard copies text to the Wayland clipboard via wl-copy.
func CopyToClipboard(text string) error {
	cmd := exec.Command("wl-copy")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// TypeText types text into the focused window via wtype.
// Supports {KEY} placeholders for special keys (e.g., {Up}, {Down}, {Left}, {Right}).
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
				args = append(args, "-k", key)
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

	return exec.Command("wtype", args...).Run()
}

// Notify sends a desktop notification via notify-send.
func Notify(title, body string) error {
	return exec.Command("notify-send",
		"-a", "Moonshine",
		"-i", "microphone-sensitivity-high",
		title, body,
		"-t", "3000",
	).Run()
}

// PlaySound plays a WAV file via pw-play (non-blocking).
func PlaySound(path string) {
	cmd := exec.Command("pw-play", path)
	cmd.Start()
	// Don't wait — fire and forget
	go cmd.Wait()
}
