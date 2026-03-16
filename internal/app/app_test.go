package app

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	appversion "translategemma-ui/internal/version"
)

func TestRunVersionFlag(t *testing.T) {
	prevVersion := appversion.Version
	prevCommit := appversion.Commit
	prevDate := appversion.Date
	appversion.Version = "v0.1.0"
	appversion.Commit = "abc1234"
	appversion.Date = "2026-03-16T00:00:00Z"
	t.Cleanup(func() {
		appversion.Version = prevVersion
		appversion.Commit = prevCommit
		appversion.Date = prevDate
	})

	output := captureStdout(t, func() error {
		return Run([]string{"--version"})
	})

	expected := "translategemma-ui version=v0.1.0 commit=abc1234 date=2026-03-16T00:00:00Z"
	if strings.TrimSpace(output) != expected {
		t.Fatalf("expected %q, got %q", expected, strings.TrimSpace(output))
	}
}

func captureStdout(t *testing.T, fn func() error) string {
	t.Helper()

	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = w

	runErr := fn()

	_ = w.Close()
	os.Stdout = originalStdout

	if runErr != nil {
		t.Fatalf("run returned error: %v", runErr)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	_ = r.Close()
	return buf.String()
}
