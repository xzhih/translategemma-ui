# TranslateGemmaUI

TranslateGemmaUI is a local TranslateGemma app with:

- a Go CLI entrypoint
- Bubble Tea TUI mode
- an embedded Web UI served from the same binary
- local model download and activation flows
- streaming text translation output
- image translation for multimodal runtimes

The app downloads packaged TranslateGemma runtimes from Hugging Face on demand and stores them under the user data directory. End users do not need Go or Bun if they install from GitHub Releases or Homebrew.

## Platform Support

Release binaries are published for:

- macOS `amd64`
- macOS `arm64`
- Linux `amd64`
- Linux `arm64`
- Windows `amd64`

Homebrew installation is supported on macOS and Linux only.
Windows users should install from the GitHub Releases page.

## Install

### Option 1: GitHub Release

Download the latest archive from [GitHub Releases](https://github.com/xzhih/translategemma-ui/releases).

Expected artifacts:

- `translategemma-ui_<version>_darwin_amd64.tar.gz`
- `translategemma-ui_<version>_darwin_arm64.tar.gz`
- `translategemma-ui_<version>_linux_amd64.tar.gz`
- `translategemma-ui_<version>_linux_arm64.tar.gz`
- `translategemma-ui_<version>_windows_amd64.zip`

### Option 2: Homebrew

Tap this repository and install the formula:

```bash
brew tap xzhih/translategemma-ui https://github.com/xzhih/translategemma-ui
brew install xzhih/translategemma-ui/translategemma-ui
```

## Quick Start

### Run the Web UI

```bash
translategemma-ui --webui
```

Open [http://127.0.0.1:8090](http://127.0.0.1:8090).

### Run the TUI

```bash
translategemma-ui --tui
```

### Check the version

```bash
translategemma-ui --version
```

### Manage runtimes from the CLI

```bash
translategemma-ui models list
translategemma-ui models download --id q4_k_m
translategemma-ui models delete --id q4_k_m
```

### Translate text from the CLI

```bash
translategemma-ui translate text \
  --text "Hello world" \
  --source-lang en \
  --target-lang zh-CN
```

### Translate an image from the CLI

```bash
translategemma-ui translate image \
  --file /path/to/image.png \
  --model-id q8_0_vision
```

## Runtime Model Source

Default runtime source:

- Hugging Face repo: [xzhih/translategemma-4b-it-llamafile](https://huggingface.co/xzhih/translategemma-4b-it-llamafile)
- Manifest URL: `https://huggingface.co/xzhih/translategemma-4b-it-llamafile/resolve/main/manifest-v1.json`

Current runtime matrix:

- `q4_k_m` for text translation
- `q6_k` for text translation
- `q8_0` for text translation
- `q8_0_vision` for text and image translation

## Data Directory

Default data directory:

- macOS / Linux: `$HOME/.translategemma-ui`
- Windows: `%USERPROFILE%\\.translategemma-ui`

Created structure:

```text
<user-home>/.translategemma-ui/
  config.json
  history.json
  state.json
  logs/
  runtimes/
  tmp/
```

Downloaded runtimes are stored under `runtimes/`.
On Windows, packaged `.llamafile` runtimes are saved locally with an `.exe` suffix for direct execution compatibility.

## Development

Development requirements:

- Go 1.25+
- Bun 1.2+

Install frontend dependencies:

```bash
cd webui
bun install
```

Run backend tests:

```bash
go test ./...
```

Run frontend tests:

```bash
cd webui
bun run test
```

Run the Vite dev server:

```bash
cd webui
bun run dev -- --host 127.0.0.1 --port 5173
```

Run the backend in another terminal:

```bash
go run ./cmd/translategemma-ui --webui
```

The Vite dev server proxies `/api/*` and `/healthz` to `http://127.0.0.1:8090`, so the local backend needs to be running while you work on the frontend.

Build a local release binary:

```bash
./tools/build_release.sh
```

Build local cross-platform binaries:

```bash
./tools/build_release_all.sh
```

Outputs:

- `dist/translategemma-ui-darwin-amd64`
- `dist/translategemma-ui-darwin-arm64`
- `dist/translategemma-ui-linux-amd64`
- `dist/translategemma-ui-linux-arm64`
- `dist/translategemma-ui-windows-amd64.exe`

The Web UI is embedded into the Go binary from `internal/web/frontend/`.
If you changed files under `webui/`, run `./tools/sync_webui_dist.sh` before a manual `go build`.

## Release Process

Tagged releases use GoReleaser through `.github/workflows/release.yml`.

Release flow:

1. push `main`
2. create and push a tag like `v0.1.0`
3. GitHub Actions runs tests, builds release archives, uploads release assets, and updates `Formula/translategemma-ui.rb`

## Troubleshooting

- Runtime logs: `<user-home>/.translategemma-ui/logs/runtime.log`
- If local runtime startup fails, make sure the local backend port is free and at least one packaged runtime is installed
