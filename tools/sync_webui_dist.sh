#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEBUI_DIR="$ROOT_DIR/webui"
TARGET_DIR="$ROOT_DIR/internal/web/frontend"

if ! command -v bun >/dev/null 2>&1; then
  echo "bun is required to build the embedded web UI" >&2
  exit 1
fi

cd "$WEBUI_DIR"
if [[ ! -d node_modules ]]; then
  bun install --frozen-lockfile
fi
bun run build

rm -rf "$TARGET_DIR"
mkdir -p "$TARGET_DIR"
cp -R dist/. "$TARGET_DIR/"
