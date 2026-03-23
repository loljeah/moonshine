{
  description = "Moonshine Voice-to-Text - Local speech recognition daemon";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };

        # Moonshine voice wheel (contains libmoonshine.so + models)
        moonshine-voice-version = "0.0.51";
        moonshine-voice-wheel = pkgs.fetchurl {
          url = "https://files.pythonhosted.org/packages/a8/f0/1e198a7ed7a5dc8c7024cb2e394eddc71c23e01197d8e19dfade196a383d/moonshine_voice-${moonshine-voice-version}-py3-none-manylinux_2_34_x86_64.whl";
          sha256 = "0da71c6e3021a412924dab67283634d9c1d13903ded42d10ca83c44881bd3e0b";
        };

        # ONNX Runtime wheel (libonnxruntime.so)
        onnxruntime-version = "1.24.4";
        onnxruntime-wheel = pkgs.fetchurl {
          url = "https://files.pythonhosted.org/packages/d5/b6/7a4df417cdd01e8f067a509e123ac8b31af450a719fa7ed81787dd6057ec/onnxruntime-${onnxruntime-version}-cp311-cp311-manylinux_2_27_x86_64.manylinux_2_28_x86_64.whl";
          sha256 = "e54ad52e61d2d4618dcff8fa1480ac66b24ee2eab73331322db1049f11ccf330";
        };

        # Extract wheels and prepare library directories
        moonshine-libs = pkgs.stdenv.mkDerivation {
          pname = "moonshine-libs";
          version = moonshine-voice-version;

          dontUnpack = true;

          nativeBuildInputs = [ pkgs.unzip ];

          buildPhase = ''
            mkdir -p moonshine onnx

            # Extract moonshine_voice wheel
            unzip -q ${moonshine-voice-wheel} -d moonshine

            # Extract onnxruntime wheel
            unzip -q ${onnxruntime-wheel} -d onnx
          '';

          installPhase = ''
            mkdir -p $out/lib $out/share/moonshine

            # Copy libmoonshine.so
            cp moonshine/moonshine_voice/libmoonshine.so $out/lib/

            # Copy moonshine_voice.libs (contains vendored onnxruntime)
            cp -r moonshine/moonshine_voice.libs/* $out/lib/ 2>/dev/null || true

            # Copy onnxruntime libs as fallback
            cp onnx/onnxruntime/capi/*.so* $out/lib/ 2>/dev/null || true

            # Copy assets directory (contains models like tiny-en/)
            cp -r moonshine/moonshine_voice/assets $out/share/moonshine/
          '';

          # Patch ELF to find libraries
          postFixup = ''
            # Fix libmoonshine.so rpath
            ${pkgs.patchelf}/bin/patchelf --set-rpath "$out/lib:${pkgs.stdenv.cc.cc.lib}/lib" $out/lib/libmoonshine.so || true

            # Fix onnxruntime rpath
            for lib in $out/lib/libonnxruntime*.so*; do
              [ -f "$lib" ] && ${pkgs.patchelf}/bin/patchelf --set-rpath "$out/lib:${pkgs.stdenv.cc.cc.lib}/lib" "$lib" 2>/dev/null || true
            done
          '';
        };

        # Go daemon binaries
        moonshine-daemon = pkgs.buildGoModule {
          pname = "moonshine-daemon";
          version = "0.1.0";

          src = ./.;

          vendorHash = "sha256-/FkH/3+MhMaOjQPZNghNOFY36FhZbWJVQND47ae0pLM=";

          nativeBuildInputs = [ pkgs.pkg-config ];

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
            # Create wrapper that sets up library path
            for bin in $out/bin/*; do
              mv "$bin" "$bin-unwrapped"
              cat > "$bin" << EOF
            #!${pkgs.bash}/bin/bash
            export LD_LIBRARY_PATH="${moonshine-libs}/lib:\''${LD_LIBRARY_PATH:-}"
            export MOONSHINE_MODEL_PATH="${moonshine-libs}/share/moonshine/assets"
            exec "$bin-unwrapped" "\$@"
            EOF
              chmod +x "$bin"
            done
          '';

          meta = with pkgs.lib; {
            description = "Moonshine voice-to-text daemon";
            homepage = "https://github.com/loljeah/moonshine";
            license = licenses.mit;
            platforms = platforms.linux;
          };
        };

        # Wrapper script for starting the daemon (compatibility with existing setup)
        moonshine-daemon-start = pkgs.writeShellScriptBin "moonshine-daemon-start" ''
          export LD_LIBRARY_PATH="${moonshine-libs}/lib:''${LD_LIBRARY_PATH:-}"
          export MOONSHINE_MODEL_PATH="${moonshine-libs}/share/moonshine/assets"
          export PATH="${pkgs.pipewire}/bin:${pkgs.wl-clipboard}/bin:${pkgs.wtype}/bin:${pkgs.libnotify}/bin:$PATH"
          exec ${moonshine-daemon}/bin/moonshine-daemon "$@"
        '';

        # Combined package
        moonshine = pkgs.symlinkJoin {
          name = "moonshine";
          paths = [ moonshine-daemon moonshine-daemon-start ];
          meta.mainProgram = "moonshine-daemon";
        };

      in {
        packages = {
          inherit moonshine moonshine-daemon moonshine-libs moonshine-daemon-start;
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
            pipewire
            wl-clipboard
            wtype
            libnotify
          ];

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
}
