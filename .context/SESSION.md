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

### Current State

- All changes committed and pushed to `origin/master`
- Build verified (`go vet`, `go build`, `nix flake check` pass)
- Daemon runs headless, controlled via socket IPC

### Open Questions

- None

### Next Steps (potential)

- Add more voice commands (backspace, delete, escape)
- German filler patterns for `LANGUAGE=de`
- Auto-restart daemon on config change
- Additional language options
