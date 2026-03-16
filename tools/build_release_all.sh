#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="$ROOT_DIR/dist"

cd "$ROOT_DIR"

source "$ROOT_DIR/tools/lib_release.sh"

"$ROOT_DIR/tools/sync_webui_dist.sh"

mkdir -p "$OUTPUT_DIR"

build_target() {
  local goos="$1"
  local goarch="$2"
  local output="$3"

  echo "building $output"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    go build \
    -trimpath \
    -ldflags="$(release_ldflags)" \
    -o "$OUTPUT_DIR/$output" \
    ./cmd/translategemma-ui
}

build_target darwin amd64 translategemma-ui-darwin-amd64
build_target darwin arm64 translategemma-ui-darwin-arm64
build_target linux amd64 translategemma-ui-linux-amd64
build_target linux arm64 translategemma-ui-linux-arm64
build_target windows amd64 translategemma-ui-windows-amd64.exe

echo "built release artifacts in $OUTPUT_DIR"
