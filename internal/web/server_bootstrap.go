package web

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"translategemma-ui/internal/config"
	"translategemma-ui/internal/huggingface"
	"translategemma-ui/internal/languages"
	"translategemma-ui/internal/models"
	"translategemma-ui/internal/runtime"
	lf "translategemma-ui/internal/runtime/llamafile"
	"translategemma-ui/internal/translate"
)

func newServer(modelID, dataRoot string) (*Server, error) {
	uiShell, err := loadIndexHTML()
	if err != nil {
		return nil, err
	}
	static, err := newStaticHandler()
	if err != nil {
		return nil, err
	}

	modelsList := huggingface.ListTranslateGemmaModels()
	cfg, _ := config.LoadAppConfig(dataRoot)
	state, _ := config.LoadAppState(dataRoot)
	backendURL := runtime.NormalizeBackendURL(state.BackendURL)
	if strings.TrimSpace(state.BackendURL) == "" {
		backendURL = runtime.NormalizeBackendURL(runtime.DefaultBackendURL)
	}
	runtimeManager := lf.NewManager(dataRoot, backendURL)

	activeID := strings.TrimSpace(modelID)
	if activeID == "" {
		activeID = strings.TrimSpace(cfg.ActiveModelID)
	}
	active := resolveActiveModel(modelsList, activeID)

	s := &Server{
		uiShell:         uiShell,
		static:          static,
		translator:      translate.NewService(backendURL),
		runtimeManager:  runtimeManager,
		backendURL:      backendURL,
		dataRoot:        dataRoot,
		availableModels: modelsList,
		activeModel:     active,
		languages:       supportedLanguages(),
		probeBackend:    runtime.ProbeBackend,
		now:             time.Now,
		cfg:             cfg,
		state:           state,
		status:          makeServerStatus(statusCodeReady, "Ready"),
	}
	if history, nextID, err := loadPersistedHistory(dataRoot); err == nil {
		s.history = history
		s.nextHistoryID = nextID
	} else {
		fmt.Fprintf(os.Stderr, "warning: unable to load history: %v\n", err)
	}
	s.applyLoadedState()
	s.syncBackendURL(s.runtimeManager.CurrentBackendURL(), false)
	if s.activeModel.ID != "" && s.activeModel.ID != s.cfg.ActiveModelID {
		s.cfg.ActiveModelID = s.activeModel.ID
		_ = config.SaveAppConfig(dataRoot, s.cfg)
	}

	if probe := s.probeBackend(backendURL); probe.Ready {
		s.setStatusCode(statusCodeRuntimeReady, "Runtime ready")
		return s, nil
	}
	if !s.hasInstalledModel() {
		s.setStatusCode(statusCodeNoLocalModelSelectDownload, "No local model installed. Select a model to download.")
		return s, nil
	}

	s.setStatusCode(statusCodeRuntimeIdleLoadOnFirstTranslation, "Runtime idle. The active model will load on first translation.")
	return s, nil
}

func resolveActiveModel(available []models.QuantizedModel, modelID string) models.QuantizedModel {
	if len(available) == 0 {
		return models.QuantizedModel{}
	}
	if modelID == "" {
		for _, item := range available {
			if item.Recommended && !strings.HasSuffix(item.ID, "_vision") {
				return item
			}
		}
		return available[0]
	}
	for _, m := range available {
		if m.ID == modelID {
			return m
		}
	}
	return available[0]
}

func supportedLanguages() []languageOption {
	return languages.Supported()
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets/", s.static))
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/translate", s.handleTranslate)
	mux.HandleFunc("/model", s.handleModel)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/api/bootstrap", s.handleBootstrap)
	mux.HandleFunc("/api/translate/stream", s.handleTranslateStream)
	mux.HandleFunc("/api/translate/image", s.handleImageTranslate)
	mux.HandleFunc("/api/models/install", s.handleModelInstall)
	mux.HandleFunc("/api/models/activate", s.handleModelActivate)
	mux.HandleFunc("/api/models/delete", s.handleModelDelete)
	mux.HandleFunc("/api/models/enable-vision", s.handleEnableVision)
	mux.HandleFunc("/api/history/delete", s.handleHistoryDelete)
	mux.HandleFunc("/api/history/clear", s.handleHistoryClear)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	runtimeReady, runtimeStatus := s.runtimeView()
	s.render(w, s.viewData("auto", "zh-CN", "", "", "", runtimeReady, runtimeStatus))
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	runtimeReady, runtimeStatus := s.runtimeView()
	writeJSON(w, http.StatusOK, s.viewData("auto", "zh-CN", "", "", "", runtimeReady, runtimeStatus))
}

// Run starts the local web server implementation.
func Run(listen, modelID, dataRoot string) error {
	s, err := newServer(modelID, dataRoot)
	if err != nil {
		return err
	}
	fmt.Printf("Web UI listening on http://%s\n", listen)
	return http.ListenAndServe(listen, s.routes())
}
