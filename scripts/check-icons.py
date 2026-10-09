#!/usr/bin/env python3
import pathlib
import configparser
import plistlib
import re
import struct
import subprocess
import sys
import zipfile
import xml.etree.ElementTree as ET


def windows(path):
    data = path.read_bytes()
    pe = struct.unpack_from('<I', data, 0x3c)[0]
    assert data[pe:pe + 4] == b'PE\0\0', 'not a Windows executable'
    count = struct.unpack_from('<H', data, pe + 6)[0]
    optional = pe + 24
    optional_size = struct.unpack_from('<H', data, pe + 20)[0]
    magic = struct.unpack_from('<H', data, optional)[0]
    directories = optional + (112 if magic == 0x20b else 96)
    resource_rva = struct.unpack_from('<I', data, directories + 16)[0]
    assert resource_rva, 'missing PE resources'

    def offset(rva):
        for n in range(count):
            section = optional + optional_size + n * 40
            size, address, raw_size, raw = struct.unpack_from('<IIII', data, section + 8)
            if address <= rva < address + max(size, raw_size):
                return raw + rva - address
        raise AssertionError('resource address outside PE sections')

    base = offset(resource_rva)

    def children(entry):
        location = base + (entry & 0x7fffffff)
        named, ids = struct.unpack_from('<HH', data, location + 12)
        return dict(struct.unpack_from('<II', data, location + 16 + n * 8)
                    for n in range(named + ids))

    def leaves(entry):
        if entry & 0x80000000:
            return [blob for child in children(entry).values() for blob in leaves(child)]
        rva, size = struct.unpack_from('<II', data, base + entry)
        return [data[offset(rva):offset(rva) + size]]

    types = children(0)
    assert 3 in types and 14 in types, 'missing icon images or icon group'
    icons = {key: leaves(value)[0] for key, value in children(types[3]).items()}
    sizes = set()
    for group in leaves(types[14]):
        reserved, kind, count = struct.unpack_from('<HHH', group)
        assert reserved == 0 and kind == 1 and count > 0, 'invalid icon group'
        assert len(group) == 6 + count * 14, 'truncated icon group'
        for n in range(count):
            start = 6 + n * 14
            size, identifier = struct.unpack_from('<IH', group, start + 8)
            blob = icons[identifier]
            assert len(blob) == size, 'icon image size mismatch'
            assert blob.startswith(b'\x89PNG\r\n\x1a\n') or blob[:4] == b'\x28\0\0\0', 'invalid icon image'
            sizes.add(group[start] or 256)
    assert {16, 32, 48, 256} <= sizes, 'missing small or high-resolution icons'


def macos(path):
    with (path / 'Contents/Info.plist').open('rb') as source:
        info = plistlib.load(source)
    name = info['CFBundleIconFile']
    data = (path / 'Contents/Resources' / name).read_bytes()
    assert data[:4] == b'icns' and struct.unpack_from('>I', data, 4)[0] == len(data), 'invalid ICNS file'
    sizes = {}
    position = 8
    while position < len(data):
        kind, size = struct.unpack_from('>4sI', data, position)
        assert size >= 32 and position + size <= len(data), 'invalid ICNS entry'
        png = data[position + 8:position + size]
        assert png.startswith(b'\x89PNG\r\n\x1a\n'), 'invalid ICNS image'
        sizes[kind] = struct.unpack_from('>II', png, 16)
        position += size
    for kind, size in [(b'icp4', 16), (b'ic07', 128), (b'ic08', 256), (b'ic09', 512), (b'ic10', 1024)]:
        assert sizes.get(kind) == (size, size), 'missing standard or Retina icon size'


def android(path):
    badging = subprocess.check_output([sys.argv[3], 'dump', 'badging', str(path)], text=True)
    icons = re.findall(r"^application-icon-\d+:'([^']+)'", badging, re.MULTILINE)
    assert icons, 'missing Android launcher icon'
    with zipfile.ZipFile(path) as apk:
        for icon in icons:
            assert apk.read(icon), 'empty Android launcher icon'


def linux(path):
    desktop = configparser.ConfigParser(interpolation=None)
    desktop.read(path / 'share/applications/io.github.andr36oid.sdflasher.desktop')
    name = desktop['Desktop Entry']['Icon']
    icon = path / 'share/icons/hicolor/scalable/apps' / (name + '.svg')
    assert ET.parse(icon).getroot().tag == '{http://www.w3.org/2000/svg}svg', 'invalid launcher icon'


def appimage(path):
    linux(path / 'usr')
    assert (path / '.DirIcon').read_bytes(), 'missing AppImage icon'


mode = sys.argv[1]
paths = sys.argv[2:3] if mode == 'android' else sys.argv[2:]
checks = {'windows': windows, 'macos': macos, 'android': android, 'linux': linux, 'appimage': appimage}
for filename in paths:
    checks[mode](pathlib.Path(filename))
    print(filename + ': application icon verified')
