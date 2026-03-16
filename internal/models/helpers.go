package models

import (
	"sort"
	"strings"
)

// FindByID looks up a packaged runtime by its stable ID.
func FindByID(items []QuantizedModel, id string) (QuantizedModel, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return QuantizedModel{}, false
	}
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return QuantizedModel{}, false
}

// SupportsVision reports whether the packaged runtime can translate images.
func SupportsVision(item QuantizedModel) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(item.ID)), "_vision") ||
		strings.Contains(strings.ToLower(item.FileName), ".mmproj-")
}

// PreferredOrder returns a stable copy ordered by the shared runtime preference.
func PreferredOrder(items []QuantizedModel) []QuantizedModel {
	out := append([]QuantizedModel(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		return preferenceRank(out[i]) < preferenceRank(out[j])
	})
	return out
}

func preferenceRank(item QuantizedModel) string {
	switch strings.ToLower(strings.TrimSpace(item.ID)) {
	case "q4_k_m":
		return "1"
	case "q6_k":
		return "2"
	case "q8_0":
		return "3"
	case "q8_0_vision":
		return "4"
	default:
		return "9:" + strings.ToLower(strings.TrimSpace(item.ID))
	}
}
