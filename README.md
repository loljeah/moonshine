# Moonshine Voice-to-Text

Local speech-to-text using Moonshine AI model. Works offline, no cloud.

## Features

- **Toggle recording** with mouse thumb button (button8)
- **Waybar indicator** with pulsing animation while recording
- **Sound notifications** for start/stop/success/error
- **Two output modes**: clipboard or type into focused window

## Installation

Run the deploy script:

```bash
./deploy.sh
```

## Usage

- **Mouse thumb button** - Record and copy to clipboard
- **Super + thumb button** - Record and type into focused window
- **Waybar click** - Same as thumb button
- **Waybar right-click** - Type mode

## Files

- `voice-toggle` - Main control script
- `record-simple.py` - Audio recording (callback-based)
- `transcribe.py` - Moonshine transcription
- `waybar-moonshine` - Waybar module
- `shell.nix` - Nix dependencies

## Requirements

- PipeWire/ALSA for audio
- Waybar for status indicator
- wl-copy/wtype for output
- nix-shell for Python environment
