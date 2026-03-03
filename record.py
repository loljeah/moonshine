#!/usr/bin/env python3
"""Record audio until stop file appears, then save and exit."""

import sounddevice as sd
import numpy as np
import os
import sys

AUDIO_FILE = "/tmp/moonshine-audio.raw"
LOCK_FILE = "/tmp/moonshine-recording.lock"
STOP_FILE = "/tmp/moonshine-stop"
SAMPLE_RATE = 16000

audio_chunks = []

def should_stop():
    return os.path.exists(STOP_FILE)

try:
    with sd.InputStream(samplerate=SAMPLE_RATE, channels=1, dtype="float32") as stream:
        while not should_stop():
            chunk, _ = stream.read(1600)  # 100ms chunks
            audio_chunks.append(chunk.flatten())
except Exception as e:
    print(f"Recording error: {e}", file=sys.stderr)

# Save audio
if audio_chunks:
    audio = np.concatenate(audio_chunks)
    audio.tofile(AUDIO_FILE)

# Remove lock to signal we're done
if os.path.exists(LOCK_FILE):
    os.remove(LOCK_FILE)
