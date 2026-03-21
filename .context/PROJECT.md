# Moonshine — Project Context

## Overview

Local offline speech-to-text daemon using the Moonshine AI model. Go-based with system tray integration for Wayland/Sway desktops.

## Architecture

```
cmd/moonshine-daemon/main.go    Entry point, loads model, starts daemon + tray
internal/
├── daemon/
│   ├── daemon.go               Core state machine (idle→recording→processing→idle)
│   ├── socket.go               Unix socket IPC server (/tmp/moonshine/sock)
│   └── output.go               Clipboard (wl-copy), typing (wtype), notifications
├── tray/
│   ├── tray.go                 System tray (fyne.io/systray) with mode/device menus
│   └── icons.go                Embedded tray icons
├── audio/
│   ├── recorder.go             pw-record wrapper for push-to-talk
│   ├── stream_recorder.go      Streaming recorder for Free Speech mode
│   ├── devices.go              PipeWire device enumeration (pw-dump)
│   └── wav.go                  PCM/WAV utilities
├── config/
│   └── config.go               ~/.config/moonshine/config (device, language)
└── moonshine/
    └── moonshine.go            cgo bindings to libmoonshine.so (transcription)
```

## Output Modes

| Mode | Behavior |
|------|----------|
| `clipboard` | Transcribe → copy to clipboard (wl-copy) |
| `type` | Transcribe → type into focused window (wtype) |
| `free-speech` | Always-on streaming, auto-types as you speak |

## State Machine

```
StateIdle → (toggle) → StateRecording → (toggle) → StateProcessing → StateIdle
                                                       ↓
                                               transcribe + output

Free Speech mode:
StateListening → (voice detected) → StateSpeechDetected → (complete) → StateListening
```

## Key Files

| File | Purpose |
|------|---------|
| `internal/daemon/daemon.go` | Toggle(), state machine, voice command expansion, history |
| `internal/daemon/output.go` | TypeText() with {KEY} placeholder support, CopyToClipboard() |
| `internal/tray/tray.go` | System tray menu, mode selection, device picker, history submenu |
| `internal/moonshine/moonshine.go` | cgo bindings to libmoonshine.so |
| `internal/audio/stream_recorder.go` | Continuous audio capture for Free Speech |

## Voice Commands (Free Speech mode)

Spoken phrases auto-expand:
- "new paragraph" → `\n\n`
- "new line" / "enter" → `\n`
- "tab" → `\t`
- "space" → ` `
- "arrow up/down/left/right" → `{Up}`, `{Down}`, `{Left}`, `{Right}`

## Dependencies

- Go 1.22+
- `fyne.io/systray` — system tray
- `libmoonshine.so` — Moonshine model runtime (cgo)
- PipeWire (`pw-record`, `pw-play`, `pw-dump`)
- Wayland tools (`wl-copy`, `wtype`, `notify-send`)

## IPC

Unix socket at `/tmp/moonshine/sock`. Commands:
- `toggle` — start/stop recording
- `mode clipboard|type|free-speech` — switch mode
- `device <search>` — switch audio input
- `status` — get current state
- `quit` — shutdown daemon

## Build

```bash
nix-shell shell-go.nix
go build -o moonshine-daemon ./cmd/moonshine-daemon
```

## Config

`~/.config/moonshine/config`:
```
DEVICE=Blue
LANGUAGE=en
```

## Data

- `~/.local/share/moonshine/sounds/` — notification WAVs
- `~/.local/share/moonshine/history.log` — transcription log
- `~/.cache/moonshine_voice/` — model files

## Commit History (recent)

| Hash | Description |
|------|-------------|
| 3e45b3a | feat: add space voice command |
| 64564a8 | feat: add voice command expansion in Free Speech mode |
| 855a8ff | feat: add transcription history log, fix Free Speech loop, flat mode menu |
| 76e58b2 | refactor: move output modes into Mode submenu in tray |
| 092df67 | feat: add Free Speech mode — always-on listening with auto-type |
| b4b3bc3 | feat: replace bash/python with Go daemon using cgo libmoonshine.so |
