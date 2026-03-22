# Moonshine Voice-to-Text

Local speech-to-text daemon using the Moonshine AI model. Works completely offline with no cloud dependency. Features a system tray icon, real-time streaming transcription, and configurable text processing.

## Features

- **Always Listening Mode** - Continuous speech-to-text without button presses
- **Press to Talk Mode** - Traditional push-to-talk recording
- **System Tray Integration** - Visual status indicator with menu controls
- **Two Output Modes**:
  - **Type** - Types transcribed text into the focused window
  - **Clipboard** - Copies transcribed text to clipboard
- **Smart Text Processing**:
  - Auto-punctuation (adds periods and question marks)
  - Number conversion ("twenty three" → "23")
  - Filler word removal (um, uh, etc.)
  - Voice commands (say "new line", "period", etc.)
  - Auto-capitalization
- **Transcription History** - Access recent transcriptions from the tray menu
- **Device Selection** - Choose audio input device from tray or CLI
- **USB Headset Keep-Alive** - Prevents USB audio devices from sleeping

## Installation

### NixOS / Nix Flakes

Add to your flake inputs:

```nix
{
  inputs.moonshine.url = "github:loljeah/moonshine";
}
```

Then include in your system packages:

```nix
environment.systemPackages = [ inputs.moonshine.packages.${system}.default ];
```

Or run directly:

```bash
nix run github:loljeah/moonshine
```

### Building from Source

```bash
git clone https://github.com/loljeah/moonshine.git
cd moonshine
nix build
./result/bin/moonshine-daemon
```

### Development

```bash
nix develop
go build ./cmd/moonshine-daemon
go build ./cmd/moonshine-ctl
```

## Usage

### Starting the Daemon

```bash
moonshine-daemon          # Normal mode
moonshine-daemon -v       # Verbose logging
```

The daemon starts with:
- **Always Listening** mode enabled
- **Type** output mode (types into focused window)
- System tray icon visible

### System Tray Menu

| Menu Item | Description |
|-----------|-------------|
| Status | Shows current state (Ready, Listening, Recording, Processing) |
| Enabled/Disabled | Master toggle to enable/disable all recognition |
| Always Listening | Continuous speech recognition mode |
| Press to Talk | Manual recording mode |
| Clipboard | Output transcriptions to clipboard |
| Type | Output transcriptions by typing |
| Device | Select audio input device |
| History | View and copy recent transcriptions |
| Quit | Stop the daemon |

### CLI Control (moonshine-ctl)

```bash
# Check status
moonshine-ctl status

# Toggle recording (Press to Talk mode)
moonshine-ctl toggle

# Switch output mode
moonshine-ctl mode clipboard
moonshine-ctl mode type

# Control Always Listening
moonshine-ctl freespeech on
moonshine-ctl freespeech off
moonshine-ctl freespeech toggle

# List audio devices
moonshine-ctl devices

# Switch audio device (substring match)
moonshine-ctl device "USB Headset"

# View/change settings
moonshine-ctl settings                    # List all
moonshine-ctl settings auto_punctuation   # Get one
moonshine-ctl settings auto_punctuation off  # Set

# Stop daemon
moonshine-ctl quit
```

### Socket API

The daemon listens on `/tmp/moonshine/moonshine.sock` for control commands:

```bash
echo "status" | nc -U /tmp/moonshine/moonshine.sock
echo "freespeech toggle" | nc -U /tmp/moonshine/moonshine.sock
echo "settings" | nc -U /tmp/moonshine/moonshine.sock
```

## Configuration

Config file location: `~/.config/moonshine/config`

The config file uses simple `KEY=VALUE` format:

```bash
# Audio device (substring match)
DEVICE=USB Headset

# Language for transcription
LANGUAGE=en

# Text processing options (on/off)
AUTO_PUNCTUATION=on
AUTO_CAPITALIZE=on
FILLER_REMOVAL=on
VOICE_COMMANDS=on

# Number format: "words" or "digits"
# "digits" converts "twenty three" to "23"
NUMBER_FORMAT=words
```

### Configuration Options

| Option | Values | Default | Description |
|--------|--------|---------|-------------|
| `DEVICE` | string | (auto) | Audio input device substring |
| `LANGUAGE` | string | `en` | Transcription language |
| `AUTO_PUNCTUATION` | on/off | `on` | Add periods and question marks |
| `AUTO_CAPITALIZE` | on/off | `on` | Capitalize sentences and "I" |
| `FILLER_REMOVAL` | on/off | `on` | Remove "um", "uh", etc. |
| `VOICE_COMMANDS` | on/off | `on` | Expand voice commands |
| `NUMBER_FORMAT` | words/digits | `words` | Number formatting |

### Voice Commands

When `VOICE_COMMANDS=on`, these phrases are expanded:

| Say | Result |
|-----|--------|
| "new line" / "enter" | Newline |
| "new paragraph" | Double newline |
| "period" / "dot" | `.` |
| "comma" | `,` |
| "question mark" | `?` |
| "exclamation mark" | `!` |
| "colon" | `:` |
| "semicolon" | `;` |
| "quote" / "single quote" | `'` |
| "double quote" | `"` |
| "open paren" | `(` |
| "close paren" | `)` |
| "open bracket" | `[` |
| "close bracket" | `]` |
| "open brace" | `{` |
| "close brace" | `}` |
| "tab" | Tab character |
| "space" | Space |
| "backspace" | Backspace key |
| "dash" / "hyphen" / "minus" | `-` |
| "underscore" | `_` |
| "plus" | `+` |
| "equals" | `=` |
| "slash" | `/` |
| "backslash" | `\` |
| "asterisk" / "star" | `*` |
| "hash" / "pound" | `#` |
| "ampersand" | `&` |
| "pipe" | `\|` |
| "dollar sign" | `$` |
| "double ampersand" | `&&` |
| "double pipe" | `\|\|` |
| "double equals" | `==` |
| "arrow up/down/left/right" | Arrow keys |

## Runtime Settings

Settings can be changed at runtime via the socket API without restarting:

```bash
# Disable auto-punctuation
echo "settings auto_punctuation off" | nc -U /tmp/moonshine/moonshine.sock

# Enable digit conversion
echo "settings number_format digits" | nc -U /tmp/moonshine/moonshine.sock
```

Note: Runtime changes are not persisted to the config file.

## Files and Directories

| Path | Description |
|------|-------------|
| `~/.config/moonshine/config` | Configuration file |
| `~/.local/share/moonshine/history.log` | Transcription history |
| `/tmp/moonshine/moonshine.sock` | Unix socket for control |
| `/tmp/moonshine/status` | Current state file |

## Requirements

- **Linux** with PipeWire audio
- **Wayland** compositor (for wtype keyboard simulation)
- **wl-clipboard** (for clipboard operations)
- **libnotify** (for desktop notifications)

## Architecture

```
moonshine-daemon
├── System Tray (fyne.io/systray)
├── Moonshine Transcriber (CGO → libmoonshine.so)
├── PipeWire Audio (pw-record streaming)
├── Socket Server (Unix socket API)
└── Output (wtype/wl-clipboard)
```

The daemon uses a streaming transcription model that processes audio in real-time chunks, providing continuous transcription in Always Listening mode.

## License

MIT
