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
	mEnabled     *systray.MenuItem // Master enable/disable toggle
	mFreeSpeech  *systray.MenuItem // Trigger: always listening
	mPushToTalk  *systray.MenuItem // Trigger: press to talk
	mClipboard   *systray.MenuItem // Output: clipboard
	mType        *systray.MenuItem // Output: type
	mDevices     []*systray.MenuItem
	mDeviceSub   *systray.MenuItem

	// History submenu
	mHistorySub   *systray.MenuItem
	mHistoryItems []*systray.MenuItem
	historyTexts  []string // full text for each slot (for clipboard copy)

	// Language/Backend submenu
	mLangSub   *systray.MenuItem
	mLangItems []*systray.MenuItem
	langOpts   []langOption
}

// langOption describes a selectable language entry.
type langOption struct {
	label   string // display name, e.g. "English (Moonshine)"
	lang    string // config value, e.g. "en"
	backend string // "moonshine" or "whisper"
}

// supportedLanguages defines all selectable language/backend combinations.
// Moonshine only supports English; all other languages use Whisper.
var supportedLanguages = []langOption{
	{"English (Moonshine)", "en", "moonshine"},
	{"German (Whisper)", "de", "whisper"},
	{"Spanish (Whisper)", "es", "whisper"},
	{"Japanese (Whisper)", "ja", "whisper"},
	{"Arabic (Whisper)", "ar", "whisper"},
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
	systray.SetTooltip("Moonshine")

	// Status (disabled, display-only)
	t.mStatus = systray.AddMenuItem("⏸ Ready", "Current state")
	t.mStatus.Disable()

	systray.AddSeparator()

	// Master enable/disable toggle
	t.mEnabled = systray.AddMenuItem("🟢 Enabled", "Click to disable speech recognition")

	systray.AddSeparator()

	// Trigger mode — how speech is captured
	t.mFreeSpeech = systray.AddMenuItem("● Always Listening", "Continuous speech-to-text")
	t.mPushToTalk = systray.AddMenuItem("○ Press to Talk", "Button-triggered recording")

	systray.AddSeparator()

	// Output destination — where transcribed text goes
	t.mClipboard = systray.AddMenuItem("○ Clipboard", "Copy transcriptions to clipboard")
	t.mType = systray.AddMenuItem("● Type", "Type into focused window")

	// Set defaults: Always Listening + Type
	t.d.SetMode(daemon.ModeType)
	t.syncModeChecks(daemon.ModeType)
	t.syncTriggerChecks(true)
	// Start FreeSpeech (no goroutine - avoid race condition)
	t.d.SetFreeSpeech(true)

	systray.AddSeparator()

	// Device submenu
	t.mDeviceSub = systray.AddMenuItem("Device", "Audio input device")
	t.refreshDevices()
	mRefresh := t.mDeviceSub.AddSubMenuItem("Refresh Devices", "Re-scan PipeWire")

	// Language/Backend submenu
	t.mLangSub = systray.AddMenuItem("Language", "Transcription language and backend")
	t.langOpts = supportedLanguages
	t.mLangItems = make([]*systray.MenuItem, len(t.langOpts))
	for i, opt := range t.langOpts {
		t.mLangItems[i] = t.mLangSub.AddSubMenuItem(opt.label, opt.lang+" / "+opt.backend)
	}
	t.syncLanguageChecks()

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

	// Start language click handlers (one goroutine per language)
	for i, opt := range t.langOpts {
		idx := i
		o := opt
		go func() {
			for range t.mLangItems[idx].ClickedCh {
				daemon.Notify("Moonshine", "Switching to "+o.label+"...")
				if err := t.d.SwitchBackend(o.backend, o.lang); err != nil {
					daemon.Notify("Moonshine", "Switch failed: "+err.Error())
				} else {
					t.syncLanguageChecks()
					daemon.Notify("Moonshine", "Switched to "+o.label)
				}
			}
		}()
	}

	// Event loop
	go t.watchState()
	go t.menuLoop(mRefresh, mQuit)
}

func (t *Tray) menuLoop(mRefresh, mQuit *systray.MenuItem) {
	for {
		select {
		case <-t.mEnabled.ClickedCh:
			// Toggle master enable/disable
			enabled := t.d.GetEnabled()
			t.d.SetEnabled(!enabled)
			t.syncEnabledCheck(!enabled)

		case <-t.mFreeSpeech.ClickedCh:
			// Select Always Listening trigger
			t.d.SetFreeSpeech(true)
			t.syncTriggerChecks(true)

		case <-t.mPushToTalk.ClickedCh:
			// Select Press to Talk trigger
			t.d.SetFreeSpeech(false)
			t.syncTriggerChecks(false)

		case <-t.mClipboard.ClickedCh:
			t.d.SetMode(daemon.ModeClipboard)
			t.syncModeChecks(daemon.ModeClipboard)

		case <-t.mType.ClickedCh:
			t.d.SetMode(daemon.ModeType)
			t.syncModeChecks(daemon.ModeType)

		case <-mRefresh.ClickedCh:
			t.refreshDevices()

		case <-mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func (t *Tray) syncModeChecks(m daemon.OutputMode) {
	switch m {
	case daemon.ModeClipboard:
		t.mClipboard.SetTitle("● Clipboard")
		t.mType.SetTitle("○ Type")
	case daemon.ModeType:
		t.mClipboard.SetTitle("○ Clipboard")
		t.mType.SetTitle("● Type")
	}
}

func (t *Tray) syncTriggerChecks(freeSpeech bool) {
	if freeSpeech {
		t.mFreeSpeech.SetTitle("● Always Listening")
		t.mPushToTalk.SetTitle("○ Press to Talk")
	} else {
		t.mFreeSpeech.SetTitle("○ Always Listening")
		t.mPushToTalk.SetTitle("● Press to Talk")
	}
}

func (t *Tray) syncLanguageChecks() {
	lang := t.d.GetLanguage()
	for i, opt := range t.langOpts {
		if opt.lang == lang {
			t.mLangItems[i].SetTitle("● " + opt.label)
		} else {
			t.mLangItems[i].SetTitle("○ " + opt.label)
		}
	}
}

func (t *Tray) syncEnabledCheck(enabled bool) {
	if enabled {
		t.mEnabled.SetTitle("🟢 Enabled")
		// Re-enable other menu items
		t.mFreeSpeech.Enable()
		t.mPushToTalk.Enable()
		t.mClipboard.Enable()
		t.mType.Enable()
	} else {
		t.mEnabled.SetTitle("🔴 Disabled")
		// Disable other menu items (greyed out)
		t.mFreeSpeech.Disable()
		t.mPushToTalk.Disable()
		t.mClipboard.Disable()
		t.mType.Disable()
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
		// Handle disabled state
		if !sc.Enabled {
			systray.SetIcon(IconDisabled)
			t.mStatus.SetTitle("⏹ Disabled")
			systray.SetTooltip("Moonshine (disabled)")
			t.syncEnabledCheck(false)
			continue
		}

		// Sync enabled state
		t.syncEnabledCheck(true)

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

		// Build status line with icons
		var statusIcon, statusText string
		switch sc.State {
		case daemon.StateIdle:
			statusIcon = "⏸"
			statusText = "Ready"
		case daemon.StateRecording:
			statusIcon = "🔴"
			statusText = "Recording..."
		case daemon.StateProcessing:
			statusIcon = "⏳"
			statusText = "Processing..."
		case daemon.StateListening:
			statusIcon = "👂"
			statusText = "Listening..."
		case daemon.StateSpeechDetected:
			statusIcon = "🗣"
			statusText = "Hearing speech..."
		}

		// Build tooltip with mode info
		var destLabel string
		switch sc.Mode {
		case daemon.ModeType:
			destLabel = "typing"
		default:
			destLabel = "clipboard"
		}

		tooltip := "Moonshine → " + destLabel
		if sc.FreeSpeech {
			tooltip = "🎤 " + tooltip
		}

		t.mStatus.SetTitle(statusIcon + " " + statusText)
		systray.SetTooltip(tooltip)

		// Sync output mode selection
		t.syncModeChecks(sc.Mode)

		// Sync trigger mode selection
		t.syncTriggerChecks(sc.FreeSpeech)

		// Refresh history when returning to idle (transcription completed)
		if sc.State == daemon.StateIdle || sc.State == daemon.StateListening {
			t.refreshHistory()
		}
	}
}

func (t *Tray) onExit() {
	// Cleanup handled by main
}
