#!/usr/bin/env bash
set -euo pipefail
usb_revision=cef48e29a57974d4e5b09369a14df55d2fc0f9b3
usb_dir=build/usb-source
if [ ! -d "$usb_dir/.git" ]; then
  git clone --no-checkout https://github.com/magnusja/libaums.git "$usb_dir"
fi
git -C "$usb_dir" fetch --depth 1 origin "$usb_revision"
git -C "$usb_dir" checkout --detach "$usb_revision"
git -C "$usb_dir" submodule update --init --depth 1 vendor/libusb

# JNI release modes are integers. Older libaums used NULL, rejected by NDK 26.
python3 - <<'PYTHON'
from pathlib import Path
p = Path('build/usb-source/libusbcommunication/src/c/usb.c')
s = p.read_text().replace('body, NULL)', 'body, 0)').replace('c_data, NULL)', 'c_data, 0)')
p.write_text(s)
PYTHON
