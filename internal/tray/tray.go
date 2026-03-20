package tray

import (
	"moonshine-daemon/internal/daemon"

	"fyne.io/systray"
)

// Tray manages the system tray icon and menu.
type Tray struct {
	d       *daemon.Daemon
	verbose bool

	// Menu items we need to update
	mStatus     *systray.MenuItem
	mClipboard  *systray.MenuItem
	mType       *systray.MenuItem
	mDevices    []*systray.MenuItem
	mDeviceSub  *systray.MenuItem
}

// Run starts the system tray. Blocks until quit is selected.
// Call from the main goroutine.
func Run(d *daemon.Daemon, verbose bool) {
	t := &Tray{d: d, verbose: verbose}
	systray.Run(t.onReady, t.onExit)
}

func (t *Tray) onReady() {
	systray.SetIcon(IconIdle)
	systray.SetTitle("Moonshine")
	systray.SetTooltip("Moonshine Voice-to-Text")

	// Status (disabled, display-only)
	t.mStatus = systray.AddMenuItem("Status: Idle", "Current state")
	t.mStatus.Disable()

	systray.AddSeparator()

	// Output mode radio group
	mOutputMode := systray.AddMenuItem("Output Mode", "")
	mOutputMode.Disable()
	t.mClipboard = mOutputMode.AddSubMenuItem("Clipboard", "Copy to clipboard")
	t.mType = mOutputMode.AddSubMenuItem("Type", "Type into focused window")
	t.mClipboard.Check()

	systray.AddSeparator()

	// Device submenu
	t.mDeviceSub = systray.AddMenuItem("Device", "Audio input device")
	t.refreshDevices()
	mRefresh := t.mDeviceSub.AddSubMenuItem("Refresh Devices", "Re-scan PipeWire")

	systray.AddSeparator()

	mQuit := systray.AddMenuItem("Quit", "Stop Moonshine daemon")

	// Event loop
	go t.watchState()
	go t.menuLoop(mRefresh, mQuit)
}

func (t *Tray) menuLoop(mRefresh, mQuit *systray.MenuItem) {
	for {
		select {
		case <-t.mClipboard.ClickedCh:
			t.d.SetMode(daemon.ModeClipboard)
			t.mClipboard.Check()
			t.mType.Uncheck()

		case <-t.mType.ClickedCh:
			t.d.SetMode(daemon.ModeType)
			t.mType.Check()
			t.mClipboard.Uncheck()

		case <-mRefresh.ClickedCh:
			t.refreshDevices()

		case <-mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func (t *Tray) refreshDevices() {
	// Remove old device menu items
	for _, m := range t.mDevices {
		m.Hide()
	}
	t.mDevices = nil

	devices, err := t.d.Devices()
	if err != nil {
		return
	}

	currentDevice := t.d.GetCurrentDeviceTarget()

	for _, dev := range devices {
		label := dev.Description
		if label == "" {
			label = dev.NodeName
		}
		m := t.mDeviceSub.AddSubMenuItem(label, dev.NodeName)

		if dev.NodeName == currentDevice {
			m.Check()
		}

		// Capture for closure
		nodeName := dev.NodeName
		go func() {
			for range m.ClickedCh {
				t.d.SwitchDevice(nodeName)
				// Update checks
				for _, dm := range t.mDevices {
					dm.Uncheck()
				}
				m.Check()
			}
		}()

		t.mDevices = append(t.mDevices, m)
	}
}

func (t *Tray) watchState() {
	for sc := range t.d.StateCh {
		switch sc.State {
		case daemon.StateIdle:
			systray.SetIcon(IconIdle)
			t.mStatus.SetTitle("Status: Idle")
		case daemon.StateRecording:
			systray.SetIcon(IconRecording)
			t.mStatus.SetTitle("Status: Recording")
		case daemon.StateProcessing:
			systray.SetIcon(IconProcessing)
			t.mStatus.SetTitle("Status: Processing")
		}
	}
}

func (t *Tray) onExit() {
	// Cleanup handled by main
}
