package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"translategemma-ui/internal/languages"
)

func TestLocalModelPathFindsPackagedLlamafile(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtimes")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime dir: %v", err)
	}

	packagedPath := filepath.Join(runtimeDir, "translategemma-4b-it.Q5_K_M.llamafile")
	if err := os.WriteFile(packagedPath, []byte("runtime"), 0o755); err != nil {
		t.Fatalf("write packaged file: %v", err)
	}

	m := model{dataRoot: root}
	got := m.localModelPath("translategemma-4b-it.Q5_K_M.llamafile")
	if got != packagedPath {
		t.Fatalf("expected packaged path %q, got %q", packagedPath, got)
	}
}

func TestNewModelAutoSelectsInstalledRuntime(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtimes")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime dir: %v", err)
	}

	const runtimeFile = "translategemma-4b-it.Q4_K_M.llamafile"
	runtimePath := filepath.Join(runtimeDir, runtimeFile)
	if err := os.WriteFile(runtimePath, []byte("#!/bin/sh\necho runtime\n"), 0o755); err != nil {
		t.Fatalf("write runtime file: %v", err)
	}

	manifestPath := filepath.Join(root, "manifest-v1.json")
	manifest := `{
  "models": [
    {
      "id": "q4_k_m",
      "display_name": "q4_k_m",
      "recommended": true,
      "features": { "vision": false },
      "runtime": {
        "llamafile": {
          "file_name": "translategemma-4b-it.Q4_K_M.llamafile",
          "path_in_repo": "translategemma-4b-it.Q4_K_M.llamafile",
          "size_bytes": 123
        }
      }
    }
  ]
}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	t.Setenv("TRANSLATEGEMMA_UI_MANIFEST_PATH", manifestPath)

	m := newModel("", root)

	if m.selectedName != runtimeFile {
		t.Fatalf("expected selected runtime %q, got %q", runtimeFile, m.selectedName)
	}
	if m.state.ActiveModelPath != runtimePath {
		t.Fatalf("expected active runtime path %q, got %q", runtimePath, m.state.ActiveModelPath)
	}
	if m.screen == modelScreen {
		t.Fatalf("expected startup to skip plain model selection when a local runtime exists")
	}
}

func TestCycleLanguageWrapsAcrossOptions(t *testing.T) {
	options := languages.WithoutAuto()
	if len(options) < 2 {
		t.Fatalf("expected multiple language options")
	}

	first := options[0].Code
	last := options[len(options)-1].Code
	if got := cycleLanguage(first, options, -1); got != last {
		t.Fatalf("expected previous language to wrap to %q, got %q", last, got)
	}

	if got := cycleLanguage(last, options, 1); got != first {
		t.Fatalf("expected next language to wrap to %q, got %q", first, got)
	}

	enIndex := -1
	for i, option := range options {
		if option.Code == "en" {
			enIndex = i
			break
		}
	}
	if enIndex == -1 {
		t.Fatalf("expected English in language options")
	}
	if got := cycleLanguage("en", options, 1); got != options[(enIndex+1)%len(options)].Code {
		t.Fatalf("expected next language after English to follow option order, got %q", got)
	}
}

func TestTranslateScreenForwardsTypingToFocusedTextarea(t *testing.T) {
	m := newModel("", t.TempDir())
	m.screen = translateScreen
	_ = m.setFocus(textFocus)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	updated := next.(model)

	if got := updated.input.Value(); got != "h" {
		t.Fatalf("expected input textarea to receive typed rune, got %q", got)
	}
}
