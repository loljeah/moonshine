# Session Journal — Moonshine

## Session: 2026-03-21

### Status

**Uncommitted changes** in 3 files (+193 lines):

1. `internal/daemon/daemon.go` — In-memory history buffer + arrow key voice commands
2. `internal/daemon/output.go` — TypeText() now parses `{KEY}` placeholders for wtype `-k`
3. `internal/tray/tray.go` — History submenu (20 slots), click-to-copy, auto-refresh

### Uncommitted Features

#### 1. In-Memory History Buffer
- `HistoryEntry` struct with `Time`, `Mode`, `Text`
- `loadHistory()` parses existing log file into memory on startup
- `History()` returns entries most-recent-first
- Capped at 50 entries (`maxHistoryEntries`)

#### 2. Arrow Key Voice Commands
New expansions in `expandVoiceCommands()`:
- "arrow down" → `{Down}`
- "arrow up" → `{Up}`
- "arrow left" → `{Left}`
- "arrow right" → `{Right}`

#### 3. TypeText {KEY} Parsing
`output.go` now parses `{KEY}` placeholders and builds wtype args:
- Text segments → passed as positional args
- `{KEY}` → `-k KEY`
- Example: `hello{Down}world` → `wtype -d 12 hello -k Down world`

#### 4. Tray History Submenu
- 20 pre-allocated slots in "History" submenu
- Shows timestamp + truncated text (60 chars max)
- Click copies full text to clipboard
- Auto-refreshes when state returns to idle/listening
- Shows count in submenu title: "History (5)"

### Open Questions

- None currently blocking

### Next Steps

Likely candidates for next development:
- Commit current changes
- Add more voice commands (backspace, delete, escape, etc.)
- History search/filter in tray
- Waybar module update to show last transcription
- Settings UI in tray (verbosity, sounds on/off)
