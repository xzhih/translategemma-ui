package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleDesktopShutdownRequiresConfiguredToken(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodPost, "/api/desktop/shutdown", nil)
	rec := httptest.NewRecorder()
	s.handleDesktopShutdown(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 without desktop token, got %d", rec.Code)
	}
}

func TestHandleDesktopShutdownRequestsStop(t *testing.T) {
	stopped := make(chan struct{}, 1)
	s := &Server{
		shutdownToken: "desktop-test-token",
		requestStop: func() {
			stopped <- struct{}{}
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/desktop/shutdown", nil)
	req.Header.Set("X-TranslateGemma-Desktop-Token", "desktop-test-token")
	rec := httptest.NewRecorder()
	s.handleDesktopShutdown(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("expected desktop shutdown to request server stop")
	}
}
