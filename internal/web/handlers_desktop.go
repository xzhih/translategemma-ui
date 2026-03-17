package web

import (
	"net/http"
	"strings"
)

func (s *Server) handleDesktopShutdown(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(s.shutdownToken) == "" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if strings.TrimSpace(r.Header.Get("X-TranslateGemma-Desktop-Token")) != s.shutdownToken {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "desktop shutdown requested",
	})

	if s.requestStop != nil {
		go s.requestStop()
	}
}
