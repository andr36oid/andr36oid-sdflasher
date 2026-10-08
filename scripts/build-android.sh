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
  -javapkg io.github.andr36oid -o build/sdflasher-mobile.aar ./mobile
python3 scripts/licenses.py
python3 scripts/android-licenses.py
variant=Debug
suffix=debug
if [[ -n "${ANDROID_KEYSTORE:-}" ]]; then variant=Release; suffix=release; fi
android/gradlew -p android ":app:assemble$variant" :app:assembleDebugAndroidTest :app:testDebugUnitTest :app:lintDebug
cp "android/app/build/outputs/apk/${suffix}/app-${suffix}.apk" "dist/andr36oid-sdflasher-${VERSION:-development}-android-universal.apk"
apk="dist/andr36oid-sdflasher-${VERSION:-development}-android-universal.apk"
"$ANDROID_HOME/build-tools/36.0.0/apksigner" verify --min-sdk-version 23 "$apk"
"$ANDROID_HOME/build-tools/36.0.0/zipalign" -c -P 16 4 "$apk"
python3 scripts/check-android-apk.py "$apk"
(cd dist && sha256sum andr36oid-sdflasher-*-android-universal.apk > SHA256SUMS-android)
