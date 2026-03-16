package web

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleHistoryDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, historyDeleteResponse{
			OK:         false,
			Count:      s.historyCount(),
			Status:     "invalid form payload",
			StatusCode: statusCodeInvalidFormPayload,
		})
		return
	}

	rawID := strings.TrimSpace(r.FormValue("history_id"))
	if rawID == "" {
		writeJSON(w, http.StatusBadRequest, historyDeleteResponse{
			OK:         false,
			Count:      s.historyCount(),
			Status:     "missing history_id",
			StatusCode: statusCodeMissingHistoryID,
		})
		return
	}

	historyID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, historyDeleteResponse{
			OK:         false,
			HistoryID:  0,
			Count:      s.historyCount(),
			Status:     "invalid history_id",
			StatusCode: statusCodeInvalidHistoryID,
		})
		return
	}

	deleted, count := s.deleteHistory(historyID)
	if !deleted {
		writeJSON(w, http.StatusNotFound, historyDeleteResponse{
			OK:         false,
			HistoryID:  historyID,
			Count:      count,
			Status:     "history item not found",
			StatusCode: statusCodeHistoryItemNotFound,
		})
		return
	}

	s.setStatusCode(statusCodeHistoryItemDeleted, "History item deleted")
	writeJSON(w, http.StatusOK, historyDeleteResponse{
		OK:         true,
		HistoryID:  historyID,
		Count:      count,
		Status:     "History item deleted",
		StatusCode: statusCodeHistoryItemDeleted,
	})
}

func (s *Server) handleHistoryClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	count := s.clearHistory()
	s.setStatusCode(statusCodeHistoryCleared, "History cleared")
	writeJSON(w, http.StatusOK, historyDeleteResponse{
		OK:         true,
		Count:      count,
		Status:     "History cleared",
		StatusCode: statusCodeHistoryCleared,
	})
}
