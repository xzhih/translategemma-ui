package modelstore

import (
	"os"
	"path/filepath"
	"testing"

	"translategemma-ui/internal/models"
)

func TestLocalModelPathOnlyAcceptsPackagedRuntime(t *testing.T) {
	root := t.TempDir()
	runtimePath := filepath.Join(root, "runtimes", "translategemma-4b-it.Q4_K_M.llamafile")
	if err := os.MkdirAll(filepath.Dir(runtimePath), 0o755); err != nil {
		t.Fatalf("mkdir runtimes: %v", err)
	}
	if err := os.WriteFile(runtimePath, []byte("runtime"), 0o644); err != nil {
		t.Fatalf("write runtime artifact: %v", err)
	}

	if got := LocalModelPath(root, "translategemma-4b-it.Q4_K_M.llamafile"); got != runtimePath {
		t.Fatalf("expected packaged runtime path %q, got %q", runtimePath, got)
	}
	if got := LocalModelPath(root, "translategemma-4b-it.Q4_K_M.gguf"); got != "" {
		t.Fatalf("expected gguf lookup to be ignored, got %q", got)
	}
}

func TestDeleteModelRemovesWindowsExecutableVariant(t *testing.T) {
	root := t.TempDir()
	runtimePath := filepath.Join(root, "runtimes", "translategemma-4b-it.Q4_K_M.llamafile.exe")
	if err := os.MkdirAll(filepath.Dir(runtimePath), 0o755); err != nil {
		t.Fatalf("mkdir runtimes: %v", err)
	}
	if err := os.WriteFile(runtimePath, []byte("runtime"), 0o644); err != nil {
		t.Fatalf("write runtime artifact: %v", err)
	}

	item := models.QuantizedModel{FileName: "translategemma-4b-it.Q4_K_M.llamafile"}
	removedPath, removed, err := deleteModelForOS(root, item, nil, "windows")
	if err != nil {
		t.Fatalf("delete model: %v", err)
	}
	if !removed {
		t.Fatalf("expected model to be removed")
	}
	if removedPath != runtimePath {
		t.Fatalf("expected removed path %q, got %q", runtimePath, removedPath)
	}
	if _, err := os.Stat(runtimePath); !os.IsNotExist(err) {
		t.Fatalf("expected runtime artifact to be deleted, got err=%v", err)
	}
}

func TestCatalogMarksWindowsExecutableVariantAsActive(t *testing.T) {
	root := t.TempDir()
	runtimePath := filepath.Join(root, "runtimes", "translategemma-4b-it.Q4_K_M.llamafile")
	if err := os.MkdirAll(filepath.Dir(runtimePath), 0o755); err != nil {
		t.Fatalf("mkdir runtimes: %v", err)
	}
	if err := os.WriteFile(runtimePath, []byte("runtime"), 0o644); err != nil {
		t.Fatalf("write runtime artifact: %v", err)
	}

	items := Catalog(root, []models.QuantizedModel{
		{ID: "q4_k_m", FileName: "translategemma-4b-it.Q4_K_M.llamafile"},
	}, "", filepath.Join(root, "runtimes", "translategemma-4b-it.Q4_K_M.llamafile.exe"))

	if len(items) != 1 {
		t.Fatalf("expected one catalog item, got %d", len(items))
	}
	if !items[0].Active {
		t.Fatalf("expected runtime wrapper variant to be marked active")
	}
}

func TestResolveCatalogItemPrefersActiveThenPreferredThenRankedFallback(t *testing.T) {
	items := []CatalogItem{
		{
			QuantizedModel: models.QuantizedModel{ID: "q8_0", FileName: "translategemma-4b-it.Q8_0.llamafile"},
			Installed:      true,
			Path:           "/tmp/translategemma-4b-it.Q8_0.llamafile",
		},
		{
			QuantizedModel: models.QuantizedModel{ID: "q4_k_m", FileName: "translategemma-4b-it.Q4_K_M.llamafile"},
			Installed:      true,
			Path:           "/tmp/translategemma-4b-it.Q4_K_M.llamafile",
		},
		{
			QuantizedModel: models.QuantizedModel{ID: "q8_0_vision", FileName: "translategemma-4b-it.Q8_0.mmproj-Q8_0.llamafile"},
			Installed:      true,
			Path:           "/tmp/translategemma-4b-it.Q8_0.mmproj-Q8_0.llamafile",
		},
	}

	idx, item, ok := ResolveCatalogItem(items, "/tmp/translategemma-4b-it.Q8_0.llamafile", ResolveOptions{PreferTextRuntime: true})
	if !ok || idx != 0 || item.ID != "q8_0" {
		t.Fatalf("expected active path to win, got idx=%d item=%q ok=%v", idx, item.ID, ok)
	}

	idx, item, ok = ResolveCatalogItem(items, "", ResolveOptions{PreferTextRuntime: true}, "q8_0")
	if !ok || idx != 0 || item.ID != "q8_0" {
		t.Fatalf("expected preferred id to win, got idx=%d item=%q ok=%v", idx, item.ID, ok)
	}

	idx, item, ok = ResolveCatalogItem(items, "", ResolveOptions{PreferTextRuntime: true})
	if !ok || idx != 1 || item.ID != "q4_k_m" {
		t.Fatalf("expected ranked text fallback to prefer q4_k_m, got idx=%d item=%q ok=%v", idx, item.ID, ok)
	}
}

func TestResolveCatalogItemFallsBackToVisionWhenNeeded(t *testing.T) {
	items := []CatalogItem{
		{
			QuantizedModel: models.QuantizedModel{ID: "q8_0_vision", FileName: "translategemma-4b-it.Q8_0.mmproj-Q8_0.llamafile"},
			Installed:      true,
			Path:           "/tmp/translategemma-4b-it.Q8_0.mmproj-Q8_0.llamafile",
		},
	}

	_, item, ok := ResolveCatalogItem(items, "", ResolveOptions{PreferTextRuntime: true})
	if !ok || item.ID != "q8_0_vision" {
		t.Fatalf("expected vision runtime fallback when it is the only local model, got item=%q ok=%v", item.ID, ok)
	}

	_, item, ok = ResolveCatalogItem(items, "", ResolveOptions{RequireVision: true}, "q8_0_vision")
	if !ok || item.ID != "q8_0_vision" {
		t.Fatalf("expected vision selection to succeed when required, got item=%q ok=%v", item.ID, ok)
	}
}
