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
	"translategemma-ui/internal/translate"
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

func runTranslateText(args []string) error {
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

	root, service, _, err := prepareTranslationRuntime(modelID, false)
	if err != nil {
		return err
	}

	output, err := service.Translate(translate.Request{
		SourceLang:             sourceLang,
		TargetLang:             targetLang,
		Text:                   text,
		TranslationInstruction: instruction,
	})
	if err != nil {
		return err
	}
	return printTranslateResult(root, selectedModelID(root), output, asJSON)
}

func runTranslateImage(args []string) error {
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

	root, service, _, err := prepareTranslationRuntime(modelID, true)
	if err != nil {
		return err
	}

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
	return printTranslateResult(root, selectedModelID(root), output, asJSON)
}

func prepareTranslationRuntime(requestedModelID string, requireVision bool) (string, *translate.Service, models.QuantizedModel, error) {
	root, err := config.EnsureDataDirs("")
	if err != nil {
		return "", nil, models.QuantizedModel{}, fmt.Errorf("initialize data dir: %w", err)
	}
	cfg, _ := config.LoadAppConfig(root)
	state, _ := config.LoadAppState(root)
	available := huggingface.ListTranslateGemmaModels()

	item, modelPath, err := resolveInstalledModel(root, available, cfg, state, requestedModelID, requireVision)
	if err != nil {
		return "", nil, models.QuantizedModel{}, err
	}

	backendURL := runtime.NormalizeBackendURL(state.BackendURL)
	if strings.TrimSpace(state.BackendURL) == "" {
		backendURL = runtime.NormalizeBackendURL(runtime.DefaultBackendURL)
	}
	manager := lf.NewManager(root, backendURL)
	manager.SetPreferredModelPath(modelPath)

	state.ActiveModelPath = modelPath
	state.RuntimeMode = runtimeModeForPath(modelPath)
	cfg.ActiveModelID = item.ID
	if err := config.SaveAppConfig(root, cfg); err != nil {
		return "", nil, models.QuantizedModel{}, err
	}
	if err := config.SaveAppState(root, state); err != nil {
		return "", nil, models.QuantizedModel{}, err
	}

	_ = manager.Stop()
	if _, err := manager.EnsureRunningWithProgress(nil); err != nil {
		return "", nil, models.QuantizedModel{}, err
	}
	backendURL = manager.CurrentBackendURL()
	state.BackendURL = backendURL
	if err := config.SaveAppState(root, state); err != nil {
		return "", nil, models.QuantizedModel{}, err
	}

	return root, translate.NewService(backendURL), item, nil
}

func resolveInstalledModel(dataRoot string, available []models.QuantizedModel, cfg config.AppConfig, state config.AppState, requestedModelID string, requireVision bool) (models.QuantizedModel, string, error) {
	if requestedModelID != "" {
		item, ok := findModelByID(available, requestedModelID)
		if !ok {
			return models.QuantizedModel{}, "", fmt.Errorf("unknown model id %q", requestedModelID)
		}
		if requireVision && !supportsVision(item) {
			return models.QuantizedModel{}, "", fmt.Errorf("model %q does not support image translation", requestedModelID)
		}
		path := modelstore.LocalModelPath(dataRoot, item.FileName)
		if path == "" {
			return models.QuantizedModel{}, "", fmt.Errorf("model %q is not installed locally; run `translategemma-ui models download --id %s` first", requestedModelID, requestedModelID)
		}
		return item, path, nil
	}

	if state.ActiveModelPath != "" && runtimeModeForPath(state.ActiveModelPath) != "" && fileExists(state.ActiveModelPath) {
		if item, ok := findModelByID(available, strings.TrimSpace(cfg.ActiveModelID)); ok {
			if !requireVision || supportsVision(item) {
				return item, state.ActiveModelPath, nil
			}
		}
		if !requireVision || strings.Contains(strings.ToLower(filepath.Base(state.ActiveModelPath)), ".mmproj-") {
			return models.QuantizedModel{ID: strings.TrimSpace(cfg.ActiveModelID), FileName: filepath.Base(state.ActiveModelPath)}, state.ActiveModelPath, nil
		}
	}

	for _, item := range preferredInstalledOrder(available, requireVision) {
		if path := modelstore.LocalModelPath(dataRoot, item.FileName); path != "" {
			return item, path, nil
		}
	}

	modeHint := "text"
	if requireVision {
		modeHint = "image"
	}
	return models.QuantizedModel{}, "", fmt.Errorf("no local %s runtime installed; use TUI/WebUI or `translategemma-ui models download --id ...` first", modeHint)
}

func preferredInstalledOrder(available []models.QuantizedModel, requireVision bool) []models.QuantizedModel {
	out := make([]models.QuantizedModel, 0, len(available))
	for _, item := range available {
		if requireVision && !supportsVision(item) {
			continue
		}
		if !requireVision && supportsVision(item) {
			continue
		}
		out = append(out, item)
	}
	sortModels(out)
	return out
}

func sortModels(items []models.QuantizedModel) {
	order := map[string]int{
		"q4_k_m":      1,
		"q6_k":        2,
		"q8_0":        3,
		"q8_0_vision": 4,
	}
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if order[items[j].ID] < order[items[i].ID] {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}

func supportsVision(item models.QuantizedModel) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(item.ID)), "_vision") ||
		strings.Contains(strings.ToLower(item.FileName), ".mmproj-")
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

func runtimeModeForPath(path string) string {
	if strings.Contains(strings.ToLower(path), ".llamafile") {
		return "single_file_llamafile"
	}
	return ""
}
