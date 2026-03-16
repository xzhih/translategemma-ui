package runtime

import (
	"net"
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
