#!/usr/bin/env bash
set -euo pipefail
trap 'adb logcat -d -v threadtime > android-logcat.txt' EXIT
adb logcat -c
api=$(adb shell getprop ro.build.version.sdk | tr -d '\r')
if (( api >= 29 )) && [[ -n ${ANDROID_PAGE_SIZE:-} ]]; then
    actual_page_size=$(adb shell getconf PAGESIZE | tr -d '\r')
    echo "Android page size: $actual_page_size (expected $ANDROID_PAGE_SIZE)"
    test "$actual_page_size" = "$ANDROID_PAGE_SIZE"
fi
adb shell setprop debug.checkjni 1
if (( api >= 29 )); then adb shell cmd uimode night no; fi
adb install apks/release/app-release.apk
adb install apks/release/test.apk
timeout 180s adb shell am instrument -w -e class io.github.andr36oid.sdflasher.ReleaseTest io.github.andr36oid.sdflasher.test/androidx.test.runner.AndroidJUnitRunner | tee instrumentation.txt
grep -Eq 'OK \([1-9][0-9]* tests\)' instrumentation.txt
if (( api >= 29 )); then
    adb shell cmd uimode night yes
    timeout 180s adb shell am instrument -w -e class io.github.andr36oid.sdflasher.ReleaseTest io.github.andr36oid.sdflasher.test/androidx.test.runner.AndroidJUnitRunner | tee instrumentation-dark.txt
    grep -Eq 'OK \([1-9][0-9]* tests\)' instrumentation-dark.txt
    adb shell cmd uimode night no
fi
adb install apks/debug/app-debug.apk
adb install apks/androidTest/debug/app-debug-androidTest.apk
timeout 180s adb shell am instrument -w -e class io.github.andr36oid.sdflasher.AppTest io.github.andr36oid.sdflasher.debug.test/androidx.test.runner.AndroidJUnitRunner | tee instrumentation-debug.txt
grep -Eq 'OK \([1-9][0-9]* tests\)' instrumentation-debug.txt
