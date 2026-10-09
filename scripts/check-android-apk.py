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
        loads, relros = [], []
        for n in range(count):
            ph = offset + n * size
            kind = struct.unpack_from('<I', data, ph)[0]
            vaddr = struct.unpack_from('<Q' if wide else '<I', data, ph + (16 if wide else 8))[0]
            memsz = struct.unpack_from('<Q' if wide else '<I', data, ph + (40 if wide else 20))[0]
            if kind == 0x6474e552:
                relros.append((vaddr, vaddr + memsz))
                assert (vaddr + memsz) % 16384 == 0, name + ': RELRO is not 16 KiB aligned'
            if kind != 1:
                continue
            align = struct.unpack_from('<Q' if wide else '<I', data, ph + (48 if wide else 28))[0]
            assert align >= 16384, name + ': load segment does not support 16 KiB pages'
            loads.append((vaddr, vaddr + memsz))
        assert loads and relros, name + ': missing load segments or RELRO protection'
        # Old loaders map only PT_LOAD. RELRO padding outside those mappings can
        # fail with ENOMEM or change the protection of an adjacent allocation.
        for page in (4096, 16384):
            mapped = sorted((start // page * page, (end + page - 1) // page * page)
                            for start, end in loads)
            for start, end in relros:
                cursor = start // page * page
                limit = (end + page - 1) // page * page
                for lower, upper in mapped:
                    if lower <= cursor < upper:
                        cursor = upper
                assert cursor >= limit, name + ': RELRO extends outside load mappings on ' + str(page) + '-byte pages'
assert abis == {'armeabi-v7a', 'arm64-v8a', 'x86', 'x86_64'}, abis
print('All four Android ABIs, 16 KiB alignment, and RELRO mappings verified.')
