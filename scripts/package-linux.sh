#!/usr/bin/env bash
set -euo pipefail
version=${VERSION:-0.0.0-dev}
version=${version#v}
deb_version=${version/-/\~}
arch=$(go env GOARCH)
case "$arch" in amd64) apparch=x86_64; deploysum=c20cd71e3a4e3b80c3483cef793cda3f4e990aca14014d23c544ca3ce1270b4d;; arm64) apparch=aarch64; deploysum=620095110d693282b8ebeb244a95b5e911cf8f65f76c88b4b47d16ae6346fcff;; *) exit 1;; esac
pkg=build/deb
mkdir -p "$pkg/DEBIAN" "$pkg/usr/bin" "$pkg/usr/share/applications" "$pkg/usr/share/icons/hicolor/scalable/apps" "$pkg/usr/share/doc/andr36oid-sdflasher"
install -m755 build/andr36oid-sdflasher build/andr36oid-sdflasher-helper "$pkg/usr/bin/"
cp packaging/*.desktop "$pkg/usr/share/applications/"
cp packaging/*.svg "$pkg/usr/share/icons/hicolor/scalable/apps/"
cp -r build/licenses "$pkg/usr/share/doc/andr36oid-sdflasher/"
cat > "$pkg/DEBIAN/control" <<CONTROL
Package: andr36oid-sdflasher
Version: $deb_version
Architecture: $arch
Maintainer: andr36oid <0@kenny.cat>
Depends: libc6 (>= 2.35), libgl1, libegl1, libwayland-client0, libxkbcommon0, libx11-6, libxcursor1, libxrandr2, libxinerama1, libxi6, libxxf86vm1, policykit-1, util-linux
Section: utils
Priority: optional
Description: Native microSD installer and updater for andr36oid
CONTROL
dpkg-deb --root-owner-group --build "$pkg" "dist/andr36oid-sdflasher-${version}-linux-${arch}.deb"
appdir=build/AppDir
mkdir -p "$appdir/usr/bin" "$appdir/usr/share/licenses/andr36oid-sdflasher" "$appdir/usr/share/metainfo"
cp packaging/*.metainfo.xml "$appdir/usr/share/metainfo/"
cp build/andr36oid-sdflasher-helper "$appdir/usr/bin/"
cp -r build/licenses/. "$appdir/usr/share/licenses/andr36oid-sdflasher/"
curl --fail --location --retry 3 "https://github.com/linuxdeploy/linuxdeploy/releases/download/1-alpha-20251107-1/linuxdeploy-${apparch}.AppImage" -o build/linuxdeploy
printf '%s  %s\n' "$deploysum" build/linuxdeploy | sha256sum --check
chmod +x build/linuxdeploy
APPIMAGE_EXTRACT_AND_RUN=1 LDAI_OUTPUT="dist/andr36oid-sdflasher-${version}-linux-${arch}.AppImage" build/linuxdeploy --appdir "$appdir" --executable build/andr36oid-sdflasher --desktop-file packaging/io.github.andr36oid.sdflasher.desktop --icon-file packaging/io.github.andr36oid.sdflasher.svg --output appimage
