package web

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"translategemma-ui/internal/config"
	"translategemma-ui/internal/models"
	"translategemma-ui/internal/runtime"
	lf "translategemma-ui/internal/runtime/llamafile"
	"translategemma-ui/internal/translate"
)

type fakeRuntimeManager struct {
	backendURL     string
	modelPath      string
	stopCalls      int
	stopOwnedCalls int
	ensureCalls    int
	ready          bool
}

type fakeTranslator struct {
	backendURL       string
	translateFn      func(context.Context, translate.Request) (string, error)
	streamFn         func(context.Context, translate.Request, func(string) error, func(translate.ProgressUpdate) error) (string, error)
	translateImageFn func(context.Context, translate.ImageRequest) (string, error)
}

func (f fakeTranslator) TranslateWithContext(ctx context.Context, req translate.Request) (string, error) {
	if f.translateFn == nil {
		return "", nil
	}
	return f.translateFn(ctx, req)
}

func (f fakeTranslator) StreamTranslateWithContextAndProgress(
	ctx context.Context,
	req translate.Request,
	onDelta func(string) error,
	onProgress func(translate.ProgressUpdate) error,
) (string, error) {
	if f.streamFn == nil {
		return "", nil
	}
	return f.streamFn(ctx, req, onDelta, onProgress)
}

func (f fakeTranslator) TranslateImageWithContext(ctx context.Context, req translate.ImageRequest) (string, error) {
	if f.translateImageFn == nil {
		return "", nil
	}
	return f.translateImageFn(ctx, req)
}

func (f fakeTranslator) SetBackendURL(next string) {
	f.backendURL = next
}

func (f *fakeRuntimeManager) SetBackendURL(next string) {
	f.backendURL = next
}

func (f *fakeRuntimeManager) SetPreferredModelPath(path string) { f.modelPath = path }

func (f *fakeRuntimeManager) CurrentBackendURL() string {
	if strings.TrimSpace(f.backendURL) == "" {
		return runtime.DefaultBackendURL
	}
	return f.backendURL
}

func (f *fakeRuntimeManager) Stop() error {
	f.stopCalls++
	f.ready = false
	return nil
}

func (f *fakeRuntimeManager) StopOwned() error {
	f.stopOwnedCalls++
	f.ready = false
	return nil
}

func (f *fakeRuntimeManager) EnsureRunningWithProgress(onProgress func(lf.Progress)) (runtime.Status, error) {
	f.ensureCalls++
	if onProgress != nil {
		onProgress(lf.Progress{Stage: "load", Percent: 10, Message: "Loading model into runtime"})
		onProgress(lf.Progress{Stage: "load", Percent: 100, Message: "Runtime ready"})
	}
	f.ready = true
	return runtime.Status{Ready: true, Message: "Runtime ready"}, nil
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	uiShell, err := loadIndexHTML()
	if err != nil {
		t.Fatalf("loadIndexHTML returned error: %v", err)
	}
	static, err := newStaticHandler()
	if err != nil {
		t.Fatalf("newStaticHandler returned error: %v", err)
	}

	manager := &fakeRuntimeManager{
		backendURL: "http://127.0.0.1:8080",
		ready:      true,
	}

	return &Server{
		uiShell:        uiShell,
		static:         static,
		backendURL:     "http://127.0.0.1:8080",
		dataRoot:       t.TempDir(),
		runtimeManager: manager,
		availableModels: []models.QuantizedModel{
			{ID: "q8_0", FileName: "translategemma-4b-it.Q8_0.llamafile", Size: "3.8 GB", Recommended: true},
			{ID: "q2_k", FileName: "translategemma-4b-it.Q2_K.llamafile", Size: "1.6 GB"},
		},
		activeModel: resolveActiveModel([]models.QuantizedModel{
			{ID: "q8_0", FileName: "translategemma-4b-it.Q8_0.llamafile", Size: "3.8 GB", Recommended: true},
			{ID: "q2_k", FileName: "translategemma-4b-it.Q2_K.llamafile", Size: "1.6 GB"},
		}, "q8_0"),
		languages: supportedLanguages(),
		probeBackend: func(string) runtime.Status {
			if manager.ready {
				return runtime.Status{Ready: true, Message: "Runtime ready"}
			}
			return runtime.Status{Ready: false, Message: "Runtime idle"}
		},
		now: func() time.Time {
			return time.Date(2026, time.March, 6, 9, 27, 54, 0, time.UTC)
		},
		cfg:    config.AppConfig{ActiveModelID: "q8_0"},
		status: makeServerStatus(statusCodeReady, "Ready"),
	}
}

func TestIndexRendersEmbeddedAssets(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<div id="root"></div>`) {
		t.Fatalf("expected rendered page to include SPA root")
	}
	if !strings.Contains(body, `/assets/`) {
		t.Fatalf("expected rendered page to reference frontend assets")
	}
}

func TestStaticAssetsAccessible(t *testing.T) {
	s := newTestServer(t)
	indexReq := httptest.NewRequest(http.MethodGet, "/", nil)
	indexRec := httptest.NewRecorder()
	s.routes().ServeHTTP(indexRec, indexReq)

	re := regexp.MustCompile(`(?:src|href)="(/assets/[^"]+)"`)
	matches := re.FindAllStringSubmatch(indexRec.Body.String(), -1)
	if len(matches) == 0 {
		t.Fatalf("expected at least one asset reference in index html")
	}

	for _, match := range matches {
		req := httptest.NewRequest(http.MethodGet, match[1], nil)
		rec := httptest.NewRecorder()
		s.routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s returned status %d", match[1], rec.Code)
		}
		if rec.Body.Len() == 0 {
			t.Fatalf("%s returned empty body", match[1])
		}
		name := path.Base(match[1])
		if strings.HasSuffix(name, ".css") && !strings.Contains(rec.Body.String(), ".app-shell") {
			t.Fatalf("%s did not look like compiled css", match[1])
		}
		if strings.HasSuffix(name, ".js") && !strings.Contains(rec.Body.String(), "TranslateGemmaUI") {
			t.Fatalf("%s did not look like compiled js", match[1])
		}
	}
}

func TestBootstrapReturnsJSONState(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/bootstrap", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var payload appStatePayload
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode bootstrap payload: %v", err)
	}
	if payload.PageTitle != "TranslateGemmaUI" {
		t.Fatalf("unexpected title: %s", payload.PageTitle)
	}
	if payload.TextSourceLang != "auto" {
		t.Fatalf("expected text source default to be auto, got %q", payload.TextSourceLang)
	}
	if len(payload.Models) == 0 {
		t.Fatalf("expected models in bootstrap payload")
	}
	if len(payload.Languages) == 0 {
		t.Fatalf("expected languages in bootstrap payload")
	}
	if payload.Languages[0].Code != "auto" {
		t.Fatalf("expected first language to be auto-detect, got %q", payload.Languages[0].Code)
	}
	if payload.Languages[0].Labels["zh-CN"] == "" {
		t.Fatalf("expected localized labels in bootstrap payload")
	}
}

func TestBootstrapReportsIdleWithoutAutoStartingRuntime(t *testing.T) {
	s := newTestServer(t)
	manager := s.runtimeManager.(*fakeRuntimeManager)
	s.probeBackend = func(string) runtime.Status {
		return runtime.Status{Ready: false, Message: "offline"}
	}
	modelPath := filepath.Join(s.dataRoot, "runtimes", "translategemma-4b-it.Q8_0.llamafile")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatalf("mkdir runtimes: %v", err)
	}
	if err := os.WriteFile(modelPath, []byte("runtime"), 0o644); err != nil {
		t.Fatalf("write runtime artifact: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/bootstrap", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var payload appStatePayload
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode bootstrap payload: %v", err)
	}
	if payload.RuntimeReady {
		t.Fatalf("expected runtime to remain idle before first translation")
	}
	if payload.RuntimeStatus != "Runtime idle" {
		t.Fatalf("expected runtime idle status, got %q", payload.RuntimeStatus)
	}
	if payload.RuntimeStatusCode != statusCodeRuntimeIdleLoadOnFirstTranslation {
		t.Fatalf("expected runtime idle status code, got %q", payload.RuntimeStatusCode)
	}
	if manager.ensureCalls != 0 {
		t.Fatalf("expected bootstrap to avoid starting runtime, got %d ensure calls", manager.ensureCalls)
	}
	for _, item := range payload.Models {
		if item.ID != "q8_0" {
			continue
		}
		if item.Active {
			t.Fatalf("expected idle runtime to keep q8_0 out of active state: %#v", item)
		}
		if !item.Selected || item.Loaded {
			t.Fatalf("expected idle runtime to keep q8_0 selected but not loaded: %#v", item)
		}
		return
	}
	t.Fatalf("expected q8_0 model in payload")
}

func TestBootstrapIncludesExpandedModelState(t *testing.T) {
	s := newTestServer(t)
	modelPath := filepath.Join(s.dataRoot, "runtimes", "translategemma-4b-it.Q8_0.llamafile")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatalf("mkdir runtimes: %v", err)
	}
	if err := os.WriteFile(modelPath, []byte("runtime"), 0o644); err != nil {
		t.Fatalf("write runtime artifact: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/bootstrap", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var payload appStatePayload
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode bootstrap payload: %v", err)
	}

	var selected modelPayload
	found := false
	for _, item := range payload.Models {
		if item.ID != "q8_0" {
			continue
		}
		selected = item
		found = true
		break
	}
	if !found {
		t.Fatalf("expected q8_0 model in payload")
	}
	if !selected.Installed || !selected.Active || !selected.Selected || !selected.Loaded {
		t.Fatalf("expected q8_0 payload to be installed/active/selected/loaded: %#v", selected)
	}
	if selected.VisionCapable {
		t.Fatalf("expected q8_0 text runtime to be non-vision: %#v", selected)
	}
	if !selected.Recommended {
		t.Fatalf("expected q8_0 to preserve recommended metadata: %#v", selected)
	}
}

func TestBootstrapIgnoresUncatalogedRuntimeFiles(t *testing.T) {
	s := newTestServer(t)
	s.probeBackend = func(string) runtime.Status {
		return runtime.Status{Ready: false, Message: "offline"}
	}
	roguePath := filepath.Join(s.dataRoot, "runtimes", "custom-runtime.llamafile")
	if err := os.MkdirAll(filepath.Dir(roguePath), 0o755); err != nil {
		t.Fatalf("mkdir runtimes: %v", err)
	}
	if err := os.WriteFile(roguePath, []byte("runtime"), 0o644); err != nil {
		t.Fatalf("write runtime artifact: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/bootstrap", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var payload appStatePayload
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode bootstrap payload: %v", err)
	}
	if !payload.NeedsModelSetup {
		t.Fatalf("expected uncataloged runtime to require model setup")
	}
	if payload.RuntimeStatus != "No local model installed" {
		t.Fatalf("expected uncataloged runtime to be ignored, got %q", payload.RuntimeStatus)
	}
	if payload.RuntimeStatusCode != statusCodeNoLocalModelSelectDownload {
		t.Fatalf("expected no-local-model runtime status code, got %q", payload.RuntimeStatusCode)
	}
}

func TestHistoryDeleteEndpoint(t *testing.T) {
	s := newTestServer(t)
	first := s.pushHistory(historyItem{
		Source: "en",
		Target: "zh-CN",
		Input:  "hello",
		Output: "你好",
		When:   s.now(),
	})
	s.pushHistory(historyItem{
		Source: "de-DE",
		Target: "en",
		Input:  "Batterie",
		Output: "Battery",
		When:   s.now().Add(time.Minute),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/history/delete", strings.NewReader("history_id="+strconv.FormatInt(first.ID, 10)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected delete success status 200, got %d", rec.Code)
	}

	var deleted historyDeleteResponse
	if err := json.NewDecoder(rec.Body).Decode(&deleted); err != nil {
		t.Fatalf("decode success response: %v", err)
	}
	if !deleted.OK {
		t.Fatalf("expected delete response OK to be true")
	}
	if deleted.StatusCode != statusCodeHistoryItemDeleted {
		t.Fatalf("expected history delete status code, got %q", deleted.StatusCode)
	}
	if deleted.Count != 1 {
		t.Fatalf("expected count 1 after delete, got %d", deleted.Count)
	}

	repeatReq := httptest.NewRequest(http.MethodPost, "/api/history/delete", strings.NewReader("history_id="+strconv.FormatInt(first.ID, 10)))
	repeatReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	repeatRec := httptest.NewRecorder()
	s.routes().ServeHTTP(repeatRec, repeatReq)

	if repeatRec.Code != http.StatusNotFound {
		t.Fatalf("expected repeated delete status 404, got %d", repeatRec.Code)
	}

	var repeated historyDeleteResponse
	if err := json.NewDecoder(repeatRec.Body).Decode(&repeated); err != nil {
		t.Fatalf("decode repeated response: %v", err)
	}
	if repeated.OK {
		t.Fatalf("expected repeated delete response OK to be false")
	}
	if repeated.StatusCode != statusCodeHistoryItemNotFound {
		t.Fatalf("expected repeated delete status code, got %q", repeated.StatusCode)
	}

	missingReq := httptest.NewRequest(http.MethodPost, "/api/history/delete", strings.NewReader(""))
	missingReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	missingRec := httptest.NewRecorder()
	s.routes().ServeHTTP(missingRec, missingReq)

	if missingRec.Code != http.StatusBadRequest {
		t.Fatalf("expected missing id status 400, got %d", missingRec.Code)
	}
}

func TestHistoryStoreOrderAndDelete(t *testing.T) {
	s := newTestServer(t)
	first := s.pushHistory(historyItem{Source: "en", Target: "zh-CN", Input: "one", Output: "一", When: s.now()})
	second := s.pushHistory(historyItem{Source: "en", Target: "zh-CN", Input: "two", Output: "二", When: s.now().Add(time.Second)})
	third := s.pushHistory(historyItem{Source: "en", Target: "zh-CN", Input: "three", Output: "三", When: s.now().Add(2 * time.Second)})

	history := s.snapshotHistory()
	if len(history) != 3 {
		t.Fatalf("expected 3 history entries, got %d", len(history))
	}
	if history[0].ID != third.ID || history[1].ID != second.ID || history[2].ID != first.ID {
		t.Fatalf("history order was not newest first")
	}

	deleted, count := s.deleteHistory(second.ID)
	if !deleted {
		t.Fatalf("expected deleteHistory to report success")
	}
	if count != 2 {
		t.Fatalf("expected count 2 after delete, got %d", count)
	}

	history = s.snapshotHistory()
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries after delete, got %d", len(history))
	}
	if history[0].ID != third.ID || history[1].ID != first.ID {
		t.Fatalf("history order after delete was wrong")
	}

	deleted, count = s.deleteHistory(9999)
	if deleted {
		t.Fatalf("expected deleteHistory to report false for missing item")
	}
	if count != 2 {
		t.Fatalf("expected count to remain 2 for missing delete, got %d", count)
	}
}

func TestHistoryClearEndpoint(t *testing.T) {
	s := newTestServer(t)
	s.pushHistory(historyItem{Source: "en", Target: "zh-CN", Input: "one", Output: "一", When: s.now()})
	s.pushHistory(historyItem{Source: "en", Target: "zh-CN", Input: "two", Output: "二", When: s.now().Add(time.Second)})

	req := httptest.NewRequest(http.MethodPost, "/api/history/clear", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected clear success status 200, got %d", rec.Code)
	}

	var payload historyDeleteResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode clear response: %v", err)
	}
	if !payload.OK {
		t.Fatalf("expected clear response OK to be true")
	}
	if payload.StatusCode != statusCodeHistoryCleared {
		t.Fatalf("expected history clear status code, got %q", payload.StatusCode)
	}
	if payload.Count != 0 {
		t.Fatalf("expected count 0 after clear, got %d", payload.Count)
	}
	if got := s.snapshotHistory(); len(got) != 0 {
		t.Fatalf("expected in-memory history to be empty after clear, got %d entries", len(got))
	}

	entries, nextID, err := config.LoadHistory(s.dataRoot)
	if err != nil {
		t.Fatalf("load persisted history: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected persisted history to be empty after clear, got %d entries", len(entries))
	}
	if nextID != 0 {
		t.Fatalf("expected next history id to reset after clear, got %d", nextID)
	}
}

func TestPushHistoryPersistsToDiskAndRestoresOnStartup(t *testing.T) {
	dataRoot := t.TempDir()
	entries := make([]config.HistoryEntry, 0, 205)
	for idx := 0; idx < 205; idx++ {
		entries = append(entries, config.HistoryEntry{
			ID:        int64(205 - idx),
			Source:    "en",
			Target:    "zh-CN",
			Input:     "input-" + strconv.Itoa(idx),
			Output:    "output-" + strconv.Itoa(idx),
			CreatedAt: time.Date(2026, time.March, 6, 9, 27, 54+idx, 0, time.UTC),
		})
	}
	if err := config.SaveHistory(dataRoot, entries, 205); err != nil {
		t.Fatalf("save seeded history: %v", err)
	}

	s, err := newServer("", dataRoot)
	if err != nil {
		t.Fatalf("newServer returned error: %v", err)
	}

	history := s.snapshotHistory()
	if len(history) != maxHistory {
		t.Fatalf("expected restored history to be trimmed to %d entries, got %d", maxHistory, len(history))
	}
	if history[0].ID != 205 {
		t.Fatalf("expected newest restored history entry first, got id %d", history[0].ID)
	}
	if history[len(history)-1].ID != 6 {
		t.Fatalf("expected restored history tail to stop at id 6, got %d", history[len(history)-1].ID)
	}

	s.pushHistory(historyItem{
		Source: "en",
		Target: "zh-CN",
		Input:  "fresh",
		Output: "新的",
		When:   time.Date(2026, time.March, 7, 8, 0, 0, 0, time.UTC),
	})

	persisted, nextID, err := config.LoadHistory(dataRoot)
	if err != nil {
		t.Fatalf("load persisted history after push: %v", err)
	}
	if len(persisted) != maxHistory {
		t.Fatalf("expected persisted history to stay capped at %d entries, got %d", maxHistory, len(persisted))
	}
	if persisted[0].ID != 206 {
		t.Fatalf("expected new history entry to be persisted first with id 206, got %d", persisted[0].ID)
	}
	if persisted[len(persisted)-1].ID != 7 {
		t.Fatalf("expected persisted history tail to stop at id 7 after new push, got %d", persisted[len(persisted)-1].ID)
	}
	if nextID != 206 {
		t.Fatalf("expected next history id 206 after push, got %d", nextID)
	}
}

func TestModelActivateReturnsErrorEventWhenModelMissing(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/models/activate", strings.NewReader("model_id=q8_0"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for streaming endpoint, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"error"`) {
		t.Fatalf("expected streaming body to include error event, got: %s", body)
	}
	if !strings.Contains(body, `"stage":"check"`) {
		t.Fatalf("expected error stage=check for missing local model, got: %s", body)
	}
}

func TestModelActivatePreloadsReplacementRuntime(t *testing.T) {
	s := newTestServer(t)
	manager := s.runtimeManager.(*fakeRuntimeManager)
	modelPath := filepath.Join(s.dataRoot, "runtimes", "translategemma-4b-it.Q8_0.llamafile")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatalf("mkdir runtimes: %v", err)
	}
	if err := os.WriteFile(modelPath, []byte("runtime"), 0o644); err != nil {
		t.Fatalf("write runtime artifact: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/models/activate", strings.NewReader("model_id=q8_0"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for streaming endpoint, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Model active") {
		t.Fatalf("expected active completion message, got: %s", body)
	}
	if manager.stopCalls != 1 {
		t.Fatalf("expected runtime stop to be called once, got %d", manager.stopCalls)
	}
	if manager.ensureCalls != 1 {
		t.Fatalf("expected activate to preload the runtime, got %d ensure calls", manager.ensureCalls)
	}
	if !strings.Contains(body, `"message":"Loading model into runtime"`) {
		t.Fatalf("expected loading progress event while preloading, got: %s", body)
	}
}

func TestModelActivateReusesAlreadyLoadedRuntime(t *testing.T) {
	s := newTestServer(t)
	manager := s.runtimeManager.(*fakeRuntimeManager)
	modelPath := filepath.Join(s.dataRoot, "runtimes", "translategemma-4b-it.Q8_0.llamafile")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatalf("mkdir runtimes: %v", err)
	}
	if err := os.WriteFile(modelPath, []byte("runtime"), 0o644); err != nil {
		t.Fatalf("write runtime artifact: %v", err)
	}
	s.state.ActiveModelPath = modelPath
	s.runtimeManager.SetPreferredModelPath(modelPath)

	req := httptest.NewRequest(http.MethodPost, "/api/models/activate", strings.NewReader("model_id=q8_0"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for streaming endpoint, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Model active") {
		t.Fatalf("expected active completion message, got: %s", body)
	}
	if manager.stopCalls != 0 {
		t.Fatalf("expected already loaded runtime to skip stop, got %d calls", manager.stopCalls)
	}
	if manager.ensureCalls != 0 {
		t.Fatalf("expected already loaded runtime to skip preload, got %d calls", manager.ensureCalls)
	}
}

func TestStopRuntimeWithEventsStopsCurrentRuntime(t *testing.T) {
	s := newTestServer(t)
	manager := s.runtimeManager.(*fakeRuntimeManager)

	if err := s.stopRuntimeWithEvents("load", nil); err != nil {
		t.Fatalf("stopRuntimeWithEvents returned error: %v", err)
	}
	if manager.stopCalls != 1 {
		t.Fatalf("expected runtime stop to be called once, got %d", manager.stopCalls)
	}
}

func TestRunHTTPServerStopsOwnedRuntimeOnShutdown(t *testing.T) {
	manager := &fakeRuntimeManager{backendURL: "http://127.0.0.1:8080", ready: true}
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runHTTPServer(ctx, server, manager, func() error {
			return server.Serve(listener)
		})
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	if err := <-done; err != nil {
		t.Fatalf("runHTTPServer returned error: %v", err)
	}
	if manager.stopOwnedCalls != 1 {
		t.Fatalf("expected owned runtime stop to be called once, got %d", manager.stopOwnedCalls)
	}
	if manager.stopCalls != 0 {
		t.Fatalf("expected shared stop path to remain unused, got %d calls", manager.stopCalls)
	}
}

func TestRunHTTPServerReturnsServeErrorAndStillCleansUpOwnedRuntime(t *testing.T) {
	manager := &fakeRuntimeManager{backendURL: "http://127.0.0.1:8080", ready: true}
	server := &http.Server{}
	serveErr := errors.New("bind failed")

	err := runHTTPServer(context.Background(), server, manager, func() error {
		return serveErr
	})
	if !errors.Is(err, serveErr) {
		t.Fatalf("expected serve error %v, got %v", serveErr, err)
	}
	if manager.stopOwnedCalls != 1 {
		t.Fatalf("expected cleanup to stop owned runtime once, got %d", manager.stopOwnedCalls)
	}
}

func TestTranslateStreamEmitsProgressAndStoresSingleHistoryEntry(t *testing.T) {
	s := newTestServer(t)
	s.translator = fakeTranslator{
		streamFn: func(ctx context.Context, req translate.Request, onDelta func(string) error, onProgress func(translate.ProgressUpdate) error) (string, error) {
			if ctx == nil {
				t.Fatalf("expected request context to be passed to translator")
			}
			if req.SourceLang != "auto" || req.TargetLang != "zh-CN" {
				t.Fatalf("unexpected request languages: %#v", req)
			}
			if err := onProgress(translate.ProgressUpdate{
				Message: "Translating part 1/2",
				Percent: 0,
			}); err != nil {
				return "", err
			}
			if err := onDelta("你好"); err != nil {
				return "", err
			}
			if err := onDelta("，世界"); err != nil {
				return "", err
			}
			return "你好，世界", nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/translate/stream", strings.NewReader("target_lang=zh-CN&input_text=Hello+world"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected NDJSON stream response")
	}

	var (
		sawProgress bool
		sawDone     bool
	)
	for _, line := range lines {
		var event streamEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode stream event %q: %v", line, err)
		}
		if event.Type == "progress" && event.Message == "Translating part 1/2" && event.Stage == "translate" {
			sawProgress = true
		}
		if event.Type == "done" {
			sawDone = true
			if event.Output != "你好，世界" {
				t.Fatalf("unexpected final output %q", event.Output)
			}
			if event.MessageCode != statusCodeTranslationCompleted {
				t.Fatalf("expected translation completed message code, got %q", event.MessageCode)
			}
			if event.Count != 1 {
				t.Fatalf("expected a single history entry, got %d", event.Count)
			}
		}
	}

	if !sawProgress {
		t.Fatalf("expected translate progress event in stream response")
	}
	if !sawDone {
		t.Fatalf("expected done event in stream response")
	}
	if got := s.historyCount(); got != 1 {
		t.Fatalf("expected one merged history entry, got %d", got)
	}
}
