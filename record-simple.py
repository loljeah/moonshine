#!/usr/bin/env python3
"""
Simple continuous recorder - records until lock file is removed.
Saves audio as raw float32 PCM at 16kHz mono.
"""

import sounddevice as sd
import numpy as np
import os
import signal
import sys
import time

AUDIO_FILE = "/tmp/moonshine/audio.raw"
LOCK_FILE = "/tmp/moonshine/recording.lock"
SAMPLE_RATE = 16000

audio_chunks = []
running = True

def handle_signal(signum, frame):
    global running
    running = False

def audio_callback(indata, frames, time_info, status):
    if status:
        print(f"Audio status: {status}", file=sys.stderr)
    audio_chunks.append(indata.copy().flatten())

signal.signal(signal.SIGTERM, handle_signal)
signal.signal(signal.SIGINT, handle_signal)

try:
    with sd.InputStream(samplerate=SAMPLE_RATE, channels=1, dtype="float32", callback=audio_callback):
        while running and os.path.exists(LOCK_FILE):
            time.sleep(0.05)
except Exception as e:
    print(f"Recording error: {e}", file=sys.stderr)
    sys.exit(1)

# Save audio
if audio_chunks:
    audio = np.concatenate(audio_chunks)
    # Normalize audio
    max_val = np.max(np.abs(audio))
    if max_val > 0:
        audio = audio / max_val * 0.95
    audio.astype(np.float32).tofile(AUDIO_FILE)
