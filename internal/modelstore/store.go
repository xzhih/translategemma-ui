package modelstore

import (
	"os"
	"path/filepath"
	"strings"

	"translategemma-ui/internal/config"
	"translategemma-ui/internal/models"
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
			Active:         path != "" && (item.ID == activeModelID || path == activeModelPath),
		})
	}
	return items
}

func LocalModelPath(dataRoot, fileName string) string {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return ""
	}
	if !strings.HasSuffix(strings.ToLower(fileName), ".llamafile") {
		return ""
	}
	path := filepath.Join(dataRoot, "runtimes", fileName)
	if fileExists(path) {
		return path
	}
	return ""
}

func DeleteModel(dataRoot string, item models.QuantizedModel, state *config.AppState) (string, bool, error) {
	base := strings.TrimSpace(item.FileName)
	if base == "" {
		return "", false, nil
	}
	paths := []string{filepath.Join(dataRoot, "runtimes", base)}

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
