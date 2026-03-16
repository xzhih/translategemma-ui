package modelstore

import (
	"strings"

	"translategemma-ui/internal/models"
	"translategemma-ui/internal/runtimeutil"
)

type ResolveOptions struct {
	RequireVision     bool
	PreferTextRuntime bool
}

// ResolveCatalogItem picks the best installed catalog entry using shared selection rules.
func ResolveCatalogItem(items []CatalogItem, activePath string, opts ResolveOptions, preferredIDs ...string) (int, CatalogItem, bool) {
	if idx, item, ok := matchCatalogByPath(items, activePath, opts.RequireVision); ok {
		return idx, item, true
	}
	for _, id := range uniquePreferredIDs(preferredIDs...) {
		if idx, item, ok := matchCatalogByID(items, id, opts.RequireVision); ok {
			return idx, item, true
		}
	}
	for _, candidate := range orderedFallbackModels(items, opts) {
		if idx, item, ok := matchCatalogByID(items, candidate.ID, opts.RequireVision); ok {
			return idx, item, true
		}
	}
	return 0, CatalogItem{}, false
}

func matchCatalogByPath(items []CatalogItem, targetPath string, requireVision bool) (int, CatalogItem, bool) {
	for idx, item := range items {
		if !item.Installed || item.Path == "" || !runtimeutil.SameRuntimePath(item.Path, targetPath) {
			continue
		}
		if requireVision && !models.SupportsVision(item.QuantizedModel) {
			continue
		}
		return idx, item, true
	}
	return 0, CatalogItem{}, false
}

func matchCatalogByID(items []CatalogItem, targetID string, requireVision bool) (int, CatalogItem, bool) {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return 0, CatalogItem{}, false
	}
	for idx, item := range items {
		if item.ID != targetID || !item.Installed {
			continue
		}
		if requireVision && !models.SupportsVision(item.QuantizedModel) {
			continue
		}
		return idx, item, true
	}
	return 0, CatalogItem{}, false
}

func orderedFallbackModels(items []CatalogItem, opts ResolveOptions) []models.QuantizedModel {
	all := make([]models.QuantizedModel, 0, len(items))
	textPreferred := make([]models.QuantizedModel, 0, len(items))
	visionFallback := make([]models.QuantizedModel, 0, len(items))
	for _, item := range items {
		if !item.Installed {
			continue
		}
		if opts.RequireVision {
			if models.SupportsVision(item.QuantizedModel) {
				all = append(all, item.QuantizedModel)
			}
			continue
		}
		all = append(all, item.QuantizedModel)
		if models.SupportsVision(item.QuantizedModel) {
			visionFallback = append(visionFallback, item.QuantizedModel)
			continue
		}
		textPreferred = append(textPreferred, item.QuantizedModel)
	}
	if opts.RequireVision {
		return models.PreferredOrder(all)
	}
	if opts.PreferTextRuntime && len(textPreferred) > 0 {
		out := models.PreferredOrder(textPreferred)
		return append(out, models.PreferredOrder(visionFallback)...)
	}
	return models.PreferredOrder(all)
}

func uniquePreferredIDs(ids ...string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
