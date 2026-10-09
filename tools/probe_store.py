"""The file store of the touch probe, built on a computer.

python3 tools/probe_store.py store.bin file...

Writes the store of firmware/touch (a table sector, then each file gzipped
from a sector boundary) for 0x150000 of the ESP32-C3 flash, so the first
files go with the firmware, before there is a Wi-Fi page to put them on:
  esptool.py --chip esp32c3 write_flash 0x0 touch-c3.bin 0x150000 store.bin
pluttan
"""

import gzip
import hashlib
import os
import struct
import sys

STORE, STORE_END, SECTOR, SLOTS = 0x150000, 0x3F0000, 4096, 63


def build(paths):
    table = bytearray(b'\xff' * SECTOR)
    data = bytearray()
    for i, path in enumerate(paths):
        if i >= SLOTS:
            sys.exit('probe_store: no more than %d files' % SLOTS)
        name = os.path.basename(path) + '.gz'
        if len(name.encode()) > 35:
            sys.exit('probe_store: name longer than 35 bytes: ' + name)
        body = gzip.compress(open(path, 'rb').read(), 9, mtime=0)
        off = SECTOR + len(data)
        entry = struct.pack('<III', 1, off, len(body)) + hashlib.md5(body).digest()
        entry += name.encode().ljust(36, b'\0')
        table[64 + 64 * i:128 + 64 * i] = entry
        data += body + b'\xff' * (-len(body) % SECTOR)
        print('%-36s %8d bytes' % (name, len(body)))
    out = bytes(table) + bytes(data)
    if STORE + len(out) > STORE_END:
        sys.exit('probe_store: %d bytes, the store takes %d' % (len(out), STORE_END - STORE))
    return out


if __name__ == '__main__':
    if len(sys.argv) < 3:
        sys.exit(__doc__)
    open(sys.argv[1], 'wb').write(build(sys.argv[2:]))
