package web

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"translategemma-ui/internal/config"
	"translategemma-ui/internal/huggingface"
	"translategemma-ui/internal/models"
	"translategemma-ui/internal/modelstore"
	"translategemma-ui/internal/runtime"
	lf "translategemma-ui/internal/runtime/llamafile"
	"translategemma-ui/internal/translate"
)

func (s *Server) handleModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form payload", http.StatusBadRequest)
		return
	}

	item, ok := s.findModelByID(strings.TrimSpace(r.FormValue("model_id")))
	if !ok {
		s.setStatusCode(statusCodeUnknownModelSelection, "Unknown model selection")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if _, err := s.activateModel(item, nil); err != nil {
		s.setStatus(err.Error())
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleModelInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form payload", http.StatusBadRequest)
		return
	}

	item, ok := s.findModelByID(strings.TrimSpace(r.FormValue("model_id")))
	if !ok {
		http.Error(w, "unknown model_id", http.StatusBadRequest)
		return
	}

	writeEvent, err := prepareStreamWriter(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.installMu.Lock()
	defer s.installMu.Unlock()

	_ = writeEvent(streamEvent{Type: "status", Stage: "check", Message: "Preparing model install", MessageCode: statusCodePreparingModelInstall, Percent: 0})
	modelPath, err := huggingface.DownloadModel(s.dataRoot, item, func(p huggingface.DownloadProgress) {
		_ = writeEvent(streamEvent{Type: "progress", Stage: "download", Message: p.Message, Percent: p.Percent})
	})
	if err != nil {
		s.setStatus(err.Error())
		_ = writeEvent(streamEvent{Type: "error", Stage: "download", Message: err.Error()})
		return
	}

	if err := s.applyActiveModelState(item, modelPath); err != nil {
		s.setStatus(err.Error())
		_ = writeEvent(streamEvent{Type: "error", Stage: "save", Message: err.Error()})
		return
	}

	if err := s.stopRuntimeWithEvents("load", writeEvent); err != nil {
		return
	}

	msg := "Model installed. Runtime will load on first translation."
	s.setStatusCode(statusCodeModelInstalledLoadOnFirstTranslation, msg)
	_ = writeEvent(streamEvent{Type: "done", Stage: "ready", Message: msg, MessageCode: statusCodeModelInstalledLoadOnFirstTranslation, Percent: 100})
}

func (s *Server) handleModelActivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form payload", http.StatusBadRequest)
		return
	}

	item, ok := s.findModelByID(strings.TrimSpace(r.FormValue("model_id")))
	if !ok {
		http.Error(w, "unknown model_id", http.StatusBadRequest)
		return
	}

	writeEvent, err := prepareStreamWriter(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.installMu.Lock()
	defer s.installMu.Unlock()

	_ = writeEvent(streamEvent{Type: "status", Stage: "check", Message: "Switching active model", MessageCode: statusCodeSwitchingActiveModel, Percent: 0})
	msg, err := s.activateModel(item, writeEvent)
	if err != nil {
		return
	}

	messageCode := statusCodeModelActive
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(item.ID)), "_vision") {
		messageCode = statusCodeVisionRuntimeActive
	}
	_ = writeEvent(streamEvent{Type: "done", Stage: "ready", Message: msg, MessageCode: messageCode, Percent: 100})
}

func (s *Server) handleModelDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form payload", http.StatusBadRequest)
		return
	}

	item, ok := s.findModelByID(strings.TrimSpace(r.FormValue("model_id")))
	if !ok {
		http.Error(w, "unknown model_id", http.StatusBadRequest)
		return
	}

	writeEvent, err := prepareStreamWriter(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.installMu.Lock()
	defer s.installMu.Unlock()

	_ = writeEvent(streamEvent{Type: "status", Stage: "delete", Message: "Removing local model", MessageCode: statusCodeRemovingLocalModel, Percent: 10})
	_, deleted, err := modelstore.DeleteModel(s.dataRoot, item, &s.state)
	if err != nil {
		s.setStatus(err.Error())
		_ = writeEvent(streamEvent{Type: "error", Stage: "delete", Message: err.Error()})
		return
	}
	if !deleted {
		msg := "Model is not installed locally"
		s.setStatusCode(statusCodeModelNotInstalledLocally, msg)
		_ = writeEvent(streamEvent{Type: "done", Stage: "delete", Message: msg, MessageCode: statusCodeModelNotInstalledLocally, Percent: 100})
		return
	}

	if s.cfg.ActiveModelID == item.ID && s.state.ActiveModelPath == "" {
		s.cfg.ActiveModelID = ""
	}
	if err := config.SaveAppConfig(s.dataRoot, s.cfg); err != nil {
		s.setStatus(err.Error())
		_ = writeEvent(streamEvent{Type: "error", Stage: "delete", Message: err.Error()})
		return
	}
	if err := config.SaveAppState(s.dataRoot, s.state); err != nil {
		s.setStatus(err.Error())
		_ = writeEvent(streamEvent{Type: "error", Stage: "delete", Message: err.Error()})
		return
	}

	s.activeModel = resolveActiveModel(s.availableModels, s.cfg.ActiveModelID)
	s.applyLoadedState()
	_ = s.runtimeManager.Stop()
	msg := "Model deleted"
	s.setStatusCode(statusCodeModelDeleted, msg)
	_ = writeEvent(streamEvent{Type: "done", Stage: "delete", Message: msg, MessageCode: statusCodeModelDeleted, Percent: 100})
}

func (s *Server) handleEnableVision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeEvent, err := prepareStreamWriter(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.installMu.Lock()
	defer s.installMu.Unlock()

	visionModel, ok := huggingface.RecommendedVisionRuntime()
	if !ok {
		_ = writeEvent(streamEvent{Type: "error", Message: "q8_0_vision runtime is not available", MessageCode: statusCodeVisionRuntimeUnavailable})
		return
	}

	if s.localModelPath(visionModel.FileName) == "" {
		_ = writeEvent(streamEvent{Type: "status", Stage: "download", Message: "Downloading q8_0_vision runtime", MessageCode: statusCodeDownloadingVisionRuntime, Percent: 0})
		modelPath, err := huggingface.DownloadModel(s.dataRoot, visionModel, func(p huggingface.DownloadProgress) {
			_ = writeEvent(streamEvent{Type: "progress", Stage: "download", Message: p.Message, Percent: p.Percent})
		})
		if err != nil {
			s.setStatus(err.Error())
			_ = writeEvent(streamEvent{Type: "error", Stage: "download", Message: err.Error()})
			return
		}
		if err := s.applyActiveModelState(visionModel, modelPath); err != nil {
			s.setStatus(err.Error())
			_ = writeEvent(streamEvent{Type: "error", Stage: "save", Message: err.Error()})
			return
		}
		if err := s.stopRuntimeWithEvents("load", writeEvent); err != nil {
			return
		}
		msg, err := s.preloadSelectedModel(visionModel, writeEvent)
		if err != nil {
			return
		}
		_ = writeEvent(streamEvent{Type: "done", Stage: "ready", Message: msg, MessageCode: statusCodeVisionRuntimeActive, Percent: 100})
		return
	}

	msg, err := s.activateModel(visionModel, writeEvent)
	if err != nil {
		return
	}
	s.setStatusCode(statusCodeVisionRuntimeActive, msg)
	_ = writeEvent(streamEvent{Type: "done", Stage: "ready", Message: msg, MessageCode: statusCodeVisionRuntimeActive, Percent: 100})
}

func (s *Server) activateModel(item models.QuantizedModel, emit func(streamEvent) error) (string, error) {
	modelPath := s.localModelPath(item.FileName)
	if modelPath == "" {
		err := errors.New("model is not installed locally")
		if emit != nil {
			_ = emit(streamEvent{Type: "error", Stage: "check", Message: err.Error(), MessageCode: statusCodeModelNotInstalledLocally})
		}
		return "", err
	}

	if emit != nil {
		_ = emit(streamEvent{Type: "progress", Stage: "load", Message: "Preparing active model", MessageCode: statusCodePreparingActiveModel, Percent: 5})
	}

	if err := s.applyActiveModelState(item, modelPath); err != nil {
		if emit != nil {
			_ = emit(streamEvent{Type: "error", Stage: "save", Message: err.Error()})
		}
		return "", err
	}
	if err := s.stopRuntimeWithEvents("load", emit); err != nil {
		return "", err
	}
	return s.preloadSelectedModel(item, emit)
}

func (s *Server) preloadSelectedModel(item models.QuantizedModel, emit func(streamEvent) error) (string, error) {
	_, err := s.ensureRuntime(func(p lf.Progress) {
		if emit == nil {
			return
		}
		_ = emit(streamEvent{Type: "progress", Stage: p.Stage, Message: p.Message, Percent: p.Percent})
	})
	if err != nil {
		s.setStatus(err.Error())
		if emit != nil {
			_ = emit(streamEvent{Type: "error", Stage: "load", Message: err.Error()})
		}
		return "", err
	}
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(item.ID)), "_vision") {
		msg := "Vision runtime active"
		s.setStatusCode(statusCodeVisionRuntimeActive, msg)
		return msg, nil
	}
	msg := "Model active"
	s.setStatusCode(statusCodeModelActive, msg)
	return msg, nil
}

func (s *Server) applyLoadedState() {
	cfgChanged := false
	if s.state.ActiveModelPath != "" && runtimeModeForPath(s.state.ActiveModelPath) != "" && fileExists(s.state.ActiveModelPath) {
		s.state.RuntimeMode = runtimeModeForPath(s.state.ActiveModelPath)
		s.runtimeManager.SetPreferredModelPath(s.state.ActiveModelPath)
	} else if p := s.localModelPath(s.activeModel.FileName); p != "" {
		s.state.ActiveModelPath = p
		s.state.RuntimeMode = runtimeModeForPath(p)
		s.runtimeManager.SetPreferredModelPath(p)
	} else if item, ok := s.firstInstalledModel(); ok {
		s.activeModel = item.QuantizedModel
		s.cfg.ActiveModelID = item.ID
		cfgChanged = true
		s.state.ActiveModelPath = item.Path
		s.state.RuntimeMode = runtimeModeForPath(item.Path)
		s.runtimeManager.SetPreferredModelPath(item.Path)
	} else {
		s.state.ActiveModelPath = ""
		s.state.RuntimeMode = ""
		s.runtimeManager.SetPreferredModelPath("")
	}

	if cfgChanged {
		if err := config.SaveAppConfig(s.dataRoot, s.cfg); err != nil {
			s.setStatus(err.Error())
		}
	}
	if err := config.SaveAppState(s.dataRoot, s.state); err != nil {
		s.setStatus(err.Error())
	}
}

func (s *Server) ensureRuntime(onProgress func(lf.Progress)) (runtime.Status, error) {
	if probe := s.probeBackend(s.backendURL); probe.Ready {
		s.syncBackendURL(s.runtimeManager.CurrentBackendURL(), false)
		return probe, nil
	}
	if !s.hasInstalledModel() {
		return runtime.Status{Ready: false, Message: errNoLocalModel.Error()}, errNoLocalModel
	}
	status, err := s.runtimeManager.EnsureRunningWithProgress(onProgress)
	s.syncBackendURL(s.runtimeManager.CurrentBackendURL(), true)
	if err != nil {
		return status, err
	}
	return status, nil
}

func (s *Server) stopRuntimeWithEvents(errorStage string, emit func(streamEvent) error) error {
	if emit != nil {
		_ = emit(streamEvent{Type: "progress", Stage: "stop", Message: "Stopping current runtime", Percent: 2})
	}
	if err := s.runtimeManager.Stop(); err != nil {
		s.setStatus(err.Error())
		if emit != nil {
			_ = emit(streamEvent{Type: "error", Stage: errorStage, Message: err.Error()})
		}
		return err
	}
	return nil
}

func (s *Server) findModelByID(id string) (models.QuantizedModel, bool) {
	for _, item := range s.availableModels {
		if item.ID == id {
			return item, true
		}
	}
	return models.QuantizedModel{}, false
}

func (s *Server) localModelPath(fileName string) string {
	return modelstore.LocalModelPath(s.dataRoot, fileName)
}

func (s *Server) firstInstalledModel() (modelstore.CatalogItem, bool) {
	catalog := modelstore.Catalog(s.dataRoot, s.availableModels, strings.TrimSpace(s.cfg.ActiveModelID), strings.TrimSpace(s.state.ActiveModelPath))
	for _, item := range catalog {
		if item.Installed {
			return item, true
		}
	}
	return modelstore.CatalogItem{}, false
}

func (s *Server) hasInstalledModel() bool {
	_, ok := s.firstInstalledModel()
	return ok
}

func (s *Server) visionEnabled() bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(s.activeModel.ID)), "_vision") ||
		strings.Contains(strings.ToLower(filepath.Base(strings.TrimSpace(s.state.ActiveModelPath))), ".mmproj-")
}

func (s *Server) runtimeView() (bool, uiStatus) {
	probe := s.probeBackend(s.backendURL)
	if probe.Ready {
		return true, makeRuntimeStatus(statusCodeRuntimeReady, "Runtime ready")
	}
	if !s.hasInstalledModel() {
		return false, makeRuntimeStatus(statusCodeNoLocalModelSelectDownload, "No local model installed")
	}
	return false, makeRuntimeStatus(statusCodeRuntimeIdleLoadOnFirstTranslation, "Runtime idle")
}

func (s *Server) syncBackendURL(next string, persist bool) {
	next = runtime.NormalizeBackendURL(next)
	if next == "" {
		next = runtime.NormalizeBackendURL(runtime.DefaultBackendURL)
	}
	s.backendURL = next
	s.state.BackendURL = next
	if updater, ok := s.translator.(interface{ SetBackendURL(string) }); ok {
		updater.SetBackendURL(next)
	} else if s.translator == nil {
		s.translator = translate.NewService(next)
	}
	if persist {
		if err := config.SaveAppState(s.dataRoot, s.state); err != nil {
			s.setStatus(err.Error())
		}
	}
}

func (s *Server) modelPayloads(runtimeReady bool) []modelPayload {
	catalog := modelstore.Catalog(s.dataRoot, s.availableModels, strings.TrimSpace(s.cfg.ActiveModelID), strings.TrimSpace(s.state.ActiveModelPath))
	out := make([]modelPayload, 0, len(catalog))
	for _, item := range catalog {
		selected := item.Active
		loaded := selected && runtimeReady
		out = append(out, modelPayload{
			ID:            item.ID,
			FileName:      item.FileName,
			Size:          item.Size,
			Installed:     item.Installed,
			Active:        loaded,
			Selected:      selected,
			Loaded:        loaded,
			VisionCapable: modelSupportsVision(item.QuantizedModel),
			Recommended:   item.Recommended,
		})
	}
	return out
}

func modelSupportsVision(item models.QuantizedModel) bool {
	lowerID := strings.ToLower(strings.TrimSpace(item.ID))
	lowerFile := strings.ToLower(strings.TrimSpace(item.FileName))
	return strings.HasSuffix(lowerID, "_vision") || strings.Contains(lowerFile, ".mmproj-")
}

func (s *Server) upsertArtifact(next config.InstalledArtifact) {
	for i := range s.state.Artifacts {
		if s.state.Artifacts[i].Kind == next.Kind && s.state.Artifacts[i].ID == next.ID {
			s.state.Artifacts[i] = next
			return
		}
	}
	s.state.Artifacts = append(s.state.Artifacts, next)
}

func (s *Server) applyActiveModelState(item models.QuantizedModel, modelPath string) error {
	s.activeModel = item
	s.cfg.ActiveModelID = item.ID
	s.state.ActiveModelPath = modelPath
	s.state.RuntimeMode = runtimeModeForPath(modelPath)
	s.runtimeManager.SetPreferredModelPath(modelPath)
	s.upsertArtifact(config.InstalledArtifact{
		Kind:      "model",
		ID:        item.ID,
		FileName:  filepath.Base(modelPath),
		Path:      modelPath,
		SizeBytes: item.SizeBytes,
	})
	if err := config.SaveAppConfig(s.dataRoot, s.cfg); err != nil {
		return err
	}
	if err := config.SaveAppState(s.dataRoot, s.state); err != nil {
		return err
	}
	return nil
}

func runtimeModeForPath(path string) string {
	if strings.Contains(strings.ToLower(path), ".llamafile") {
		return "single_file_llamafile"
	}
	return ""
}
