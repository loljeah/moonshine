# Moonshine — Future Features

## Output Modes

### Append Mode
Doesn't replace clipboard — appends to a running buffer. Good for dictating long text across multiple recordings. "Flush" sends the whole thing at once.

### Command Mode
Speak shell commands, moonshine executes them. "Open Firefox", "kill last process", "git status". Voice-driven terminal.

### Translate Mode
Speak in one language, output in another. Moonshine transcribes, then pipe through a local translation model.

### Whisper Mode
Low-confidence filter. If the model isn't confident, it drops the text instead of outputting garbage. Shows a "?" icon so you know it heard but didn't trust itself.

### Pipe Mode
Output goes to stdout of a named pipe (`/tmp/moonshine/pipe`). Any script can consume it. Chain moonshine into other tools.

---

## Context-Aware Features

### Window-Aware Formatting
Detects the focused app and adjusts output. Terminal gets raw text, LibreOffice gets punctuated/capitalized text, code editor gets snake_case or camelCase conversion.

### Smart Punctuation
"period", "comma", "new line", "question mark" spoken as words get converted to actual punctuation. Toggle-able.

### Scratchpad
A tiny floating window that shows transcription history. Click any entry to re-copy it. Persistent across reboots.

---

## Power User

### Hotword Trigger
Always-listening mode for a wake word (like "hey moonshine"), then records until silence. No button needed.

### Macro Recorder
Define voice shortcuts. Say "email signature" and it pastes a predefined block. Voice-triggered snippets.

### Correction Loop
After transcription, briefly show the result in a popup. Say "fix that" to re-record, or "ok" to confirm. Reduces garbage output.

### Silence Timeout
Auto-stop recording after N seconds of silence instead of requiring a button press. Configurable threshold.

---

## Systray

### Audio Level Meter
Live mic level in the tray icon itself (like a tiny VU meter). Instantly see if your mic is picking up.

### Quick Device Switcher
Right-click menu shows all PipeWire sources, click to switch. No config file editing.

### Session Stats
"You've dictated 847 words today" in the tooltip. Gamification for people who want to use voice more.

---

## Accessibility — Coconut Integration (Blind Users)

### Audio Feedback Loop
Every state change spoken aloud, not just notification sounds. "Recording started", "Processing", "Copied: hello world" (TTS reads back the transcription). Confirm what was captured without needing to see anything.

### Correction by Voice
"Wrong. Try again" re-records. "Read that back" replays TTS of last transcription. "Spell that" reads letter by letter. No screen needed to verify.

### Screen Reader Integration
Output transcription as AT-SPI events so Orca/BRLTTY pick it up natively. Not just clipboard paste — announce through the accessibility stack.

### Navigation Mode
"Where am I?" speaks the focused window title and app. "Read clipboard" speaks current clipboard. Moonshine becomes a general voice query layer for the desktop.

### Silence-Based Stop
Critical for blind users who can't see a recording indicator. Record until 2-3 seconds of silence, then auto-transcribe. No second button press needed.

### Audio Confirmation Before Output
Speak the transcription back via TTS, then "say confirm to paste" or "say again to re-record". Prevents wrong text from being typed into the wrong place.

### Coconut Contextual Awareness
Coconut knows what app/window is focused. Moonshine can adapt: in a terminal, offer to execute; in a text field, paste; in a file manager, use voice to navigate.

### Voice Menu System
Instead of a visual tray menu, a spoken menu. "Settings" -> "Device: PRO X headset. Say switch to change. Language: English. Say change to switch." Fully navigable by voice.

### IPC Protocol
Moonshine exposes a socket or named pipe so Coconut can request transcription programmatically. Not just hotkey-triggered — any Coconut module can say "get voice input from user".

### Multi-Modal Output
Text to clipboard, but also to a BRLTTY pipe for braille display, and TTS for audio confirmation. Three outputs simultaneously.
