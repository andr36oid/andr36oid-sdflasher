#!/usr/bin/env bash
set -euo pipefail
version=${VERSION:-development}
mkdir -p build dist
ext=
if [ "$(go env GOOS)" = windows ]; then ext=.exe; fi
CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o "build/andr36oid-sdflasher-helper$ext" ./cmd/andr36oid-sdflasher-helper
flags="-s -w -X github.com/andr36oid/andr36oid-sdflasher/internal/ui.Version=$version"
if [ "$(go env GOOS)" = windows ]; then flags="$flags -H=windowsgui -extldflags=-static"; fi
CGO_ENABLED=1 go build -trimpath -ldflags "$flags" -o "build/andr36oid-sdflasher$ext" ./cmd/andr36oid-sdflasher
python3 scripts/licenses.py
