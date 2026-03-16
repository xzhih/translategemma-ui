package tui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"translategemma-ui/internal/config"
	"translategemma-ui/internal/huggingface"
	"translategemma-ui/internal/languages"
	"translategemma-ui/internal/models"
	"translategemma-ui/internal/runtime"
	"translategemma-ui/internal/translate"
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
	if err := config.SaveAppState(root, config.AppState{BackendURL: "http://127.0.0.1:1"}); err != nil {
		t.Fatalf("save app state: %v", err)
	}

	restoreCatalog := huggingface.SeedCatalogForTests([]models.QuantizedModel{
		{
			ID:          "q4_k_m",
			Kind:        "model",
			FileName:    runtimeFile,
			Size:        "123 B",
			SizeBytes:   123,
			DownloadURL: "https://example.com/translategemma-4b-it.Q4_K_M.llamafile",
			Recommended: true,
		},
	})
	defer restoreCatalog()

	m := newModel("", root)

	if m.selectedName != runtimeFile {
		t.Fatalf("expected selected runtime %q, got %q", runtimeFile, m.selectedName)
	}
	if m.state.ActiveModelPath != runtimePath {
		t.Fatalf("expected active runtime path %q, got %q", runtimePath, m.state.ActiveModelPath)
	}
	if m.screen != translateScreen {
		t.Fatalf("expected startup to open translate screen when a local runtime exists, got %v", m.screen)
	}
	if m.runtimeReady {
		t.Fatalf("expected startup to keep runtime idle until first translation")
	}
	if m.status != "Runtime idle. The selected model will load on first translation." {
		t.Fatalf("unexpected idle status: %q", m.status)
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

func TestTranslateScreenAutoLoadsInstalledRuntimeWhenBackendIdle(t *testing.T) {
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
	if err := config.SaveAppState(root, config.AppState{BackendURL: "http://127.0.0.1:1"}); err != nil {
		t.Fatalf("save app state: %v", err)
	}

	restoreCatalog := huggingface.SeedCatalogForTests([]models.QuantizedModel{
		{
			ID:          "q4_k_m",
			Kind:        "model",
			FileName:    runtimeFile,
			Size:        "123 B",
			SizeBytes:   123,
			DownloadURL: "https://example.com/translategemma-4b-it.Q4_K_M.llamafile",
			Recommended: true,
		},
	})
	defer restoreCatalog()

	m := newModel("", root)
	m.screen = translateScreen
	m.input.SetValue("hello")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	updated := next.(model)

	if updated.screen != provisionScreen {
		t.Fatalf("expected auto-load to move to provision screen, got %v", updated.screen)
	}
	if updated.pendingRequest == nil {
		t.Fatalf("expected translation request to be queued while runtime loads")
	}
	if updated.pendingRequest.Text != "hello" {
		t.Fatalf("expected queued request text %q, got %q", "hello", updated.pendingRequest.Text)
	}
	if updated.state.ActiveModelPath != runtimePath {
		t.Fatalf("expected active runtime path %q, got %q", runtimePath, updated.state.ActiveModelPath)
	}
	if cmd == nil {
		t.Fatalf("expected auto-load to start a runtime activation command")
	}
}

func TestModelScreenShowsSelectedAndLoadedStatesSeparately(t *testing.T) {
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
	if err := config.SaveAppState(root, config.AppState{BackendURL: "http://127.0.0.1:1"}); err != nil {
		t.Fatalf("save app state: %v", err)
	}

	restoreCatalog := huggingface.SeedCatalogForTests([]models.QuantizedModel{
		{
			ID:          "q4_k_m",
			Kind:        "model",
			FileName:    runtimeFile,
			Size:        "123 B",
			SizeBytes:   123,
			DownloadURL: "https://example.com/translategemma-4b-it.Q4_K_M.llamafile",
			Recommended: true,
		},
	})
	defer restoreCatalog()

	m := newModel("", root)
	m.screen = modelScreen
	m.runtimeReady = false

	idleView := m.View()
	if !strings.Contains(idleView, "SELECTED") {
		t.Fatalf("expected idle model view to show selected runtime, got:\n%s", idleView)
	}
	if strings.Contains(idleView, "active") {
		t.Fatalf("expected idle model view to avoid stale active label, got:\n%s", idleView)
	}
	if strings.Contains(idleView, "LOADED") {
		t.Fatalf("expected idle model view to avoid loaded label, got:\n%s", idleView)
	}
	if !strings.Contains(idleView, "Runtime Catalog") {
		t.Fatalf("expected redesigned model view to render the runtime catalog panel, got:\n%s", idleView)
	}

	m.runtimeReady = true
	_ = m.syncCatalogList()
	readyView := m.View()
	if !strings.Contains(readyView, "LOADED") {
		t.Fatalf("expected ready model view to show loaded runtime, got:\n%s", readyView)
	}
	if !strings.Contains(readyView, "Loaded runtime: "+runtimeFile) {
		t.Fatalf("expected banner to show loaded runtime descriptor, got:\n%s", readyView)
	}
}

func TestSelectingAlreadyLoadedRuntimeReturnsToWorkspaceWithoutReload(t *testing.T) {
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models", "/healthz", "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := config.SaveAppState(root, config.AppState{BackendURL: server.URL}); err != nil {
		t.Fatalf("save app state: %v", err)
	}

	restoreCatalog := huggingface.SeedCatalogForTests([]models.QuantizedModel{
		{
			ID:          "q4_k_m",
			Kind:        "model",
			FileName:    runtimeFile,
			Size:        "123 B",
			SizeBytes:   123,
			DownloadURL: "https://example.com/translategemma-4b-it.Q4_K_M.llamafile",
			Recommended: true,
		},
	})
	defer restoreCatalog()

	m := newModel("", root)
	if !m.runtimeReady {
		t.Fatalf("expected startup probe to mark the runtime ready")
	}
	m.screen = modelScreen

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := next.(model)

	if updated.screen != translateScreen {
		t.Fatalf("expected selecting the already loaded runtime to return to workspace, got %v", updated.screen)
	}
	if updated.workTask != nil {
		t.Fatalf("expected no provision task when selecting the already loaded runtime")
	}
	if updated.provisionStage != "" {
		t.Fatalf("expected no provision stage when runtime is already loaded, got %q", updated.provisionStage)
	}
	if updated.status != "Runtime already loaded: "+runtimeFile {
		t.Fatalf("unexpected status after reselecting loaded runtime: %q", updated.status)
	}
	if cmd == nil {
		t.Fatalf("expected returning to workspace to restore input focus")
	}
}

func TestProvisionDoneSyncsBackendAndResumesPendingTranslation(t *testing.T) {
	m := newModel("", t.TempDir())
	m.screen = provisionScreen
	m.pendingRequest = &translate.Request{
		SourceLang: "en",
		TargetLang: "zh-CN",
		Text:       "hello",
	}

	next, cmd := m.Update(provisionDoneMsg{
		ModelPath:  "/tmp/translategemma-4b-it.Q4_K_M.llamafile",
		BackendURL: "http://127.0.0.1:18080",
		Message:    "Runtime ready",
	})
	updated := next.(model)

	if updated.screen != translateScreen {
		t.Fatalf("expected translate screen after runtime load, got %v", updated.screen)
	}
	if !updated.streaming {
		t.Fatalf("expected pending translation to resume automatically")
	}
	if updated.pendingRequest != nil {
		t.Fatalf("expected pending translation request to be cleared")
	}
	if updated.backendURL != runtime.NormalizeBackendURL("http://127.0.0.1:18080") {
		t.Fatalf("expected model backend URL to sync, got %q", updated.backendURL)
	}
	if updated.state.BackendURL != runtime.NormalizeBackendURL("http://127.0.0.1:18080") {
		t.Fatalf("expected state backend URL to sync, got %q", updated.state.BackendURL)
	}
	if updated.service.BackendURL != runtime.NormalizeBackendURL("http://127.0.0.1:18080") {
		t.Fatalf("expected translate service backend URL to sync, got %q", updated.service.BackendURL)
	}
	if updated.status != "Streaming translation..." {
		t.Fatalf("expected status to transition to streaming, got %q", updated.status)
	}
	if cmd == nil {
		t.Fatalf("expected resumed translation to return a stream command")
	}
}

func TestProvisionDoneDoesNotCancelSuccessfulRuntimeTask(t *testing.T) {
	m := newModel("", t.TempDir())
	m.screen = provisionScreen

	cancelCalls := 0
	m.workTask = &taskState{
		onCancel: func() {
			cancelCalls++
		},
	}

	next, _ := m.Update(provisionDoneMsg{
		ModelPath:  "/tmp/translategemma-4b-it.Q4_K_M.llamafile",
		BackendURL: "http://127.0.0.1:18080",
		Message:    "Runtime ready",
	})
	updated := next.(model)

	if cancelCalls != 0 {
		t.Fatalf("expected successful provision completion to avoid cancel callback, got %d calls", cancelCalls)
	}
	if updated.workTask != nil {
		t.Fatalf("expected successful provision completion to release the task")
	}
}

func TestTaskClosedDoesNotTriggerProvisionCancelCallback(t *testing.T) {
	m := newModel("", t.TempDir())

	cancelCalls := 0
	task := &taskState{
		onCancel: func() {
			cancelCalls++
		},
	}
	m.workTask = task

	updated := m.handleTaskClosed(taskClosedMsg{kind: provisionTaskKind, task: task})

	if cancelCalls != 0 {
		t.Fatalf("expected natural task closure to avoid cancel callback, got %d calls", cancelCalls)
	}
	if updated.workTask != nil {
		t.Fatalf("expected natural task closure to release the task")
	}
}

func TestStaleProvisionTaskClosedDoesNotClearCurrentTask(t *testing.T) {
	m := newModel("", t.TempDir())
	oldTask := &taskState{}
	currentTask := &taskState{}
	m.workTask = currentTask
	m.pendingRequest = &translate.Request{Text: "hello"}

	updated := m.handleTaskClosed(taskClosedMsg{kind: provisionTaskKind, task: oldTask})

	if updated.workTask != currentTask {
		t.Fatalf("expected stale close event to preserve the current provision task")
	}
	if updated.pendingRequest == nil {
		t.Fatalf("expected stale close event to preserve the pending request")
	}
}

func TestStaleStreamTaskClosedDoesNotClearCurrentTask(t *testing.T) {
	m := newModel("", t.TempDir())
	oldTask := &taskState{}
	currentTask := &taskState{}
	m.streamTask = currentTask
	m.streaming = true

	updated := m.handleTaskClosed(taskClosedMsg{kind: streamTaskKind, task: oldTask})

	if updated.streamTask != currentTask {
		t.Fatalf("expected stale close event to preserve the current stream task")
	}
	if !updated.streaming {
		t.Fatalf("expected stale close event to preserve streaming state")
	}
}
