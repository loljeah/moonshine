{
  description = "Moonshine Voice-to-Text — Local offline speech recognition daemon";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    let
      # Home Manager module (system-independent)
      hmModule = import ./nix/hm-module.nix;

      # Per-system outputs
      perSystem = flake-utils.lib.eachDefaultSystem (system:
        let
          pkgs = import nixpkgs { inherit system; };

          # ── Native libraries ─────────────────────────────────────────────

          moonshine-voice-version = "0.0.51";

          moonshine-voice-wheel = pkgs.fetchurl {
            url = "https://files.pythonhosted.org/packages/a8/f0/1e198a7ed7a5dc8c7024cb2e394eddc71c23e01197d8e19dfade196a383d/moonshine_voice-${moonshine-voice-version}-py3-none-manylinux_2_34_x86_64.whl";
            sha256 = "0da71c6e3021a412924dab67283634d9c1d13903ded42d10ca83c44881bd3e0b";
          };

          onnxruntime-version = "1.24.4";

          onnxruntime-wheel = pkgs.fetchurl {
            url = "https://files.pythonhosted.org/packages/d5/b6/7a4df417cdd01e8f067a509e123ac8b31af450a719fa7ed81787dd6057ec/onnxruntime-${onnxruntime-version}-cp311-cp311-manylinux_2_27_x86_64.manylinux_2_28_x86_64.whl";
            sha256 = "e54ad52e61d2d4618dcff8fa1480ac66b24ee2eab73331322db1049f11ccf330";
          };

          moonshine-libs = pkgs.stdenv.mkDerivation {
            pname = "moonshine-libs";
            version = moonshine-voice-version;
            dontUnpack = true;
            nativeBuildInputs = [ pkgs.unzip ];

            buildPhase = ''
              mkdir -p moonshine onnx
              unzip -q ${moonshine-voice-wheel} -d moonshine
              unzip -q ${onnxruntime-wheel} -d onnx
            '';

            installPhase = ''
              mkdir -p $out/lib $out/share/moonshine
              cp moonshine/moonshine_voice/libmoonshine.so $out/lib/
              cp -r moonshine/moonshine_voice.libs/* $out/lib/ 2>/dev/null || true
              cp onnx/onnxruntime/capi/*.so* $out/lib/ 2>/dev/null || true
              cp -r moonshine/moonshine_voice/assets $out/share/moonshine/
            '';

            postFixup = ''
              ${pkgs.patchelf}/bin/patchelf \
                --set-rpath "$out/lib:${pkgs.stdenv.cc.cc.lib}/lib" \
                $out/lib/libmoonshine.so || true

              for lib in $out/lib/libonnxruntime*.so*; do
                [ -f "$lib" ] && ${pkgs.patchelf}/bin/patchelf \
                  --set-rpath "$out/lib:${pkgs.stdenv.cc.cc.lib}/lib" \
                  "$lib" 2>/dev/null || true
              done
            '';
          };

          # Runtime deps that must be in PATH for the daemon
          runtimeDeps = with pkgs; [
            pipewire
            wl-clipboard
            wtype
            libnotify
          ];

          # ── Go daemon package ────────────────────────────────────────────

          moonshine = pkgs.buildGoModule {
            pname = "moonshine";
            version = "0.2.0";
            src = ./.;
            vendorHash = null;

            nativeBuildInputs = [
              pkgs.pkg-config
              pkgs.makeWrapper
            ];

            buildInputs = [
              moonshine-libs
              pkgs.dbus
              pkgs.libayatana-appindicator
              pkgs.gtk3
              pkgs.glib
            ];

            env = {
              CGO_ENABLED = "1";
              CGO_CFLAGS = "-I${moonshine-libs}/lib";
              CGO_LDFLAGS = "-L${moonshine-libs}/lib -lmoonshine -Wl,-rpath,${moonshine-libs}/lib";
            };

            subPackages = [ "cmd/moonshine-daemon" "cmd/moonshine-ctl" ];

            postInstall = ''
              for bin in $out/bin/*; do
                wrapProgram "$bin" \
                  --prefix LD_LIBRARY_PATH : "${moonshine-libs}/lib" \
                  --set MOONSHINE_MODEL_PATH "${moonshine-libs}/share/moonshine/assets" \
                  --prefix PATH : "${pkgs.lib.makeBinPath runtimeDeps}"
              done
            '';

            meta = with pkgs.lib; {
              description = "Moonshine voice-to-text daemon";
              homepage = "https://github.com/loljeah/moonshine";
              license = licenses.mit;
              platforms = platforms.linux;
              mainProgram = "moonshine-daemon";
            };
          };

          # ── Rofi launcher ────────────────────────────────────────────────

          moonshine-rofi = pkgs.writeShellScriptBin "moonshine-rofi" ''
            set -euo pipefail

            CTL="${moonshine}/bin/moonshine-ctl"

            # Get current state for the menu
            status=$($CTL status 2>/dev/null | sed 's/^OK //' || echo "offline")
            fs=$($CTL freespeech 2>/dev/null | sed 's/^OK //' || echo "off")

            if [ "$fs" = "on" ]; then
              trigger_label="Always Listening  [active]"
              ptt_label="Press to Talk"
            else
              trigger_label="Always Listening"
              ptt_label="Press to Talk  [active]"
            fi

            choice=$(printf '%s\n' \
              "Toggle Recording" \
              "$trigger_label" \
              "$ptt_label" \
              "---" \
              "Clipboard Mode" \
              "Type Mode" \
              "---" \
              "Scratch That" \
              "Status: $status" \
              "---" \
              "Quit Daemon" \
            | ${pkgs.rofi}/bin/rofi -dmenu -i -p "Moonshine" -no-custom \
                -theme-str 'window { width: 300px; }')

            case "$choice" in
              "Toggle Recording")
                result=$($CTL toggle 2>&1)
                ${pkgs.libnotify}/bin/notify-send -a Moonshine "Toggle" "$result" -t 2000
                ;;
              *"Always Listening"*)
                $CTL freespeech on >/dev/null 2>&1
                ${pkgs.libnotify}/bin/notify-send -a Moonshine "Always Listening" "Enabled" -t 2000
                ;;
              *"Press to Talk"*)
                $CTL freespeech off >/dev/null 2>&1
                ${pkgs.libnotify}/bin/notify-send -a Moonshine "Press to Talk" "Enabled" -t 2000
                ;;
              "Clipboard Mode")
                $CTL mode clipboard >/dev/null 2>&1
                ${pkgs.libnotify}/bin/notify-send -a Moonshine "Mode" "Clipboard" -t 2000
                ;;
              "Type Mode")
                $CTL mode type >/dev/null 2>&1
                ${pkgs.libnotify}/bin/notify-send -a Moonshine "Mode" "Type" -t 2000
                ;;
              "Scratch That")
                result=$($CTL scratch 2>&1)
                ${pkgs.libnotify}/bin/notify-send -a Moonshine "Scratch" "$result" -t 2000
                ;;
              "Status: "*)
                result=$($CTL status 2>&1)
                ${pkgs.libnotify}/bin/notify-send -a Moonshine "Status" "$result" -t 3000
                ;;
              "Quit Daemon")
                $CTL quit >/dev/null 2>&1
                ${pkgs.libnotify}/bin/notify-send -a Moonshine "Daemon" "Stopped" -t 2000
                ;;
            esac
          '';

        in {
          packages = {
            inherit moonshine moonshine-libs moonshine-rofi;
            default = moonshine;
          };

          # Development shell
          devShells.default = pkgs.mkShell {
            name = "moonshine-dev";

            packages = with pkgs; [
              go
              gcc
              pkg-config
              dbus
              libayatana-appindicator
              gtk3
              glib
            ] ++ runtimeDeps;

            CGO_ENABLED = "1";
            CGO_CFLAGS = "-I${moonshine-libs}/lib";
            CGO_LDFLAGS = "-L${moonshine-libs}/lib -lmoonshine -Wl,-rpath,${moonshine-libs}/lib";
            LD_LIBRARY_PATH = "${moonshine-libs}/lib";
            MOONSHINE_MODEL_PATH = "${moonshine-libs}/share/moonshine/assets";

            shellHook = ''
              echo "Moonshine dev environment"
              echo "  go build ./cmd/...    Build binaries"
              echo "  nix build             Build flake package"
            '';
          };
        }
      );

    in
      perSystem // {
        # Home Manager module — import in your HM config:
        #   imports = [ moonshine.homeManagerModules.default ];
        homeManagerModules.default = hmModule;
        homeManagerModules.moonshine = hmModule;

        # Overlay for adding moonshine to pkgs
        overlays.default = final: prev: {
          moonshine = perSystem.packages.${prev.system}.moonshine;
          moonshine-rofi = perSystem.packages.${prev.system}.moonshine-rofi;
        };
      };
}
