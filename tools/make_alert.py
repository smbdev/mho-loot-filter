"""Converts a sound file into the Wwise PCM media the filter plays when an alerted item drops.

usage: python tools/make_alert.py <sound file>   (needs: pip install miniaudio)

Writes internal/wwise/alert.wem: 16-bit mono 44.1 kHz PCM, normalised to just under full scale, laid out like the PCM sounds the game ships
(fmt with a packed channel config, then a JUNK chunk that aligns the samples to 16 bytes).
"""
import array
import os
import struct
import sys

import miniaudio

RATE = 44100
PEAK = 0.97  # of full scale; game sounds sit well below this, so the alert stands out
OUT = os.path.join(os.path.dirname(__file__), '..', 'internal', 'wwise', 'alert.wem')
MONO_CENTER = 0x4101  # AkChannelConfig: 1 channel, standard layout, front center


def wem(samples):
    fmt = struct.pack('<HHIIHHHHI', 0xFFFE, 1, RATE, RATE * 2, 2, 16, 6, 0, MONO_CENTER)
    body = b'WAVE' + b'fmt ' + struct.pack('<I', len(fmt)) + fmt + b'JUNK' + struct.pack('<I', 4) + bytes(4)
    body += b'data' + struct.pack('<I', len(samples)) + samples
    return b'RIFF' + struct.pack('<I', len(body)) + body


def main(path):
    sound = miniaudio.decode_file(path, output_format=miniaudio.SampleFormat.SIGNED16, nchannels=1, sample_rate=RATE)
    samples = sound.samples
    gain = PEAK * 32767 / max(1, max(abs(s) for s in samples))
    data = wem(array.array('h', (round(s * gain) for s in samples)).tobytes())
    with open(OUT, 'wb') as f:
        f.write(data)
    print(f'{sound.duration:.2f} s, {len(data)} bytes -> {os.path.normpath(OUT)}')


if __name__ == '__main__':
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    main(sys.argv[1])
