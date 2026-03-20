# Moonshine Go Daemon - Development Environment
# Usage: nix-shell shell-go.nix
{ pkgs ? import <nixpkgs> {} }:

let
  moonshineDir = builtins.getEnv "HOME" + "/.local/share/moonshine-pkgs";
  moonshineLib = "${moonshineDir}/moonshine_voice";
  onnxruntimeLib = "${moonshineDir}/moonshine_voice.libs";
in
pkgs.mkShell {
  name = "moonshine-go";

  packages = with pkgs; [
    go
    gcc
    pkg-config

    # System tray (fyne.io/systray SNI/D-Bus)
    dbus
    libayatana-appindicator
    gtk3
    glib

    # Audio tools
    pipewire

    # Output tools
    wl-clipboard
    wtype
    libnotify
  ];

  CGO_ENABLED = "1";

  shellHook = ''
    export MOONSHINE_LIB="${moonshineLib}"
    export ONNXRUNTIME_LIB="${onnxruntimeLib}"

    # CGO flags for linking libmoonshine.so
    export CGO_CFLAGS="-I${moonshineLib}"
    export CGO_LDFLAGS="-L${moonshineLib} -L${onnxruntimeLib} -lmoonshine -Wl,-rpath,${moonshineLib} -Wl,-rpath,${onnxruntimeLib}"

    # Runtime library path
    export LD_LIBRARY_PATH="${moonshineLib}:${onnxruntimeLib}''${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"

    # Verify libraries exist
    if [ ! -f "${moonshineLib}/libmoonshine.so" ]; then
      echo "WARNING: libmoonshine.so not found at ${moonshineLib}"
      echo "  Install moonshine-voice first: nix-shell shell.nix"
    fi

    if [ -t 0 ] && [ -t 1 ]; then
      echo ""
      echo "Moonshine Go dev environment ready"
      echo "  go build ./cmd/...        Build all binaries"
      echo "  go run ./cmd/moonshine-daemon  Run daemon"
      echo "  go run ./cmd/moonshine-ctl     Run CLI"
      echo ""
    fi
  '';
}
