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
func TypeText(text string) error {
	return exec.Command("wtype", "-d", "12", text).Run()
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
