# Moonshine Voice - Speech-to-Text AI
# Usage: nix-shell shell.nix
# Or just run the scripts directly after first setup

{ pkgs ? import <nixpkgs> {} }:

let
  pythonEnv = pkgs.python311.withPackages (ps: with ps; [
    pip
    numpy
    sounddevice
    requests
    tqdm
    filelock
    platformdirs
    cffi
    tokenizers
  ]);
in
pkgs.mkShell {
  name = "moonshine-voice";

  packages = [
    pythonEnv
    pkgs.portaudio
    pkgs.alsa-utils
    pkgs.wl-clipboard
    pkgs.wtype
    pkgs.libnotify
  ];

  shellHook = ''
    export MOONSHINE_DIR="$HOME/.local/share/moonshine-pkgs"
    export LD_LIBRARY_PATH="/run/current-system/sw/share/nix-ld/lib''${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
    export PYTHONPATH="$MOONSHINE_DIR:''${PYTHONPATH:-}"

    # Install moonshine-voice if not present (silently)
    if [ ! -f "$MOONSHINE_DIR/.installed" ]; then
      mkdir -p "$MOONSHINE_DIR" 2>/dev/null
      pip install --target="$MOONSHINE_DIR" --quiet moonshine-voice --no-deps 2>/dev/null
      pip install --target="$MOONSHINE_DIR" --quiet onnxruntime 2>/dev/null
      touch "$MOONSHINE_DIR/.installed"
    fi

    # Add this directory to PATH for scripts
    export PATH="${toString ./.}:$PATH"

    # Only show help in fully interactive shell (not nix-shell --run)
    if [ -t 0 ] && [ -t 1 ] && [ -z "$NIX_SHELL_RUN" ]; then
      echo ""
      echo "Moonshine Voice ready"
      echo "  moonshine-voice [seconds]     - Transcribe to stdout"
      echo "  voice-type [seconds] [mode]   - Clipboard/type mode"
      echo "  Modes: clipboard (default), type, both"
      echo ""
    fi
  '';
}
