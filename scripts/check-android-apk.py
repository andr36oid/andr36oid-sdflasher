#!/usr/bin/env python3
import struct
import sys
import zipfile

abis = set()
with zipfile.ZipFile(sys.argv[1]) as apk:
    for name in apk.namelist():
        if not name.startswith('lib/') or not name.endswith('.so'):
            continue
        abis.add(name.split('/')[1])
        data = apk.read(name)
        assert data[:4] == b'\x7fELF' and data[5] == 1, name + ': invalid ELF'
        wide = data[4] == 2
        offset = struct.unpack_from('<Q' if wide else '<I', data, 32 if wide else 28)[0]
        size, count = struct.unpack_from('<HH', data, 54 if wide else 42)
        for n in range(count):
            ph = offset + n * size
            kind = struct.unpack_from('<I', data, ph)[0]
            if kind == 0x6474e552:
                vaddr = struct.unpack_from('<Q' if wide else '<I', data, ph + (16 if wide else 8))[0]
                memsz = struct.unpack_from('<Q' if wide else '<I', data, ph + (40 if wide else 20))[0]
                assert (vaddr + memsz) % 16384 == 0, name + ': RELRO is not 16 KiB aligned'
            if kind != 1:
                continue
            align = struct.unpack_from('<Q' if wide else '<I', data, ph + (48 if wide else 28))[0]
            assert align >= 16384, name + ': load segment does not support 16 KiB pages'
assert abis == {'armeabi-v7a', 'arm64-v8a', 'x86', 'x86_64'}, abis
print('All four Android ABIs and 16 KiB ELF alignment verified.')
