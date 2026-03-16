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
