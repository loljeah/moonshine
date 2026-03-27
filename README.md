# Moonshine Voice-to-Text

Local speech-to-text daemon using the Moonshine AI model. Works completely offline with no cloud dependency. Runs as a headless systemd user service, controlled via CLI.

## Features

- **Always Listening Mode** - Continuous speech-to-text without button presses
- **Press to Talk Mode** - Traditional push-to-talk recording
- **Two Output Modes**:
  - **Type** - Types transcribed text into the focused window
  - **Clipboard** - Copies transcribed text to clipboard
- **Smart Text Processing**:
  - Auto-punctuation (adds periods and question marks)
  - Number conversion ("twenty three" → "23")
  - Filler word removal (um, uh, etc.)
  - Voice commands (say "new line", "period", etc.)
  - Auto-capitalization
- **Dual Transcription Backends**:
  - **Moonshine** - Fast, streaming, supports en/es/ar/ja
  - **Whisper** - Slower, supports 40+ languages including German
- **Rofi Launcher** - Quick access menu via rofi
- **Home Manager Integration** - Declarative NixOS configuration

## Installation

### Home Manager (Recommended)

Add to your flake inputs:

```nix
{
  inputs.moonshine.url = "github:loljeah/moonshine";
}
```

Then in your home.nix:

```nix
{ inputs, pkgs, ... }:
{
  imports = [ inputs.moonshine.homeManagerModules.default ];

  services.moonshine = {
    enable = true;
    package = inputs.moonshine.packages.${pkgs.system}.default;
    settings = {
      device = "PRO X";           # Audio device substring
      language = "en";
      backend = "moonshine";
      autoPunctuation = true;
      autoCapitalize = true;
      fillerRemoval = true;
      voiceCommands = true;
      numberFormat = "words";     # or "digits"
      silenceTimeout = 3;         # seconds before auto-stop in PTT mode
    };
    verbose = false;
  };
}
```

This creates a systemd user service that starts automatically on login.

### Manual Installation

```bash
git clone https://github.com/loljeah/moonshine.git
cd moonshine
nix build
./result/bin/moonshine-daemon
```

Or run directly:

```bash
nix run github:loljeah/moonshine
```

## Usage

### Starting the Daemon

```bash
moonshine-daemon              # Normal mode
moonshine-daemon --verbose    # Verbose logging
```

Or via systemd (if using Home Manager):

```bash
systemctl --user start moonshine-daemon
systemctl --user status moonshine-daemon
journalctl --user -u moonshine-daemon -f
```

### CLI Control (moonshine-ctl)

```bash
# Check status
moonshine-ctl status

# Toggle recording (Press to Talk mode)
moonshine-ctl toggle
moonshine-ctl toggle clipboard   # Toggle and output to clipboard
moonshine-ctl toggle type        # Toggle and output by typing

# Switch output mode
moonshine-ctl mode clipboard
moonshine-ctl mode type

# Control Always Listening (Free Speech)
moonshine-ctl freespeech on
moonshine-ctl freespeech off
moonshine-ctl freespeech toggle

# List audio devices
moonshine-ctl devices

# Switch audio device (substring match)
moonshine-ctl device "USB Headset"

# View/change settings
moonshine-ctl settings                       # List all
moonshine-ctl settings AUTO_PUNCTUATION      # Get one
moonshine-ctl settings AUTO_PUNCTUATION off  # Set

# Undo last transcription
moonshine-ctl scratch

# View daemon logs
moonshine-ctl logs
moonshine-ctl logs 100   # Last 100 lines

# Stop daemon
moonshine-ctl quit
```

### Rofi Launcher

```bash
moonshine-rofi
```

Or bind to a key in your window manager:

```bash
# Sway example
bindsym $mod+Shift+v exec moonshine-rofi
```

### Socket API

The daemon listens on `/tmp/moonshine/moonshine.sock`:

```bash
echo "status" | nc -U /tmp/moonshine/moonshine.sock
echo "freespeech toggle" | nc -U /tmp/moonshine/moonshine.sock
echo "toggle clipboard" | nc -U /tmp/moonshine/moonshine.sock
```

### Keybindings Example (Sway)

```bash
# Toggle recording
bindsym $mod+v exec moonshine-ctl toggle

# Toggle Free Speech mode
bindsym $mod+Shift+v exec moonshine-ctl freespeech toggle

# Open rofi menu
bindsym $mod+Ctrl+v exec moonshine-rofi
```

## Configuration

Config file: `~/.config/moonshine/config`

```bash
# Audio device (substring match)
DEVICE=PRO X

# Transcription backend: moonshine or whisper
BACKEND=moonshine

# Language (moonshine: en/es/ar/ja, whisper: 40+ languages)
LANGUAGE=en

# Text processing (on/off)
AUTO_PUNCTUATION=on
AUTO_CAPITALIZE=on
FILLER_REMOVAL=on
VOICE_COMMANDS=on

# Number format: words or digits
# digits converts "twenty three" to "23"
NUMBER_FORMAT=words

# Punctuation added at sentence end (. or empty for none)
SENTENCE_END=.

# Seconds of silence before auto-stop in PTT mode (0 = manual only)
SILENCE_TIMEOUT=3

# Whisper-specific settings
WHISPER_MODEL=/path/to/ggml-base.bin
THREADS=4
```

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
| "tab" | Tab character |
| "space" | Space |
| "dash" / "hyphen" | `-` |
| "underscore" | `_` |
| "arrow up/down/left/right" | Arrow keys |
| "scratch that" | Undo last output |

### User Macros

Create `~/.config/moonshine/macros` for custom expansions:

```
my email = user@example.com
shebang = #!/usr/bin/env bash
home address = 123 Main St, City
```

## Files and Directories

| Path | Description |
|------|-------------|
| `~/.config/moonshine/config` | Configuration file |
| `~/.config/moonshine/macros` | User-defined voice macros |
| `~/.local/share/moonshine/history.log` | Transcription history |
| `~/.local/share/moonshine/daemon.log` | Daemon log file |
| `/tmp/moonshine/moonshine.sock` | Unix socket for control |

## Requirements

- **Linux** with PipeWire audio
- **Wayland** compositor (for wtype keyboard simulation)
- **wl-clipboard** (for clipboard operations)
- **libnotify** (for desktop notifications)

## Architecture

```
moonshine-daemon (headless service)
├── Socket Server (/tmp/moonshine/moonshine.sock)
├── Transcriber (Moonshine or Whisper backend)
│   ├── Moonshine (CGO → libmoonshine.so)
│   └── Whisper (whisper.cpp bindings)
├── Audio (PipeWire pw-record streaming)
└── Output (wtype/wl-clipboard/notify-send)

moonshine-ctl (CLI client)
└── Sends commands to socket

moonshine-rofi (launcher)
└── Rofi menu → moonshine-ctl
```

## Development

```bash
nix develop
go build ./cmd/moonshine-daemon
go build ./cmd/moonshine-ctl
./moonshine-daemon --verbose
```

## License

MIT
