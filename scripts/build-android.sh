#!/usr/bin/env bash
set -euo pipefail
export ANDROID_HOME=${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}
: "${ANDROID_HOME:?Set ANDROID_HOME to your Android SDK}"
export ANDROID_NDK_HOME="$ANDROID_HOME/ndk/26.1.10909125"
go_bin="$(go env GOPATH)/bin"
export PATH="$go_bin:$PATH"
mobile_revision=v0.0.0-20260908204917-8b95e45f8d3e
go install "golang.org/x/mobile/cmd/gomobile@$mobile_revision"
go install "golang.org/x/mobile/cmd/gobind@$mobile_revision"
bash scripts/fetch-android-usb.sh
mkdir -p build dist
# One APK carries every ABI supported by the USB stack.
gomobile bind -ldflags="-s -w -extldflags=-Wl,-z,max-page-size=16384,-z,common-page-size=16384" -target=android/arm64,android/arm,android/amd64,android/386 -androidapi 23 \
  -javapkg io.github.andr36oid.bindings -o build/sdflasher-mobile.aar ./mobile
unzip -p build/sdflasher-mobile.aar classes.jar > build/sdflasher-mobile.jar
python3 scripts/licenses.py
python3 scripts/android-licenses.py
android/gradlew -p android :app:assembleRelease :smoke:assembleRelease :app:assembleDebug :app:assembleDebugAndroidTest :app:testDebugUnitTest :app:lintRelease
cp android/app/build/outputs/apk/release/app-release.apk "dist/andr36oid-sdflasher-${VERSION:-development}-android-universal.apk"
apk="dist/andr36oid-sdflasher-${VERSION:-development}-android-universal.apk"
"$ANDROID_HOME/build-tools/36.0.0/apksigner" verify --min-sdk-version 23 "$apk"
"$ANDROID_HOME/build-tools/36.0.0/zipalign" -c -P 16 4 "$apk"
python3 scripts/check-android-apk.py "$apk"
cp android/app/build/outputs/mapping/release/mapping.txt "dist/andr36oid-sdflasher-${VERSION:-development}-android-mapping.txt"
(cd dist && sha256sum andr36oid-sdflasher-*-android-universal.apk andr36oid-sdflasher-*-android-mapping.txt > SHA256SUMS-android)
mkdir -p build/android-test-inputs/{release,debug,androidTest/debug}
cp android/app/build/outputs/apk/release/app-release.apk build/android-test-inputs/release/
cp android/smoke/build/outputs/apk/release/smoke-release.apk build/android-test-inputs/release/test.apk
cp android/app/build/outputs/apk/debug/app-debug.apk build/android-test-inputs/debug/
cp android/app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk build/android-test-inputs/androidTest/debug/
