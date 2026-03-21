package tray

import (
	"fmt"

	"moonshine-daemon/internal/daemon"

	"fyne.io/systray"
)

const historySlots = 20

// Tray manages the system tray icon and menu.
type Tray struct {
	d       *daemon.Daemon
	verbose bool

	// Menu items we need to update
	mStatus      *systray.MenuItem
	mClipboard   *systray.MenuItem
	mType        *systray.MenuItem
	mFreeSpeech  *systray.MenuItem
	mDevices     []*systray.MenuItem
	mDeviceSub   *systray.MenuItem

	// History submenu
	mHistorySub   *systray.MenuItem
	mHistoryItems []*systray.MenuItem
	historyTexts  []string // full text for each slot (for clipboard copy)
}

// Run starts the system tray. Blocks until quit is selected.
// Call from the main goroutine.
func Run(d *daemon.Daemon, verbose bool) {
	t := &Tray{d: d, verbose: verbose}
	systray.Run(t.onReady, t.onExit)
}

func (t *Tray) onReady() {
	systray.SetIcon(IconIdle)
	systray.SetTitle("")
	systray.SetTooltip("Moonshine — Clipboard")

	// Status (disabled, display-only)
	t.mStatus = systray.AddMenuItem("Idle — Clipboard", "Current state and mode")
	t.mStatus.Disable()

	systray.AddSeparator()

	// Output mode — radio items
	t.mClipboard = systray.AddMenuItem("Clipboard", "Copy transcription to clipboard")
	t.mType = systray.AddMenuItem("Type", "Type transcription into focused window")
	t.mFreeSpeech = systray.AddMenuItem("Free Speech", "Always-on listening, auto-type speech")
	t.mClipboard.Check()

	systray.AddSeparator()

	// Device submenu
	t.mDeviceSub = systray.AddMenuItem("Device", "Audio input device")
	t.refreshDevices()
	mRefresh := t.mDeviceSub.AddSubMenuItem("Refresh Devices", "Re-scan PipeWire")

	// History submenu
	t.mHistorySub = systray.AddMenuItem("History", "Transcription history")
	t.mHistoryItems = make([]*systray.MenuItem, historySlots)
	t.historyTexts = make([]string, historySlots)
	for i := 0; i < historySlots; i++ {
		m := t.mHistorySub.AddSubMenuItem("", "")
		m.Hide()
		t.mHistoryItems[i] = m

		idx := i
		go func() {
			for range m.ClickedCh {
				text := t.historyTexts[idx]
				if text != "" {
					daemon.CopyToClipboard(text)
				}
			}
		}()
	}
	t.refreshHistory()

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
			t.syncModeChecks(daemon.ModeClipboard)

		case <-t.mType.ClickedCh:
			t.d.SetMode(daemon.ModeType)
			t.syncModeChecks(daemon.ModeType)

		case <-t.mFreeSpeech.ClickedCh:
			t.d.SetMode(daemon.ModeFreeSpeech)
			t.syncModeChecks(daemon.ModeFreeSpeech)
			t.d.StartListening()

		case <-mRefresh.ClickedCh:
			t.refreshDevices()

		case <-mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func (t *Tray) syncModeChecks(m daemon.OutputMode) {
	t.mClipboard.Uncheck()
	t.mType.Uncheck()
	t.mFreeSpeech.Uncheck()
	switch m {
	case daemon.ModeClipboard:
		t.mClipboard.Check()
	case daemon.ModeType:
		t.mType.Check()
	case daemon.ModeFreeSpeech:
		t.mFreeSpeech.Check()
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

func (t *Tray) refreshHistory() {
	entries := t.d.History()

	for i := 0; i < historySlots; i++ {
		if i < len(entries) {
			e := entries[i]
			ts := e.Time.Format("15:04")
			text := e.Text
			if len(text) > 60 {
				text = text[:57] + "..."
			}
			label := fmt.Sprintf("%s — %s", ts, text)
			t.mHistoryItems[i].SetTitle(label)
			t.mHistoryItems[i].SetTooltip(e.Text)
			t.mHistoryItems[i].Show()
			t.historyTexts[i] = e.Text
		} else {
			t.mHistoryItems[i].Hide()
			t.historyTexts[i] = ""
		}
	}

	if len(entries) == 0 {
		t.mHistorySub.SetTitle("History (empty)")
	} else {
		t.mHistorySub.SetTitle(fmt.Sprintf("History (%d)", len(entries)))
	}
}

func (t *Tray) watchState() {
	for sc := range t.d.StateCh {
		// Update icon
		switch sc.State {
		case daemon.StateIdle:
			systray.SetIcon(IconIdle)
		case daemon.StateRecording, daemon.StateSpeechDetected:
			systray.SetIcon(IconRecording)
		case daemon.StateProcessing:
			systray.SetIcon(IconProcessing)
		case daemon.StateListening:
			systray.SetIcon(IconListening)
		}

		// Update status line and tooltip with state + mode
		var modeLabel string
		switch sc.Mode {
		case daemon.ModeType:
			modeLabel = "Type"
		case daemon.ModeFreeSpeech:
			modeLabel = "Free Speech"
		default:
			modeLabel = "Clipboard"
		}

		var stateLabel string
		switch sc.State {
		case daemon.StateIdle:
			stateLabel = "Idle"
		case daemon.StateRecording:
			stateLabel = "Recording"
		case daemon.StateProcessing:
			stateLabel = "Processing"
		case daemon.StateListening:
			stateLabel = "Listening"
		case daemon.StateSpeechDetected:
			stateLabel = "Speech"
		}

		t.mStatus.SetTitle(stateLabel + " — " + modeLabel)
		systray.SetTooltip("Moonshine — " + modeLabel)

		// Sync radio checks with current mode
		t.syncModeChecks(sc.Mode)

		// Refresh history when returning to idle (transcription completed)
		if sc.State == daemon.StateIdle || sc.State == daemon.StateListening {
			t.refreshHistory()
		}
	}
}

func (t *Tray) onExit() {
	// Cleanup handled by main
}
