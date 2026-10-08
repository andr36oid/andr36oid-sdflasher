#!/usr/bin/env bash
set -euo pipefail
version=${VERSION:-0.0.0-dev}
bundle_version=${version#v}
bundle_version=${bundle_version%%-*}
arch=$(go env GOARCH)
bundle='build/andr36oid SD Flasher.app'
mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources"
cp build/andr36oid-sdflasher build/andr36oid-sdflasher-helper "$bundle/Contents/MacOS/"
cp -r build/licenses "$bundle/Contents/Resources/"
cat > "$bundle/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>andr36oid-sdflasher</string>
<key>CFBundleIdentifier</key><string>io.github.andr36oid.sdflasher</string>
<key>CFBundleName</key><string>andr36oid SD Flasher</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>$bundle_version</string>
<key>CFBundleVersion</key><string>$bundle_version</string>
<key>NSHighResolutionCapable</key><true/>
<key>LSMinimumSystemVersion</key><string>12.0</string>
<key>NSRemovableVolumesUsageDescription</key><string>Install and update your andr36oid microSD card.</string>
</dict></plist>
PLIST
codesign --force --sign - "$bundle/Contents/MacOS/andr36oid-sdflasher-helper"
codesign --force --deep --sign - "$bundle"
ditto -c -k --sequesterRsrc --keepParent "$bundle" "dist/andr36oid-sdflasher-${version}-macos-${arch}.zip"
