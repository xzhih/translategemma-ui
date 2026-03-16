package web

import (
	"encoding/json"
	"net/http"
	"os"

	"translategemma-ui/internal/config"
)

func (s *Server) viewData(source, target, inputText, textInstruction, outputText string, runtimeReady bool, runtimeStatus uiStatus) appStatePayload {
	history := s.snapshotHistory()
	status := s.getStatus()
	return appStatePayload{
		PageTitle:         "TranslateGemmaUI",
		ActiveTab:         "text",
		TextSourceLang:    source,
		TextTargetLang:    target,
		TextInstruction:   textInstruction,
		TextInput:         inputText,
		TextOutput:        outputText,
		FileSourceLang:    "auto",
		FileTargetLang:    "zh-CN",
		FileInstruction:   "",
		FileOutput:        "",
		Status:            status.Message,
		StatusCode:        status.Code,
		ActiveModelID:     s.cfg.ActiveModelID,
		Models:            s.modelPayloads(runtimeReady),
		Languages:         s.languages,
		History:           historyToPayloads(history),
		HistoryCount:      len(history),
		RuntimeStatus:     runtimeStatus.Message,
		RuntimeStatusCode: runtimeStatus.Code,
		RuntimeReady:      runtimeReady,
		NeedsModelSetup:   !runtimeReady && !s.hasInstalledModel(),
		VisionEnabled:     s.visionEnabled(),
		MaxUploadMB:       maxImageUploadMB,
		Now:               s.now().Format("15:04"),
	}
}

func (s *Server) pushHistory(item historyItem) historyItem {
	s.mu.Lock()
	s.nextHistoryID++
	item.ID = s.nextHistoryID
	s.history = append([]historyItem{item}, s.history...)
	if len(s.history) > maxHistory {
		s.history = s.history[:maxHistory]
	}
	err := s.persistHistoryLocked()
	s.mu.Unlock()
	if err != nil {
		s.setStatus(err.Error())
	}
	return item
}

func (s *Server) deleteHistory(id int64) (bool, int) {
	s.mu.Lock()
	for idx := range s.history {
		if s.history[idx].ID != id {
			continue
		}
		s.history = append(s.history[:idx], s.history[idx+1:]...)
		err := s.persistHistoryLocked()
		count := len(s.history)
		s.mu.Unlock()
		if err != nil {
			s.setStatus(err.Error())
		}
		return true, count
	}
	count := len(s.history)
	s.mu.Unlock()
	return false, count
}

func (s *Server) clearHistory() int {
	s.mu.Lock()
	s.history = nil
	s.nextHistoryID = 0
	err := s.persistHistoryLocked()
	count := len(s.history)
	s.mu.Unlock()
	if err != nil {
		s.setStatus(err.Error())
	}
	return count
}

func (s *Server) snapshotHistory() []historyItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]historyItem, len(s.history))
	copy(out, s.history)
	return out
}

func (s *Server) historyCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.history)
}

func historyToPayload(item historyItem) *historyPayload {
	return &historyPayload{
		ID:     item.ID,
		Source: item.Source,
		Target: item.Target,
		Input:  item.Input,
		Output: item.Output,
		When:   item.When.Format("15:04:05"),
	}
}

func historyToPayloads(items []historyItem) []historyPayload {
	out := make([]historyPayload, 0, len(items))
	for _, item := range items {
		out = append(out, *historyToPayload(item))
	}
	return out
}

func loadPersistedHistory(root string) ([]historyItem, int64, error) {
	entries, nextID, err := config.LoadHistory(root)
	if err != nil {
		return nil, 0, err
	}
	if len(entries) > maxHistory {
		entries = entries[:maxHistory]
	}
	items := make([]historyItem, 0, len(entries))
	for _, entry := range entries {
		items = append(items, historyItem{
			ID:     entry.ID,
			Source: entry.Source,
			Target: entry.Target,
			Input:  entry.Input,
			Output: entry.Output,
			When:   entry.CreatedAt,
		})
	}
	if len(items) > 0 && nextID < items[0].ID {
		nextID = items[0].ID
	}
	return items, nextID, nil
}

func (s *Server) persistHistoryLocked() error {
	entries := make([]config.HistoryEntry, 0, len(s.history))
	for _, item := range s.history {
		entries = append(entries, config.HistoryEntry{
			ID:        item.ID,
			Source:    item.Source,
			Target:    item.Target,
			Input:     item.Input,
			Output:    item.Output,
			CreatedAt: item.When,
		})
	}
	if err := config.SaveHistory(s.dataRoot, entries, s.nextHistoryID); err != nil {
		return err
	}
	return nil
}

func (s *Server) setStatus(msg string) {
	s.setStatusCode("", msg)
}

func (s *Server) setStatusCode(code, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = uiStatus{Code: code, Message: msg}
}

func (s *Server) getStatus() uiStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *Server) render(w http.ResponseWriter, state appStatePayload) {
	_ = state
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(s.uiShell)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
