# Possible Features

Ideas for future moonshine development, organized by category.

---

## Audio & Input

### Wake Word Detection
- Listen for "Hey Moonshine" or custom wake word to activate
- Would allow hands-free activation without always transcribing
- Could use a lightweight keyword spotting model

### Multi-Language Support
- Hot-switch between languages mid-session
- Auto-detect language from speech
- Support more languages beyond en/es/ar/ja

### Noise Gate / VAD Improvements
- Configurable silence threshold
- Better voice activity detection to reduce false triggers
- Environment noise calibration on startup

### Audio Source Routing
- Capture from specific applications (PipeWire stream routing)
- Record system audio for transcribing videos/calls
- Virtual microphone output for feeding into other apps

### Bluetooth Headset Button Support
- Map headset media buttons to toggle recording
- Support for various BT profiles (HFP, A2DP)

---

## Output & Integration

### Application-Specific Profiles
- Different output modes per application
- Auto-detect focused app and apply rules
- Example: clipboard mode in terminal, type mode in browser

### Formatted Output
- Markdown formatting via voice ("bold this", "make heading")
- Code block mode for programming
- List mode (auto-bullet points)

### Multi-Clipboard Support
- Append mode (add to existing clipboard)
- Clipboard history ring
- Primary selection support (middle-click paste)

### Direct Application Integration
- DBus integration for specific apps
- Obsidian/Logseq plugin for note-taking
- IDE integration (VS Code, Neovim)

### Text-to-Speech Response
- Read back transcription for confirmation
- Speak error messages
- Accessibility mode

---

## Commands & Control

### Custom Voice Commands
- User-definable command → action mappings
- Execute shell commands via voice
- Snippets/templates ("insert email signature")

### Dictation Modes
- Email mode (Dear, Sincerely, auto-formatting)
- Code mode (camelCase, snake_case conversion)
- Prose mode (enhanced punctuation)

### Correction Commands
- "Scratch that" - delete last utterance
- "Replace X with Y" - find/replace
- "Undo" - revert last action
- "Spell that" - letter-by-letter input

### Navigation Commands
- "Go to end" - Ctrl+End
- "Select all" - Ctrl+A
- "Find X" - Ctrl+F + type X

---

## UI & Visualization

### Floating Transcription Window
- Real-time preview of what's being transcribed
- Click to edit before sending
- Configurable position/transparency

### Waveform Visualization
- Show audio levels in tray or popup
- Visual feedback for speech detection
- Debug view for troubleshooting

### Keyboard Shortcut Configuration
- Global hotkeys for toggle/mode switch
- Configurable via config file or GUI
- Hyprland/Sway keybind integration examples

### Status Bar Integration
- Waybar module with status/mode display
- i3status-rust integration
- Polybar support

---

## Privacy & Security

### Local Model Updates
- Check for model updates without cloud
- Download from trusted mirrors
- Verify model checksums

### Audio Privacy Mode
- Never write audio to disk
- Secure memory handling
- Auto-clear transcription history

### Encrypted History
- Encrypt history.log at rest
- Require unlock to view history
- Auto-purge after N days

---

## Performance & Reliability

### Model Warm-up
- Pre-load model on system startup
- Keep model hot in memory
- Faster first transcription

### Streaming Improvements
- Lower latency streaming mode
- Adaptive chunk sizing based on speech patterns
- Better handling of long pauses

### Auto-Recovery
- Detect and recover from PipeWire crashes
- Auto-restart on model errors
- Watchdog for daemon health

### Resource Management
- CPU/memory usage limits
- Pause on battery/power saver mode
- Reduce polling when idle

---

## Developer & Power User

### Plugin System
- Lua/Python hooks for custom processing
- Pre/post transcription filters
- Custom voice command handlers

### API Expansion
- HTTP API option (localhost only)
- WebSocket for real-time updates
- JSON output format option

### Logging & Debug
- Configurable log levels
- Export debug bundle for issues
- Performance profiling mode

### Testing Tools
- Audio file input mode (transcribe WAV/MP3)
- Benchmark mode for model comparison
- Accuracy testing against reference transcripts

---

## Platform & Packaging

### X11 Support
- xdotool backend for typing
- xclip backend for clipboard
- Full X11 compatibility mode

### Systemd Integration
- User service file
- Socket activation
- Proper lifecycle management

### Container Support
- Flatpak packaging
- AppImage build
- Docker for server use cases

### macOS Port
- CoreAudio backend
- macOS system tray
- Accessibility API for typing

---

## Priority Suggestions

### High Value / Lower Effort
1. Custom voice commands (user-defined mappings)
2. Keyboard shortcut configuration
3. Waybar/status bar integration
4. "Scratch that" correction command
5. Application-specific profiles

### High Value / Higher Effort
1. Wake word detection
2. Floating transcription preview window
3. Plugin system
4. Multi-language hot-switching
5. Code dictation mode

### Nice to Have
1. Audio source routing
2. Text-to-speech feedback
3. Encrypted history
4. macOS port
5. HTTP API
