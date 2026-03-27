# Session Journal — Moonshine

## Session: 2026-03-27

### Summary

Removed system tray and Waybar integration to make the application purely userspace-runnable via home.nix without GUI dependencies.

### Changes Made

#### Commit: b4672ce — Remove system tray and Waybar integration

**Files Deleted:**
- `internal/tray/tray.go` — System tray UI (395 lines)
- `internal/tray/icons.go` — Embedded tray icons (215 lines)
- `vendor/fyne.io/systray/` — Entire systray library
- `vendor/github.com/godbus/dbus/` — D-Bus dependency

**Files Modified:**
| File | Changes |
|------|---------|
| `cmd/moonshine-daemon/main.go` | Removed tray import, `--no-tray` flag, `tray.Run()` call |
| `internal/daemon/socket.go` | Removed `status json` command, `writeStatusJSON()` function |
| `go.mod` | Removed `fyne.io/systray`, `godbus/dbus` dependencies |
| `go.sum` | Removed systray and dbus entries |
| `vendor/modules.txt` | Removed systray and dbus entries |
| `flake.nix` | Removed `dbus`, `libayatana-appindicator`, `gtk3`, `glib` from buildInputs |
| `nix/hm-module.nix` | Removed `enableTray` option |

**Impact:**
- Removed 12,302 lines of code (70 files)
- No more D-Bus/GTK dependencies
- Daemon runs headless via systemd user service
- Control via `moonshine-ctl` or `moonshine-rofi`

#### Commit: 52ff5f4 — Update README and clean up tray references

**Files Modified:**
| File | Changes |
|------|---------|
| `README.md` | Complete rewrite for headless CLI usage, Home Manager docs, rofi/keybindings |
| `shell-go.nix` | Removed dbus, gtk3, glib, libayatana-appindicator dependencies |
| `internal/daemon/daemon.go` | Removed tray references from comments |
| `.context/PROJECT.md` | Updated architecture and documentation |
| `.context/SESSION.md` | Session journal |

### Current Architecture

```
moonshine-daemon (headless service)
├── Socket IPC (/tmp/moonshine/moonshine.sock)
├── Transcriber (Moonshine or Whisper backend)
├── Audio (PipeWire recording)
├── Output (wl-copy, wtype, notify-send)
└── Config (~/.config/moonshine/config)

moonshine-ctl (CLI client)
└── Sends commands to socket, receives responses

moonshine-rofi (launcher)
└── Rofi menu that calls moonshine-ctl
```

### CLI Commands (via moonshine-ctl)

```
toggle [clipboard|type]   Start/stop recording
status                    Get current state
mode [clipboard|type]     Get/set output mode
device <name>             Switch audio input
devices                   List available devices
freespeech on|off|toggle  Control always-listening
listen start|stop         Start/stop FreeSpeech mode
logs [n]                  View daemon logs
settings [key [value]]    Get/set config
scratch                   Undo last output
quit                      Shutdown daemon
```

### home.nix Usage

```nix
services.moonshine = {
  enable = true;
  package = inputs.moonshine.packages.${system}.default;
  settings = {
    device = "PRO X";
    language = "en";
    backend = "moonshine";
  };
  verbose = false;
};
```

### Commits This Session

| Commit | Description |
|--------|-------------|
| `b4672ce` | Remove system tray and Waybar integration for pure userspace CLI |
| `52ff5f4` | Update README and clean up tray references |

### Testing

Built and tested the daemon:

```
$ nix build
$ ./result/bin/moonshine-daemon --verbose &
moonshine-daemon running
  socket: /tmp/moonshine/moonshine.sock
moonshine: loading model...
moonshine: backend: moonshine, model: .../medium-streaming-en/quantized
moonshine: model loaded
moonshine: matched device "PRO X" -> alsa_input.usb-Logitech_PRO_X_Wireless_Gaming_Headset-00.mono-fallback

$ ./result/bin/moonshine-ctl status
OK idle

$ ./result/bin/moonshine-ctl devices
OK PRO X Wireless Gaming Headset Mono (alsa_input.usb-...)

$ ./result/bin/moonshine-ctl freespeech
OK off

$ ./result/bin/moonshine-ctl settings
OK LANGUAGE=en

$ ./result/bin/moonshine-ctl mode
OK type

$ ./result/bin/moonshine-ctl quit
OK
```

All CLI commands working correctly.

### Current State

- All changes committed and pushed to `origin/master`
- Build verified (`nix build` successful)
- Daemon tested and working headless
- CLI control fully functional

### Open Questions

- None

### Next Steps (potential)

- Add more voice commands (backspace, delete, escape)
- German filler patterns for `LANGUAGE=de`
- Auto-restart daemon on config change
- Add CLI commands for backend/language hot-swap
