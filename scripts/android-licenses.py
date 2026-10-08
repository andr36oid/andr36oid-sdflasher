#!/usr/bin/env python3
import pathlib
import shutil

out = pathlib.Path('build/android-assets/licenses')
out.mkdir(parents=True, exist_ok=True)
for source in pathlib.Path('build/licenses').iterdir():
    if source.is_file():
        shutil.copyfile(source, out / source.name)
for source, name in [
    ('build/usb-source/LICENSE', 'libaums-Apache-2.0.txt'),
    ('build/usb-source/vendor/libusb/COPYING', 'libusb-LGPL-2.1.txt'),
]:
    shutil.copyfile(source, out / name)
(out / 'Android-NOTICE.txt').write_text('''andr36oid SD Flasher — GPLv3
USB libraries:
  libaums, copyright 2014–2023 Magnus Jahnen and contributors, Apache 2.0
  https://github.com/magnusja/libaums
  Source revision: cef48e29a57974d4e5b09369a14df55d2fc0f9b3
  JNI release-mode arguments are corrected from NULL to integer zero for NDK 26.
  libusb, copyright libusb contributors, LGPL 2.1 or later
  https://github.com/libusb/libusb
  Source revision: 87a55632db62c9bdc58cd31d3ccfa673f1bb017f
  libusb is a separate shared library. Its source is fetched by scripts/fetch-android-usb.sh.
  Rebuild and replace it with the Android project's CMake build.
AndroidX Core and annotation libraries, copyright The Android Open Source Project, Apache 2.0
Kotlin standard library, copyright JetBrains s.r.o. and Kotlin contributors, Apache 2.0
Full source and build files: https://github.com/andr36oid/andr36oid-sdflasher
''', encoding='utf-8')
