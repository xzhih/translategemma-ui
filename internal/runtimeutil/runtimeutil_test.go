package runtimeutil

import (
	"testing"

	"translategemma-ui/internal/config"
	"translategemma-ui/internal/models"
)

type fakeBackendTarget struct {
	backendURL string
}

func (f *fakeBackendTarget) SetBackendURL(next string) {
	f.backendURL = next
}

func TestSameRuntimePathMatchesRuntimeWrappers(t *testing.T) {
	a := "/tmp/translategemma-4b-it.Q8_0.llamafile"
	b := "/tmp/translategemma-4b-it.Q8_0.llamafile.exe"

	if !SameRuntimePath(a, b) {
		t.Fatalf("expected runtime wrapper variants to match")
	}
}

func TestApplyActiveModelUpdatesConfigAndState(t *testing.T) {
	cfg := config.AppConfig{}
	state := config.AppState{}

	ApplyActiveModel(&cfg, &state, models.QuantizedModel{ID: "q8_0"}, "/tmp/runtime.llamafile")

	if cfg.ActiveModelID != "q8_0" {
		t.Fatalf("expected config active model id to be updated, got %q", cfg.ActiveModelID)
	}
	if state.ActiveModelPath != "/tmp/runtime.llamafile" {
		t.Fatalf("expected active path to be updated, got %q", state.ActiveModelPath)
	}
	if state.RuntimeMode != "single_file_llamafile" {
		t.Fatalf("expected runtime mode to be inferred, got %q", state.RuntimeMode)
	}
}

func TestSyncBackendURLNormalizesAndUpdatesTargets(t *testing.T) {
	state := config.AppState{}
	runtimeTarget := &fakeBackendTarget{}
	serviceTarget := &fakeBackendTarget{}

	got := SyncBackendURL(&state, "127.0.0.1:8080", runtimeTarget, serviceTarget)

	if got != "http://127.0.0.1:8080" {
		t.Fatalf("expected normalized backend url, got %q", got)
	}
	if state.BackendURL != got {
		t.Fatalf("expected state backend url to match normalized value, got %q", state.BackendURL)
	}
	if runtimeTarget.backendURL != got {
		t.Fatalf("expected runtime target backend url to update, got %q", runtimeTarget.backendURL)
	}
	if serviceTarget.backendURL != got {
		t.Fatalf("expected service target backend url to update, got %q", serviceTarget.backendURL)
	}
}
