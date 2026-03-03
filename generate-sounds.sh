#!/usr/bin/env bash
#
# Generate notification sounds for Moonshine using sox
#

SOUND_DIR="$HOME/.local/share/moonshine/sounds"
mkdir -p "$SOUND_DIR"

# Generate sounds with sox (via nix-shell if not installed)
generate() {
    nix-shell -p sox --run "
        sox -n '$SOUND_DIR/start.wav' synth 0.15 sine 880 fade 0 0.15 0.05 vol 0.5
        sox -n '$SOUND_DIR/stop.wav' synth 0.15 sine 660 fade 0 0.15 0.05 vol 0.5
        sox -n '$SOUND_DIR/success.wav' synth 0.1 sine 880 : synth 0.1 sine 1100 fade 0 0.2 0.05 vol 0.5
        sox -n '$SOUND_DIR/error.wav' synth 0.3 sine 330 fade 0 0.3 0.1 vol 0.4
    " 2>/dev/null
}

if [ ! -f "$SOUND_DIR/start.wav" ]; then
    generate
    echo "✓ Sounds generated in $SOUND_DIR"
fi
