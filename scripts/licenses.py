#!/usr/bin/env python3
import json
import pathlib
import shutil
import subprocess

out = pathlib.Path('build/licenses')
out.mkdir(parents=True, exist_ok=True)
shutil.copyfile('LICENSE', out / 'andr36oid-GPL-3.0.txt')
raw = subprocess.check_output(['go', 'list', '-m', '-json', 'all'], text=True)
decoder = json.JSONDecoder()
while raw.strip():
    module, end = decoder.raw_decode(raw.lstrip())
    raw = raw.lstrip()[end:]
    if 'Dir' not in module or module.get('Main'):
        continue
    root = pathlib.Path(module['Dir'])
    for p in root.iterdir():
        if p.is_file() and p.name.upper().startswith(('LICENSE', 'COPYING', 'NOTICE')):
            name = module['Path'].replace('/', '_') + '-' + p.name
            shutil.copyfile(p, out / name)
