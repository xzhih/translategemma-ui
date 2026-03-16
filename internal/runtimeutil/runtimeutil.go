package runtimeutil

import (
	"path/filepath"
	"strings"

	"translategemma-ui/internal/config"
	"translategemma-ui/internal/models"
	"translategemma-ui/internal/runtime"
)

// BackendURLTarget is implemented by runtime managers and translators that can swap backends in place.
type BackendURLTarget interface {
	SetBackendURL(string)
}

// RuntimeModeForPath resolves the persisted runtime mode for a local artifact path.
func RuntimeModeForPath(path string) string {
	if strings.Contains(strings.ToLower(strings.TrimSpace(path)), ".llamafile") {
		return "single_file_llamafile"
	}
	return ""
}

// SameRuntimePath treats platform-specific runtime wrappers as the same packaged runtime.
func SameRuntimePath(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	return runtimeStem(a) == runtimeStem(b)
}

// CanReuseLoadedRuntime reports whether the requested runtime is already the loaded backend.
func CanReuseLoadedRuntime(selectedPath, activePath string, runtimeReady bool) bool {
	return runtimeReady && SameRuntimePath(selectedPath, activePath)
}

// ApplyRuntimePath updates persisted runtime metadata for a selected local artifact.
func ApplyRuntimePath(state *config.AppState, modelPath string) {
	if state == nil {
		return
	}
	modelPath = strings.TrimSpace(modelPath)
	state.ActiveModelPath = modelPath
	state.RuntimeMode = RuntimeModeForPath(modelPath)
}

// ApplyActiveModel updates the shared config/state pair for a selected local model.
func ApplyActiveModel(cfg *config.AppConfig, state *config.AppState, item models.QuantizedModel, modelPath string) {
	if cfg != nil {
		cfg.ActiveModelID = strings.TrimSpace(item.ID)
	}
	ApplyRuntimePath(state, modelPath)
}

// SyncBackendURL normalizes a backend URL, stores it in state, and updates any live targets.
func SyncBackendURL(state *config.AppState, next string, targets ...BackendURLTarget) string {
	next = runtime.NormalizeBackendURL(next)
	if next == "" {
		next = runtime.NormalizeBackendURL(runtime.DefaultBackendURL)
	}
	if state != nil {
		state.BackendURL = next
	}
	for _, target := range targets {
		if target == nil {
			continue
		}
		target.SetBackendURL(next)
	}
	return next
}

func runtimeStem(path string) string {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(path)))
	for _, suffix := range []string{".llamafile.exe", ".llamafile", ".exe"} {
		base = strings.TrimSuffix(base, suffix)
	}
	return base
}
