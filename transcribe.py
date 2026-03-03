#!/usr/bin/env python3
"""
Transcribe audio file using Moonshine.
Prints transcription to stdout.
"""

import numpy as np
import sys
import os

AUDIO_FILE = "/tmp/moonshine/audio.raw"

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

        model_path, model_arch = get_model_for_language("en")
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
