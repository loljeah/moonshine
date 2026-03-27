# Moonshine — Project Context

## Overview

Local offline speech-to-text daemon using the Moonshine AI model. Go-based headless daemon for Wayland/Sway desktops, controlled via CLI or rofi launcher.

## Architecture

```
cmd/
├── moonshine-daemon/main.go    Entry point, loads model, starts daemon
└── moonshine-ctl/main.go       CLI client for socket commands

internal/
├── daemon/
│   ├── daemon.go               Core state machine (idle→recording→processing→idle)
│   ├── socket.go               Unix socket IPC server (/tmp/moonshine/moonshine.sock)
│   └── output.go               Clipboard (wl-copy), typing (wtype), notifications
├── audio/
│   ├── recorder.go             pw-record wrapper for push-to-talk
│   ├── stream_recorder.go      Streaming recorder for Free Speech mode
│   ├── devices.go              PipeWire device enumeration (pw-dump)
│   └── wav.go                  PCM/WAV utilities
├── config/
│   └── config.go               ~/.config/moonshine/config (device, language, backend)
├── transcriber/
│   ├── transcriber.go          Abstract Transcriber interface
│   ├── moonshine_adapter.go    Wraps internal/moonshine for Transcriber interface
│   ├── whisper_adapter.go      Whisper.cpp backend (build tag: whisper)
│   ├── whisper_stub.go         Stub when whisper not compiled
│   └── factory.go              Backend selection factory
└── moonshine/
    └── moonshine.go            cgo bindings to libmoonshine.so (transcription)

nix/
└── hm-module.nix               Home Manager module for home.nix integration
```

## Output Modes

| Mode | Behavior |
|------|----------|
| `clipboard` | Transcribe → copy to clipboard (wl-copy) |
| `type` | Transcribe → type into focused window (wtype) |

## Trigger Modes

| Mode | Behavior |
|------|----------|
| Press-to-Talk | `moonshine-ctl toggle` starts/stops recording |
| Free Speech | Always-on listening, auto-types as you speak |

## State Machine

```
Press-to-Talk:
StateIdle → (toggle) → StateRecording → (toggle) → StateProcessing → StateIdle
                                                       ↓
                                               transcribe + output

Free Speech mode:
StateListening → (voice detected) → StateSpeechDetected → (silence) → StateListening
                                           ↓
                                    transcribe + output
```

## Key Files

| File | Purpose |
|------|---------|
| `internal/daemon/daemon.go` | Toggle(), state machine, voice command expansion, filler removal |
| `internal/daemon/socket.go` | Unix socket server, command parsing |
| `internal/daemon/output.go` | TypeText() with {KEY} placeholder support, CopyToClipboard() |
| `internal/transcriber/transcriber.go` | Transcriber interface (Moonshine/Whisper backends) |
| `internal/audio/stream_recorder.go` | Continuous audio capture for Free Speech |

## Voice Commands (Free Speech mode)

Spoken phrases auto-expand:
- "new paragraph" → `\n\n`
- "new line" / "enter" → `\n`
- "tab" → `\t`
- "space" → ` `
- "arrow up/down/left/right" → `{Up}`, `{Down}`, `{Left}`, `{Right}`
- "scratch that" → undo last output

## Dependencies

- Go 1.23+
- `libmoonshine.so` — Moonshine model runtime (cgo)
- PipeWire (`pw-record`, `pw-play`, `pw-dump`)
- Wayland tools (`wl-copy`, `wtype`, `notify-send`)

## IPC

Unix socket at `/tmp/moonshine/moonshine.sock`. Commands:

```
toggle [clipboard|type]   Start/stop recording
status                    Get current state
mode [clipboard|type]     Get/set output mode
device <name>             Switch audio input
devices                   List available devices
freespeech on|off|toggle  Control always-listening mode
listen start|stop         Start/stop FreeSpeech
logs [n]                  View daemon logs (default 50)
settings [key [value]]    Get/set config parameters
scratch                   Undo last output
quit                      Shutdown daemon
```

## Build

```bash
nix develop
go build ./cmd/moonshine-daemon
go build ./cmd/moonshine-ctl
```

Or via flake:
```bash
nix build
```

## Config

`~/.config/moonshine/config`:
```
DEVICE=PRO X
LANGUAGE=en
BACKEND=moonshine
AUTO_PUNCTUATION=on
AUTO_CAPITALIZE=on
FILLER_REMOVAL=on
VOICE_COMMANDS=on
NUMBER_FORMAT=words
SENTENCE_END=.
SILENCE_TIMEOUT=3
```

## Home Manager Integration

```nix
services.moonshine = {
  enable = true;
  package = inputs.moonshine.packages.${system}.default;
  settings = {
    device = "PRO X";
    language = "en";
    backend = "moonshine";
    autoPunctuation = true;
    fillerRemoval = true;
  };
  verbose = false;
};
```

## Data

- `~/.config/moonshine/config` — main config
- `~/.config/moonshine/macros` — user-defined voice macros
- `~/.local/share/moonshine/sounds/` — notification WAVs
- `~/.local/share/moonshine/history.log` — transcription log
- `~/.local/share/moonshine/daemon.log` — daemon log
- `~/.cache/moonshine_voice/` — model files

## Transcription Backends

| Backend | Languages | Notes |
|---------|-----------|-------|
| Moonshine | en, es, ar, ja | Fast, streaming, default |
| Whisper | 40+ (de, fr, etc.) | Slower, more languages |

## Commit History (recent)

| Hash | Description |
|------|-------------|
| b4672ce | Remove system tray and Waybar integration for pure userspace CLI |
| bf2b1f0 | Add runtime language hot-swap and expand tray language menu |
| 4ae6683 | Add Home Manager module, rofi launcher, currency symbols |
| 07a5366 | Add word boundary fix, scratch-that undo, macros, Waybar JSON |
| 998f4c4 | Expand filler word removal with categorized patterns |
| 4fe18a4 | Add dual-backend transcriber architecture (Moonshine + Whisper) |
