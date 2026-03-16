package app

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"translategemma-ui/internal/config"
	"translategemma-ui/internal/huggingface"
	"translategemma-ui/internal/models"
	"translategemma-ui/internal/modelstore"
	"translategemma-ui/internal/runtime"
	lf "translategemma-ui/internal/runtime/llamafile"
	"translategemma-ui/internal/runtimeutil"
	"translategemma-ui/internal/translate"
)

var (
	prepareTranslationRuntimeFn = prepareTranslationRuntime
	printTranslateResultFn      = printTranslateResult
)

func runTranslateCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: translategemma-ui translate [text|image] [flags]")
	}

	switch args[0] {
	case "text":
		return runTranslateText(args[1:])
	case "image":
		return runTranslateImage(args[1:])
	default:
		return fmt.Errorf("unknown translate subcommand %q", args[0])
	}
}

func runTranslateText(args []string) (err error) {
	fs := flag.NewFlagSet("translate text", flag.ContinueOnError)
	var (
		text        string
		sourceLang  string
		targetLang  string
		instruction string
		modelID     string
		asJSON      bool
	)
	fs.StringVar(&text, "text", "", "Text to translate")
	fs.StringVar(&sourceLang, "source-lang", "auto", "Source language code")
	fs.StringVar(&targetLang, "target-lang", "zh-CN", "Target language code")
	fs.StringVar(&instruction, "instruction", "", "Optional translation instruction")
	fs.StringVar(&modelID, "model-id", "", "Installed runtime ID to use")
	fs.BoolVar(&asJSON, "json", false, "Print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("missing --text")
	}

	root, service, _, cleanup, err := prepareTranslationRuntimeFn(modelID, false)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, cleanup())
	}()

	output, err := service.Translate(translate.Request{
		SourceLang:             sourceLang,
		TargetLang:             targetLang,
		Text:                   text,
		TranslationInstruction: instruction,
	})
	if err != nil {
		return err
	}
	return printTranslateResultFn(root, selectedModelID(root), output, asJSON)
}

func runTranslateImage(args []string) (err error) {
	fs := flag.NewFlagSet("translate image", flag.ContinueOnError)
	var (
		filePath    string
		sourceLang  string
		targetLang  string
		instruction string
		modelID     string
		asJSON      bool
	)
	fs.StringVar(&filePath, "file", "", "Image file to translate")
	fs.StringVar(&sourceLang, "source-lang", "auto", "Source language code")
	fs.StringVar(&targetLang, "target-lang", "zh-CN", "Target language code")
	fs.StringVar(&instruction, "instruction", "", "Optional translation instruction")
	fs.StringVar(&modelID, "model-id", "", "Installed vision runtime ID to use")
	fs.BoolVar(&asJSON, "json", false, "Print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(filePath) == "" {
		return errors.New("missing --file")
	}

	content, mimeType, err := readImageInput(filePath)
	if err != nil {
		return err
	}

	root, service, _, cleanup, err := prepareTranslationRuntimeFn(modelID, true)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, cleanup())
	}()

	output, err := service.TranslateImage(translate.ImageRequest{
		SourceLang:             sourceLang,
		TargetLang:             targetLang,
		ImageMime:              mimeType,
		ImageBytes:             content,
		TranslationInstruction: instruction,
	})
	if err != nil {
		return err
	}
	return printTranslateResultFn(root, selectedModelID(root), output, asJSON)
}

func prepareTranslationRuntime(requestedModelID string, requireVision bool) (string, *translate.Service, models.QuantizedModel, func() error, error) {
	root, err := config.EnsureDataDirs("")
	if err != nil {
		return "", nil, models.QuantizedModel{}, nil, fmt.Errorf("initialize data dir: %w", err)
	}
	cfg, _ := config.LoadAppConfig(root)
	state, _ := config.LoadAppState(root)
	available := huggingface.ListTranslateGemmaModels()

	item, modelPath, err := resolveInstalledModel(root, available, cfg, state, requestedModelID, requireVision)
	if err != nil {
		return "", nil, models.QuantizedModel{}, nil, err
	}

	backendURL := runtimeutil.SyncBackendURL(&state, state.BackendURL)
	manager := lf.NewManager(root, backendURL)
	manager.SetPreferredModelPath(modelPath)
	cleanup := manager.StopOwned
	previousActiveModelPath := state.ActiveModelPath

	runtimeutil.ApplyActiveModel(&cfg, &state, item, modelPath)
	if err := config.SaveAppConfig(root, cfg); err != nil {
		return "", nil, models.QuantizedModel{}, nil, err
	}
	if err := config.SaveAppState(root, state); err != nil {
		return "", nil, models.QuantizedModel{}, nil, err
	}

	probe := runtime.ProbeBackend(backendURL)
	if !runtimeutil.CanReuseLoadedRuntime(modelPath, previousActiveModelPath, probe.Ready) {
		_ = manager.Stop()
		if _, err := manager.EnsureRunningWithProgress(nil); err != nil {
			return "", nil, models.QuantizedModel{}, nil, err
		}
		backendURL = runtimeutil.SyncBackendURL(&state, manager.CurrentBackendURL(), manager)
		if err := config.SaveAppState(root, state); err != nil {
			return "", nil, models.QuantizedModel{}, nil, err
		}
	}

	return root, translate.NewService(backendURL), item, cleanup, nil
}

func resolveInstalledModel(dataRoot string, available []models.QuantizedModel, cfg config.AppConfig, state config.AppState, requestedModelID string, requireVision bool) (models.QuantizedModel, string, error) {
	if requestedModelID != "" {
		item, ok := models.FindByID(available, requestedModelID)
		if !ok {
			return models.QuantizedModel{}, "", fmt.Errorf("unknown model id %q", requestedModelID)
		}
		if requireVision && !models.SupportsVision(item) {
			return models.QuantizedModel{}, "", fmt.Errorf("model %q does not support image translation", requestedModelID)
		}
		path := modelstore.LocalModelPath(dataRoot, item.FileName)
		if path == "" {
			return models.QuantizedModel{}, "", fmt.Errorf("model %q is not installed locally; run `translategemma-ui models download --id %s` first", requestedModelID, requestedModelID)
		}
		return item, path, nil
	}

	catalog := modelstore.Catalog(dataRoot, available, strings.TrimSpace(cfg.ActiveModelID), strings.TrimSpace(state.ActiveModelPath))
	if _, item, ok := modelstore.ResolveCatalogItem(catalog, state.ActiveModelPath, modelstore.ResolveOptions{
		RequireVision:     requireVision,
		PreferTextRuntime: !requireVision,
	}, cfg.ActiveModelID); ok {
		return item.QuantizedModel, item.Path, nil
	}

	if state.ActiveModelPath != "" && runtimeutil.RuntimeModeForPath(state.ActiveModelPath) != "" && fileExists(state.ActiveModelPath) {
		if !requireVision || strings.Contains(strings.ToLower(filepath.Base(state.ActiveModelPath)), ".mmproj-") {
			return models.QuantizedModel{ID: strings.TrimSpace(cfg.ActiveModelID), FileName: filepath.Base(state.ActiveModelPath)}, state.ActiveModelPath, nil
		}
	}

	modeHint := "text"
	if requireVision {
		modeHint = "image"
	}
	return models.QuantizedModel{}, "", fmt.Errorf("no local %s runtime installed; use TUI/WebUI or `translategemma-ui models download --id ...` first", modeHint)
}

func readImageInput(path string) ([]byte, string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	if len(content) == 0 {
		return nil, "", errors.New("image file is empty")
	}
	mimeType := http.DetectContentType(content)
	if !strings.HasPrefix(mimeType, "image/") {
		return nil, "", fmt.Errorf("unsupported image MIME type: %s", mimeType)
	}
	return content, mimeType, nil
}

func printTranslateResult(root, modelID, output string, asJSON bool) error {
	if !asJSON {
		fmt.Println(output)
		return nil
	}
	payload := map[string]any{
		"model_id": modelID,
		"output":   output,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func selectedModelID(root string) string {
	cfg, err := config.LoadAppConfig(root)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.ActiveModelID)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
