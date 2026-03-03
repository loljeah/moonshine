#!/usr/bin/env bash
# Moonshine Voice-to-Text - Deploy Script
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

ok()   { printf "${GREEN}  [ok]${NC}  %s\n" "$*"; }
warn() { printf "${YELLOW}  [!]${NC}  %s\n" "$*"; }

echo "── Moonshine Voice-to-Text ──────────────────────────────────"

# Create directories
mkdir -p ~/.local/share/moonshine
mkdir -p ~/.local/share/moonshine/sounds
mkdir -p /tmp/moonshine

# Deploy core files
cp shell.nix ~/.local/share/moonshine/ && ok "shell.nix"
cp transcribe.py ~/.local/share/moonshine/ && ok "transcribe.py"
cp record-simple.py ~/.local/share/moonshine/ && ok "record-simple.py"
cp voice-toggle ~/.local/share/moonshine/ && ok "voice-toggle"

# Legacy scripts
cp moonshine-voice ~/.local/share/moonshine/ 2>/dev/null && ok "moonshine-voice"
cp voice-type ~/.local/share/moonshine/ 2>/dev/null && ok "voice-type"
cp voice-record ~/.local/share/moonshine/ 2>/dev/null && ok "voice-record"
cp record.py ~/.local/share/moonshine/ 2>/dev/null && ok "record.py"

# Waybar module
cp waybar-moonshine ~/.local/bin/ && ok "waybar-moonshine -> ~/.local/bin/"

# Make executable
chmod +x ~/.local/bin/waybar-moonshine
chmod +x ~/.local/share/moonshine/voice-toggle
chmod +x ~/.local/share/moonshine/transcribe.py
chmod +x ~/.local/share/moonshine/record-simple.py
chmod +x ~/.local/share/moonshine/moonshine-voice 2>/dev/null
chmod +x ~/.local/share/moonshine/voice-type 2>/dev/null
chmod +x ~/.local/share/moonshine/voice-record 2>/dev/null
chmod +x ~/.local/share/moonshine/record.py 2>/dev/null

# Symlinks to PATH
ln -sf ~/.local/share/moonshine/voice-toggle ~/.local/bin/voice-toggle && ok "symlink voice-toggle"
ln -sf ~/.local/share/moonshine/moonshine-voice ~/.local/bin/moonshine-voice 2>/dev/null && ok "symlink moonshine-voice"
ln -sf ~/.local/share/moonshine/voice-type ~/.local/bin/voice-type 2>/dev/null && ok "symlink voice-type"
ln -sf ~/.local/share/moonshine/voice-record ~/.local/bin/voice-record 2>/dev/null && ok "symlink voice-record"

# Generate sounds
if [[ -f generate-sounds.sh ]]; then
    bash generate-sounds.sh 2>/dev/null && ok "notification sounds" || warn "sound generation skipped"
fi

# Initialize state
echo "idle" > /tmp/moonshine/status

echo ""
ok "Moonshine deployed. Add keybindings to sway config:"
echo "    bindsym --whole-window button8 exec ~/.local/bin/voice-toggle clipboard"
echo "    bindsym --whole-window \$mod+button8 exec ~/.local/bin/voice-toggle type"
