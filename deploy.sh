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
mkdir -p ~/.local/bin
mkdir -p ~/.config/moonshine
mkdir -p /tmp/moonshine

# Create default config if it doesn't exist
if [ ! -f ~/.config/moonshine/config ]; then
    cat > ~/.config/moonshine/config << 'CONF'
# Moonshine Voice-to-Text Configuration
#
# DEVICE - Audio input device
#   Leave empty for system default.
#   Use PipeWire node name or a substring to match (e.g. "PRO X" for headset)
#   List sources: pw-cli list-objects Node | grep -A2 "Audio/Source"
DEVICE=

# LANGUAGE - Transcription language (default: en)
LANGUAGE=en
CONF
    ok "config -> ~/.config/moonshine/config"
else
    ok "config exists (kept)"
fi

# --- Go daemon binaries ---
if [ -f "$SCRIPT_DIR/cmd/moonshine-daemon/main.go" ]; then
    echo ""
    echo "Building Go binaries..."
    if command -v go &>/dev/null; then
        GO_CMD="go"
    elif [ -f "$SCRIPT_DIR/shell-go.nix" ]; then
        GO_CMD="nix-shell $SCRIPT_DIR/shell-go.nix --run"
        # Build inside nix-shell for cgo deps
        nix-shell "$SCRIPT_DIR/shell-go.nix" --run "
            cd '$SCRIPT_DIR'
            go build -o moonshine-daemon ./cmd/moonshine-daemon && \
            go build -o moonshine-ctl ./cmd/moonshine-ctl
        " && {
            cp moonshine-daemon ~/.local/bin/moonshine-daemon && ok "moonshine-daemon -> ~/.local/bin/"
            cp moonshine-ctl ~/.local/bin/moonshine-ctl && ok "moonshine-ctl -> ~/.local/bin/"
            chmod +x ~/.local/bin/moonshine-daemon ~/.local/bin/moonshine-ctl
            rm -f moonshine-daemon moonshine-ctl
        } || warn "Go build failed — see shell-go.nix"
    else
        warn "Go not found and no shell-go.nix — skipping daemon build"
    fi
fi

# --- Legacy scripts (kept for fallback) ---
cp shell.nix ~/.local/share/moonshine/ 2>/dev/null && ok "shell.nix (legacy)"
cp transcribe.py ~/.local/share/moonshine/ 2>/dev/null && ok "transcribe.py (legacy)"
cp record-simple ~/.local/share/moonshine/ 2>/dev/null && ok "record-simple (legacy)"
cp voice-toggle ~/.local/share/moonshine/ 2>/dev/null && ok "voice-toggle (legacy)"

# Legacy symlinks
ln -sf ~/.local/share/moonshine/voice-toggle ~/.local/bin/voice-toggle 2>/dev/null && ok "symlink voice-toggle (legacy)"

# Waybar module (still useful as fallback status indicator)
cp waybar-moonshine ~/.local/bin/ 2>/dev/null && ok "waybar-moonshine -> ~/.local/bin/"
chmod +x ~/.local/bin/waybar-moonshine 2>/dev/null

# Make legacy scripts executable
chmod +x ~/.local/share/moonshine/voice-toggle 2>/dev/null
chmod +x ~/.local/share/moonshine/transcribe.py 2>/dev/null
chmod +x ~/.local/share/moonshine/record-simple 2>/dev/null

# Generate sounds
if [[ -f generate-sounds.sh ]]; then
    bash generate-sounds.sh 2>/dev/null && ok "notification sounds" || warn "sound generation skipped"
fi

# Initialize state
echo "idle" > /tmp/moonshine/status

echo ""
ok "Moonshine deployed."
echo ""
echo "  Go daemon (recommended):"
echo "    exec ~/.local/bin/moonshine-daemon"
echo "    bindsym --whole-window button8 exec ~/.local/bin/moonshine-ctl toggle clipboard"
echo "    bindsym --whole-window \$mod+button8 exec ~/.local/bin/moonshine-ctl toggle type"
echo ""
echo "  Legacy bash (fallback):"
echo "    bindsym --whole-window button8 exec ~/.local/bin/voice-toggle clipboard"
echo "    bindsym --whole-window \$mod+button8 exec ~/.local/bin/voice-toggle type"
