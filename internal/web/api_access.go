package web

import (
	"net/http"
	"strings"
)

func withTranslationAPIAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isTranslationAPIPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		headers := w.Header()
		headers.Add("Vary", "Origin")
		headers.Add("Vary", "Access-Control-Request-Method")
		headers.Add("Vary", "Access-Control-Request-Headers")
		headers.Add("Vary", "Access-Control-Request-Private-Network")
		headers.Set("Access-Control-Allow-Origin", "*")
		headers.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		headers.Set("Access-Control-Max-Age", "600")
		if requestedHeaders := strings.TrimSpace(r.Header.Get("Access-Control-Request-Headers")); requestedHeaders != "" {
			headers.Set("Access-Control-Allow-Headers", requestedHeaders)
		} else {
			headers.Set("Access-Control-Allow-Headers", "Content-Type, Accept")
		}
		if strings.EqualFold(strings.TrimSpace(r.Header.Get("Access-Control-Request-Private-Network")), "true") {
			headers.Set("Access-Control-Allow-Private-Network", "true")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isTranslationAPIPath(path string) bool {
	return path == "/healthz" || path == "/api/translate" || strings.HasPrefix(path, "/api/translate/")
}
