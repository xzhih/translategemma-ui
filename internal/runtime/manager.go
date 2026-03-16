package runtime

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBackendURL is the local OpenAI-compatible endpoint exposed by llamafile.
const DefaultBackendURL = "http://127.0.0.1:8080"

// Status represents runtime backend health.
type Status struct {
	Ready   bool
	Message string
}

// NormalizeBackendURL ensures the backend URL is parseable and stable.
func NormalizeBackendURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultBackendURL
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	return strings.TrimRight(raw, "/")
}

// BackendAddress extracts host and port from a backend URL.
func BackendAddress(raw string) (host, port string, err error) {
	u, err := url.Parse(NormalizeBackendURL(raw))
	if err != nil {
		return "", "", err
	}
	host = u.Host
	if strings.Contains(host, ":") {
		h, p, splitErr := net.SplitHostPort(host)
		if splitErr == nil {
			host, port = h, p
		}
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if port == "" {
		port = "8080"
	}
	return host, port, nil
}

// JoinBackendURL formats a normalized local backend URL from host and port.
func JoinBackendURL(host, port string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "127.0.0.1"
	}
	port = strings.TrimSpace(port)
	if port == "" {
		port = "8080"
	}
	return NormalizeBackendURL((&url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, port),
	}).String())
}

// CanBindBackendPort reports whether the given host:port can be bound locally.
func CanBindBackendPort(host, port string) bool {
	ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// ResolveLaunchBackendURL returns a backend URL that can be used for a new runtime.
// If the preferred URL is already serving a compatible backend, it is reused.
// If the preferred port is occupied by something else, the next available port is chosen.
func ResolveLaunchBackendURL(raw string) (string, bool, error) {
	preferred := NormalizeBackendURL(raw)
	host, port, err := BackendAddress(preferred)
	if err != nil {
		return "", false, err
	}
	if ProbeBackend(preferred).Ready {
		return preferred, false, nil
	}
	if CanBindBackendPort(host, port) {
		return preferred, false, nil
	}
	startPort, err := strconv.Atoi(port)
	if err != nil {
		return "", false, fmt.Errorf("invalid backend port %q: %w", port, err)
	}
	for offset := 1; offset <= 100; offset++ {
		candidatePort := strconv.Itoa(startPort + offset)
		if !CanBindBackendPort(host, candidatePort) {
			continue
		}
		return JoinBackendURL(host, candidatePort), true, nil
	}
	return "", false, fmt.Errorf("no available backend port found near %s", preferred)
}

// ProbeBackend checks if the local backend is reachable.
func ProbeBackend(raw string) Status {
	base := NormalizeBackendURL(raw)
	client := &http.Client{Timeout: 2 * time.Second}

	type probe struct {
		Endpoint string
		MinCode  int
		MaxCode  int
	}
	endpoints := []probe{
		{Endpoint: "/v1/models", MinCode: 200, MaxCode: 299},
		{Endpoint: "/healthz", MinCode: 200, MaxCode: 299},
		{Endpoint: "/", MinCode: 200, MaxCode: 299},
	}
	for _, ep := range endpoints {
		req, err := http.NewRequest(http.MethodGet, base+ep.Endpoint, nil)
		if err != nil {
			continue
		}
		if ep.Endpoint == "/v1/models" {
			req.Header.Set("Authorization", "Bearer no-key")
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= ep.MinCode && resp.StatusCode <= ep.MaxCode {
			return Status{Ready: true, Message: fmt.Sprintf("backend reachable at %s%s", base, ep.Endpoint)}
		}
	}
	return Status{Ready: false, Message: fmt.Sprintf("backend unreachable at %s", base)}
}
