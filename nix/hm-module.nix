{ config, lib, pkgs, ... }:

let
  cfg = config.services.moonshine;
  inherit (lib) mkEnableOption mkOption mkIf types;

  # Convert Nix settings attrset to KEY=VALUE config file content
  boolToOnOff = b: if b then "on" else "off";

  configLines = lib.concatStringsSep "\n" (lib.filter (s: s != "") [
    (lib.optionalString (cfg.settings.device != "") "DEVICE=${cfg.settings.device}")
    "LANGUAGE=${cfg.settings.language}"
    "BACKEND=${cfg.settings.backend}"
    "AUTO_PUNCTUATION=${boolToOnOff cfg.settings.autoPunctuation}"
    "AUTO_CAPITALIZE=${boolToOnOff cfg.settings.autoCapitalize}"
    "FILLER_REMOVAL=${boolToOnOff cfg.settings.fillerRemoval}"
    "VOICE_COMMANDS=${boolToOnOff cfg.settings.voiceCommands}"
    "NUMBER_FORMAT=${cfg.settings.numberFormat}"
    "SENTENCE_END=${cfg.settings.sentenceEnd}"
    "SILENCE_TIMEOUT=${toString cfg.settings.silenceTimeout}"
    (lib.optionalString (cfg.settings.whisperModel != "") "WHISPER_MODEL=${cfg.settings.whisperModel}")
    "THREADS=${toString cfg.settings.threads}"
  ]);

  macrosLines = lib.concatStringsSep "\n"
    (lib.mapAttrsToList (phrase: replacement: "${phrase} = ${replacement}") cfg.macros);

in {
  options.services.moonshine = {
    enable = mkEnableOption "Moonshine voice-to-text daemon";

    package = mkOption {
      type = types.package;
      description = "The moonshine package to use.";
    };

    settings = {
      device = mkOption {
        type = types.str;
        default = "";
        description = "PipeWire audio input device substring filter. Empty for system default.";
        example = "PRO X";
      };

      language = mkOption {
        type = types.enum [ "en" "es" "ar" "ja" "de" ];
        default = "en";
        description = "Transcription language.";
      };

      backend = mkOption {
        type = types.enum [ "moonshine" "whisper" ];
        default = "moonshine";
        description = "Transcription backend engine.";
      };

      autoPunctuation = mkOption {
        type = types.bool;
        default = true;
        description = "Automatically insert sentence-ending punctuation.";
      };

      autoCapitalize = mkOption {
        type = types.bool;
        default = true;
        description = "Capitalize first letter and after sentence-ending punctuation.";
      };

      fillerRemoval = mkOption {
        type = types.bool;
        default = true;
        description = "Remove filler words (um, uh, you know, etc).";
      };

      voiceCommands = mkOption {
        type = types.bool;
        default = true;
        description = "Expand voice commands (new line, period, asterisk, etc).";
      };

      numberFormat = mkOption {
        type = types.enum [ "words" "digits" ];
        default = "words";
        description = "Number output format. 'digits' converts 'twenty three' to '23'.";
      };

      sentenceEnd = mkOption {
        type = types.str;
        default = ".";
        description = "Punctuation appended at sentence end. Set to 'none' to disable.";
      };

      silenceTimeout = mkOption {
        type = types.ints.between 0 30;
        default = 3;
        description = "Seconds of silence before auto-stopping push-to-talk. 0 = manual stop only.";
      };

      whisperModel = mkOption {
        type = types.str;
        default = "";
        description = "Path to Whisper GGML model file. Only used when backend = whisper.";
      };

      threads = mkOption {
        type = types.ints.between 1 32;
        default = 4;
        description = "CPU threads for Whisper transcription.";
      };
    };

    macros = mkOption {
      type = types.attrsOf types.str;
      default = {};
      description = "User-defined voice macros. Keys are spoken phrases, values are replacements.";
      example = lib.literalExpression ''
        {
          "my email" = "user@example.com";
          "shebang" = "#!/usr/bin/env bash";
        }
      '';
    };

    verbose = mkOption {
      type = types.bool;
      default = false;
      description = "Enable verbose daemon logging.";
    };

    enableTray = mkOption {
      type = types.bool;
      default = true;
      description = "Show system tray icon.";
    };
  };

  config = mkIf cfg.enable {
    # Install the package
    home.packages = [ cfg.package ];

    # Generate config file
    xdg.configFile."moonshine/config" = {
      text = configLines + "\n";
    };

    # Generate macros file (only if macros are defined)
    xdg.configFile."moonshine/macros" = mkIf (cfg.macros != {}) {
      text = macrosLines + "\n";
    };

    # Systemd user service
    systemd.user.services.moonshine-daemon = {
      Unit = {
        Description = "Moonshine voice-to-text daemon";
        After = [ "graphical-session.target" "pipewire.service" ];
        PartOf = [ "graphical-session.target" ];
      };

      Service = {
        Type = "simple";
        ExecStart = lib.concatStringsSep " " ([
          "${cfg.package}/bin/moonshine-daemon"
        ] ++ lib.optional cfg.verbose "--verbose"
          ++ lib.optional (!cfg.enableTray) "--no-tray");
        Restart = "on-failure";
        RestartSec = 5;
      };

      Install = {
        WantedBy = [ "graphical-session.target" ];
      };
    };
  };
}
