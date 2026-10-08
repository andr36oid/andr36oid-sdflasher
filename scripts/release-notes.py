#!/usr/bin/env python3
import pathlib
import sys
from urllib.parse import quote

tag = sys.argv[1]
root = pathlib.Path('dist')
base = 'https://github.com/andr36oid/andr36oid-sdflasher/releases/download/' + quote(tag, safe='') + '/'
rows = []
for system, arch, label in [
    ('windows', 'amd64', 'Windows x64'), ('windows', 'arm64', 'Windows ARM64'),
    ('linux', 'amd64', 'Linux x64'), ('linux', 'arm64', 'Linux ARM64'),
    ('macos', 'amd64', 'macOS Intel'), ('macos', 'arm64', 'macOS Apple Silicon'),
    ('android', 'universal', 'Android 6.0+ · ARM / ARM64 / x86 / x64'),
]:
    links = []
    for p in sorted(root.glob('*-' + system + '-' + arch + '*')):
        suffix = p.suffix
        kind = {'.apk': 'APK', '.deb': '.deb', '.AppImage': 'AppImage', '.flatpak': 'Flatpak', '.zip': 'Portable ZIP' if system == 'windows' else 'App ZIP'}.get(suffix)
        if kind:
            links.append('[' + kind + '](' + base + quote(p.name, safe='') + ')')
    if links:
        rows.append('| ' + label + ' | ' + ' · '.join(links) + ' |')
print('| Platform | Download |\n| --- | --- |\n' + '\n'.join(rows))
print('\n[SHA256 checksums](' + base + 'SHA256SUMS)')
print('\nAndroid uses a directly connected USB card reader. Internal SD slots, hubs, hard drives and SSDs are unsupported. Only andr36oid Treble release images can be installed.')
