# TranslateGemmaUI

TranslateGemmaUI runner with:

- Go CLI entrypoint
- CLI model management commands (`models list|download|delete`)
- Bubble Tea TUI (`--tui`)
- Local Web UI (`--webui`)
- First-run model install flow from Hugging Face
- Download and model-loading progress in both TUI and WebUI
- Streaming text translation output
- WebUI copy actions (input and output)
- WebUI image translation for runtimes that support TranslateGemma multimodal inference

## Requirements

- Go 1.24+
- Bun 1.2+
- macOS / Linux supported by the bundled runtime flow
- Optional for private Hugging Face access: `HF_TOKEN` or `HUGGINGFACE_HUB_TOKEN`

## Quick Start

### 1) Build Release Binary

```bash
./tools/build_release.sh
```

Release output:

```bash
dist/translategemma-ui
```

To build release binaries for macOS, Linux, and Windows in one pass:

```bash
./tools/build_release_all.sh
```

Cross-platform outputs:

```bash
dist/translategemma-ui-darwin-arm64
dist/translategemma-ui-linux-amd64
dist/translategemma-ui-windows-amd64.exe
```

This command:

- builds the React WebUI
- syncs the compiled assets into `internal/web/frontend/`
- embeds them into the Go binary
- produces a stripped release binary

If you want to build manually:

```bash
./tools/sync_webui_dist.sh
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/translategemma-ui ./cmd/translategemma-ui
```

The React WebUI is embedded into the Go binary from `internal/web/frontend/`.
`internal/web/frontend/` is generated at build time and should not be committed.
If you only changed Go code, `go build` is enough. If you changed files under `webui/`, run `./tools/sync_webui_dist.sh` first.

### 2) Run Web UI

```bash
./dist/translategemma-ui --webui
```

Open [http://127.0.0.1:8090](http://127.0.0.1:8090).

If no local model is installed, WebUI opens a blocking setup modal:

1. choose a quantized model
2. download starts immediately
3. loading progress is shown
4. runtime becomes ready automatically

### 3) Run TUI

```bash
./dist/translategemma-ui --tui
```

If no local model is installed, TUI enters provisioning flow with:

- model selection
- download progress bar
- runtime loading progress bar

### 4) Manage Models From CLI

```bash
./dist/translategemma-ui models list
./dist/translategemma-ui models download --id q4_k_m
./dist/translategemma-ui models delete --id q4_k_m
```

`models download` streams download progress in the terminal and updates the local app state.

## Development

### Backend

Run tests:

```bash
go test ./...
```

Build a local debug binary:

```bash
go build -o translategemma-ui ./cmd/translategemma-ui
```

Run the embedded WebUI directly from Go:

```bash
./translategemma-ui --webui --listen 127.0.0.1:8090
```

### React WebUI

Run frontend tests:

```bash
cd webui
bun run test
```

Run the Vite development server:

```bash
cd webui
bun run dev -- --host 127.0.0.1 --port 5173
```

The Vite dev server proxies `/api/*` and `/healthz` to `http://127.0.0.1:8090`.
During frontend development you usually run:

1. Go backend on `127.0.0.1:8090`
2. Vite dev server on `127.0.0.1:5173`

### WebUI Localization

The React app now uses `i18next` / `react-i18next` with locale resources under:

- `webui/src/locales/en/translation.json`
- `webui/src/locales/zh-CN/translation.json`

Locale resolution order:

1. `?lang=en` or `?lang=zh-CN`
2. `localStorage["tg_ui_locale"]`
3. browser language
4. fallback to `en`

There is also a compact locale toggle in the WebUI header for switching between `en` and `zh-CN`.

If you want to use your own TranslateGemma runtime to draft a new locale file, start the local runtime first and run:

```bash
python3 tools/translate_locale.py \
  --source webui/src/locales/en/translation.json \
  --target webui/src/locales/zh-CN/translation.json \
  --target-lang zh-CN
```

The script uses the local `/v1/translate` endpoint and preserves existing translated entries unless you pass `--overwrite`.

### Sync Embedded Frontend Assets

If you changed anything under `webui/`, refresh the embedded production assets before committing or building a release:

```bash
./tools/sync_webui_dist.sh
```

This copies `webui/dist/` into `internal/web/frontend/`, which is what the Go binary embeds.

## Model Source

Default runtime source:

- Hugging Face repo: [xzhih/translategemma-4b-it-llamafile](https://huggingface.co/xzhih/translategemma-4b-it-llamafile)
- Manifest URL: `https://huggingface.co/xzhih/translategemma-4b-it-llamafile/resolve/main/manifest-v1.json`

Discovery behavior:

- by default the app reads the remote `manifest-v1.json`
- if `TRANSLATEGEMMA_UI_MANIFEST_PATH` is set, the app uses that local manifest instead
- if the Hugging Face repo requires authentication, set `HF_TOKEN` or `HUGGINGFACE_HUB_TOKEN`

Useful local development override:

```bash
export TRANSLATEGEMMA_UI_MANIFEST_PATH=/absolute/path/to/translategemma-runtime-publisher/dist/translategemma-4b-it/manifest-v1.json
```

## Model Selection

Current runtime matrix:

- `q4_k_m` (text)
- `q6_k` (text)
- `q8_0` (text)
- `q8_0_vision` (text + image)

Default recommendation:

- text translation: `q4_k_m`
- image translation: `q8_0_vision`

Current packaged runtime files come from the runtime publisher output and map to:

- `translategemma-4b-it.Q4_K_M.llamafile`
- `translategemma-4b-it.Q6_K.llamafile`
- `translategemma-4b-it.Q8_0.llamafile`
- `translategemma-4b-it.Q8_0.mmproj-Q8_0.llamafile`

## Usage Notes

### Web UI

- `Translate` streams output token-by-token.
- `Copy Input` copies source text.
- `Copy Output` copies translated result.
- `↔` swaps source and target languages.
- Text and file tabs both expose an optional `translation_instruction` field.
- `File Translation` is separated into its own top tab and currently supports JPEG/PNG/GIF uploads (up to 10MB).
- File translation uses the TranslateGemma-specific `/v1/translate` runtime API and requires `q8_0_vision`.
- WebUI can switch the active runtime to `q8_0_vision` directly from the file tab.
- `History` is scrollable and opens a detail modal with copy actions.
- `Model Management` supports local download, activate, and delete flows.

### TUI

- `Enter`: continue / confirm / translate
- `j` / `k` (or arrows): move model cursor
- `Tab`: switch between source text and optional instruction field
- `Ctrl+S`: swap source/target language
- `Esc`: back to model selection
- `q`: quit

## Data Directory

Default directory:

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

The application persists active model/runtime selection in `config.json` and `state.json`.
WebUI history is stored in `history.json` with a 200-entry cap.
Downloaded runtimes are stored as packaged `.llamafile` files under `runtimes/`.

## Runtime Behavior

- backend endpoint: `http://127.0.0.1:8080`
- if backend is already reachable, it is reused
- if backend is offline and local model exists, the app auto-starts runtime
- runtime process is launched with `--server`
- text and image translation use the TranslateGemma-specific `/v1/translate` route
- React WebUI in release builds is served from embedded static assets inside the Go binary

## Troubleshooting

- runtime logs: `<user-home>/.translategemma-ui/logs/runtime.log`
- if startup fails, check:
  - local `.llamafile` exists in `runtimes/`
  - local port `8080` is not occupied
  - if the model list is empty in development, check `TRANSLATEGEMMA_UI_MANIFEST_PATH`
  - if downloads fail with `401/403`, check `HF_TOKEN` or `HUGGINGFACE_HUB_TOKEN`

## Docs

- Single source of truth: `docs/FEATURE_REQUIREMENTS_CN.md`
- Embedded React WebUI source (Bun/Vite): `webui/README.md`
