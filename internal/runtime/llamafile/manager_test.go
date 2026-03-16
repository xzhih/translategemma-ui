package llamafile

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildLaunchCommandLlamafileUsesServerArgs(t *testing.T) {
	dataRoot := t.TempDir()
	m := NewManager(dataRoot, "http://127.0.0.1:8080")

	candidate, err := m.buildLaunchCommand("/tmp/model.llamafile", "127.0.0.1", "8080")
	if err != nil {
		t.Fatalf("buildLaunchCommand returned error: %v", err)
	}
	args := strings.Join(candidate.Args, " ")
	if !strings.Contains(args, "--server") || !strings.Contains(args, "--host 127.0.0.1") || !strings.Contains(args, "--port 8080") {
		t.Fatalf("expected server launch args, got %q", args)
	}
}

func TestBuildLaunchCommandAPELLamafileUsesShellWrapper(t *testing.T) {
	shellPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}

	dataRoot := t.TempDir()
	runtimePath := filepath.Join(dataRoot, "model.llamafile")
	if err := os.WriteFile(runtimePath, []byte("MZqFpD='fake-ape"), 0o755); err != nil {
		t.Fatalf("write fake runtime: %v", err)
	}

	m := NewManager(dataRoot, "http://127.0.0.1:8080")
	candidate, err := m.buildLaunchCommand(runtimePath, "127.0.0.1", "8080")
	if err != nil {
		t.Fatalf("buildLaunchCommand returned error: %v", err)
	}
	if candidate.Path != shellPath {
		t.Fatalf("expected shell wrapper %q, got %q", shellPath, candidate.Path)
	}
	if len(candidate.Args) == 0 || candidate.Args[0] != runtimePath {
		t.Fatalf("expected runtime path as first shell arg, got %#v", candidate.Args)
	}
}

func TestBuildLaunchCommandPlainLlamafileStaysDirect(t *testing.T) {
	dataRoot := t.TempDir()
	runtimePath := filepath.Join(dataRoot, "model.llamafile")
	if err := os.WriteFile(runtimePath, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatalf("write fake runtime: %v", err)
	}

	m := NewManager(dataRoot, "http://127.0.0.1:8080")
	candidate, err := m.buildLaunchCommand(runtimePath, "127.0.0.1", "8080")
	if err != nil {
		t.Fatalf("buildLaunchCommand returned error: %v", err)
	}
	if candidate.Path != runtimePath {
		t.Fatalf("expected direct runtime path %q, got %q", runtimePath, candidate.Path)
	}
}

func TestBuildLaunchCommandRejectsNonLlamafile(t *testing.T) {
	m := NewManager(t.TempDir(), "http://127.0.0.1:8080")
	if _, err := m.buildLaunchCommand("/tmp/model.gguf", "127.0.0.1", "8080"); err == nil {
		t.Fatalf("expected non-llamafile runtime to be rejected")
	}
}
