#!/usr/bin/env bash
set -euo pipefail
trap 'adb logcat -d -v threadtime > android-logcat.txt' EXIT
adb logcat -c
adb install apks/release/app-release.apk
adb install apks/androidTest/release/app-release-androidTest.apk
adb shell am instrument -w io.github.andr36oid.sdflasher.test/androidx.test.runner.AndroidJUnitRunner | tee instrumentation.txt
grep -Fq 'OK (4 tests)' instrumentation.txt
adb install apks/debug/app-debug.apk
adb install apks/androidTest/debug/app-debug-androidTest.apk
adb shell am instrument -w io.github.andr36oid.sdflasher.debug.test/androidx.test.runner.AndroidJUnitRunner | tee instrumentation-debug.txt
grep -Fq 'OK (4 tests)' instrumentation-debug.txt
