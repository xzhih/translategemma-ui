package runtime

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveLaunchBackendURLKeepsFreePort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on ephemeral port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	resolved, switched, err := ResolveLaunchBackendURL("http://" + addr)
	if err != nil {
		t.Fatalf("ResolveLaunchBackendURL returned error: %v", err)
	}
	if switched {
		t.Fatalf("expected free port to be kept, got switched URL %q", resolved)
	}
	if resolved != NormalizeBackendURL("http://"+addr) {
		t.Fatalf("expected resolved URL %q, got %q", NormalizeBackendURL("http://"+addr), resolved)
	}
}

func TestResolveLaunchBackendURLSwitchesWhenPortOccupied(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on occupied port: %v", err)
	}
	defer ln.Close()

	occupiedURL := "http://" + ln.Addr().String()
	resolved, switched, err := ResolveLaunchBackendURL(occupiedURL)
	if err != nil {
		t.Fatalf("ResolveLaunchBackendURL returned error: %v", err)
	}
	if !switched {
		t.Fatalf("expected occupied port to trigger a switch")
	}
	if resolved == NormalizeBackendURL(occupiedURL) {
		t.Fatalf("expected a different backend URL than %q", occupiedURL)
	}
}

func TestResolveLaunchBackendURLSwitchesWhenPortResponds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	resolved, switched, err := ResolveLaunchBackendURL(server.URL)
	if err != nil {
		t.Fatalf("ResolveLaunchBackendURL returned error: %v", err)
	}
	if !switched {
		t.Fatalf("expected responding port to trigger a switch")
	}
	if resolved == NormalizeBackendURL(server.URL) {
		t.Fatalf("expected a different backend URL than %q", server.URL)
	}
}

func TestProbeBackendRejectsRootOnlyServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	if status := ProbeBackend(server.URL); status.Ready {
		t.Fatalf("expected root-only server to be ignored, got %#v", status)
	}
}

func TestProbeBackendAcceptsModelsEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	if status := ProbeBackend(server.URL); !status.Ready {
		t.Fatalf("expected OpenAI-compatible endpoint to be accepted, got %#v", status)
	}
}
