#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="$ROOT_DIR/dist"
OUTPUT_NAME="${OUTPUT_NAME:-translategemma-ui}"

cd "$ROOT_DIR"

source "$ROOT_DIR/tools/lib_release.sh"

"$ROOT_DIR/tools/sync_webui_dist.sh"

mkdir -p "$OUTPUT_DIR"

target_goos="${GOOS:-$(go env GOOS)}"
if [[ "$target_goos" == "windows" && "$OUTPUT_NAME" != *.exe ]]; then
  OUTPUT_NAME="${OUTPUT_NAME}.exe"
fi

CGO_ENABLED="${CGO_ENABLED:-0}" \
go build \
  -trimpath \
  -ldflags="$(release_ldflags)" \
  -o "$OUTPUT_DIR/$OUTPUT_NAME" \
  ./cmd/translategemma-ui

echo "built $OUTPUT_DIR/$OUTPUT_NAME"
