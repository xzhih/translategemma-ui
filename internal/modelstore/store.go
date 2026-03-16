package modelstore

import (
	"os"
	"path/filepath"
	"strings"

	"translategemma-ui/internal/config"
	"translategemma-ui/internal/models"
	"translategemma-ui/internal/platform"
	"translategemma-ui/internal/runtimeutil"
)

type CatalogItem struct {
	models.QuantizedModel
	Installed bool
	Path      string
	Active    bool
}

func Catalog(dataRoot string, available []models.QuantizedModel, activeModelID, activeModelPath string) []CatalogItem {
	items := make([]CatalogItem, 0, len(available))
	for _, item := range available {
		path := LocalModelPath(dataRoot, item.FileName)
		items = append(items, CatalogItem{
			QuantizedModel: item,
			Installed:      path != "",
			Path:           path,
			Active:         path != "" && (item.ID == activeModelID || runtimeutil.SameRuntimePath(path, activeModelPath)),
		})
	}
	return items
}

func LocalModelPath(dataRoot, fileName string) string {
	return localModelPathForOS(dataRoot, fileName, runtimeGOOS())
}

func localModelPathForOS(dataRoot, fileName, goos string) string {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return ""
	}
	if !strings.HasSuffix(strings.ToLower(fileName), ".llamafile") {
		return ""
	}
	for _, candidate := range platform.RuntimeFileCandidates(goos, fileName) {
		path := filepath.Join(dataRoot, "runtimes", candidate)
		if fileExists(path) {
			return path
		}
	}
	return ""
}

func DeleteModel(dataRoot string, item models.QuantizedModel, state *config.AppState) (string, bool, error) {
	return deleteModelForOS(dataRoot, item, state, runtimeGOOS())
}

func deleteModelForOS(dataRoot string, item models.QuantizedModel, state *config.AppState, goos string) (string, bool, error) {
	base := strings.TrimSpace(item.FileName)
	if base == "" {
		return "", false, nil
	}
	paths := make([]string, 0, 2)
	for _, candidate := range platform.RuntimeFileCandidates(goos, base) {
		paths = append(paths, filepath.Join(dataRoot, "runtimes", candidate))
	}

	removedAny := false
	removedPath := ""
	for _, path := range paths {
		if !fileExists(path) {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return "", false, err
		}
		if removedPath == "" {
			removedPath = path
		}
		removedAny = true
	}
	if !removedAny {
		return "", false, nil
	}
	if state != nil {
		for _, path := range paths {
			if state.ActiveModelPath != path {
				continue
			}
			state.ActiveModelPath = ""
			state.RuntimeMode = ""
			break
		}
		filtered := state.Artifacts[:0]
		for _, artifact := range state.Artifacts {
			keep := true
			for _, path := range paths {
				if artifact.Path == path {
					keep = false
					break
				}
			}
			if !keep {
				continue
			}
			filtered = append(filtered, artifact)
		}
		state.Artifacts = filtered
	}
	return removedPath, true, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func runtimeGOOS() string {
	current := platform.Current()
	if idx := strings.IndexByte(current, '/'); idx > 0 {
		return current[:idx]
	}
	return current
}
