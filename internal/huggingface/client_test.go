package huggingface

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestWriteDownloadedArtifactReportsDetailedProgress(t *testing.T) {
	t.Parallel()

	dstPath := filepath.Join(t.TempDir(), "runtime.llamafile")
	payload := bytes.Repeat([]byte("a"), 4096)
	var events []DownloadProgress

	gotPath, err := writeDownloadedArtifact(context.Background(), bytes.NewReader(payload), dstPath, int64(len(payload)), int64(len(payload)), func(p DownloadProgress) {
		events = append(events, p)
	})
	if err != nil {
		t.Fatalf("writeDownloadedArtifact returned error: %v", err)
	}
	if gotPath != dstPath {
		t.Fatalf("expected destination path %q, got %q", dstPath, gotPath)
	}
	if len(events) < 2 {
		t.Fatalf("expected multiple progress events, got %d", len(events))
	}

	last := events[len(events)-1]
	if last.Downloaded != int64(len(payload)) {
		t.Fatalf("expected downloaded bytes %d, got %d", len(payload), last.Downloaded)
	}
	if last.Total != int64(len(payload)) {
		t.Fatalf("expected total bytes %d, got %d", len(payload), last.Total)
	}
	if last.Percent != 100 {
		t.Fatalf("expected final percent 100, got %.2f", last.Percent)
	}
	if last.Message != "Download completed" {
		t.Fatalf("expected final message %q, got %q", "Download completed", last.Message)
	}
}

func TestWriteDownloadedArtifactRemovesPartialOnCancel(t *testing.T) {
	t.Parallel()

	dstPath := filepath.Join(t.TempDir(), "runtime.llamafile")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := writeDownloadedArtifact(ctx, bytes.NewReader([]byte("partial")), dstPath, 7, 7, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if fileExists(dstPath + ".partial") {
		t.Fatalf("expected partial file to be removed after cancellation")
	}
	if fileExists(dstPath) {
		t.Fatalf("expected destination file to not exist after cancellation")
	}
}
