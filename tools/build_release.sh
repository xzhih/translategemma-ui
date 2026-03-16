#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="$ROOT_DIR/dist"
OUTPUT_NAME="${OUTPUT_NAME:-translategemma-ui}"

cd "$ROOT_DIR"

"$ROOT_DIR/tools/sync_webui_dist.sh"

mkdir -p "$OUTPUT_DIR"

CGO_ENABLED="${CGO_ENABLED:-0}" \
go build \
  -trimpath \
  -ldflags="-s -w" \
  -o "$OUTPUT_DIR/$OUTPUT_NAME" \
  ./cmd/translategemma-ui

echo "built $OUTPUT_DIR/$OUTPUT_NAME"
