#!/usr/bin/env python3
"""
Transcribe audio file using Moonshine.
Prints transcription to stdout.
"""

import numpy as np
import sys
import os

AUDIO_FILE = "/tmp/moonshine/audio.raw"
CONFIG_FILE = os.path.expanduser("~/.config/moonshine/config")

def read_config(key, default=None):
    """Read a key=value from the config file."""
    if not os.path.exists(CONFIG_FILE):
        return default
    try:
        with open(CONFIG_FILE) as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith("#"):
                    continue
                if "=" in line:
                    k, v = line.split("=", 1)
                    if k.strip() == key:
                        return v.strip() or default
    except Exception:
        pass
    return default

def main():
    if not os.path.exists(AUDIO_FILE):
        return

    try:
        audio = np.fromfile(AUDIO_FILE, dtype=np.float32)
    except Exception:
        return

    # Need at least 0.1 seconds of audio
    if len(audio) < 1600:
        return

    try:
        from moonshine_voice import get_model_for_language
        from moonshine_voice.transcriber import Transcriber

        language = read_config("LANGUAGE", "en")
        model_path, model_arch = get_model_for_language(language)
        transcriber = Transcriber(str(model_path), model_arch)
        result = transcriber.transcribe_without_streaming(audio.tolist(), 16000)

        if result and result.lines:
            text = " ".join(line.text.strip() for line in result.lines if line.text.strip())
            if text:
                print(text)
    except Exception as e:
        print(f"Transcription error: {e}", file=sys.stderr)

if __name__ == "__main__":
    main()
