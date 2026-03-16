package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"translategemma-ui/internal/models"
	"translategemma-ui/internal/translate"
)

func TestRunTranslateTextStopsOwnedRuntimeAfterSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/translate" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"你好"}}]}`))
	}))
	defer server.Close()

	prevPrepare := prepareTranslationRuntimeFn
	prevPrint := printTranslateResultFn
	t.Cleanup(func() {
		prepareTranslationRuntimeFn = prevPrepare
		printTranslateResultFn = prevPrint
	})

	cleanupCalls := 0
	printedOutput := ""
	prepareTranslationRuntimeFn = func(string, bool) (string, *translate.Service, models.QuantizedModel, func() error, error) {
		return t.TempDir(), translate.NewService(server.URL), models.QuantizedModel{ID: "q8_0"}, func() error {
			cleanupCalls++
			return nil
		}, nil
	}
	printTranslateResultFn = func(root, modelID, output string, asJSON bool) error {
		printedOutput = output
		return nil
	}

	if err := runTranslateText([]string{"--text", "Hello"}); err != nil {
		t.Fatalf("runTranslateText returned error: %v", err)
	}
	if cleanupCalls != 1 {
		t.Fatalf("expected cleanup to run once, got %d", cleanupCalls)
	}
	if printedOutput != "你好" {
		t.Fatalf("expected translated output to be printed, got %q", printedOutput)
	}
}

func TestRunTranslateTextStopsOwnedRuntimeAfterFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/translate" {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "backend exploded", http.StatusInternalServerError)
	}))
	defer server.Close()

	prevPrepare := prepareTranslationRuntimeFn
	prevPrint := printTranslateResultFn
	t.Cleanup(func() {
		prepareTranslationRuntimeFn = prevPrepare
		printTranslateResultFn = prevPrint
	})

	cleanupCalls := 0
	printCalled := false
	prepareTranslationRuntimeFn = func(string, bool) (string, *translate.Service, models.QuantizedModel, func() error, error) {
		return t.TempDir(), translate.NewService(server.URL), models.QuantizedModel{ID: "q8_0"}, func() error {
			cleanupCalls++
			return nil
		}, nil
	}
	printTranslateResultFn = func(root, modelID, output string, asJSON bool) error {
		printCalled = true
		return nil
	}

	err := runTranslateText([]string{"--text", "Hello"})
	if err == nil {
		t.Fatalf("expected translation error")
	}
	if !strings.Contains(err.Error(), "backend returned 500 Internal Server Error") {
		t.Fatalf("unexpected error: %v", err)
	}
	if cleanupCalls != 1 {
		t.Fatalf("expected cleanup to run once, got %d", cleanupCalls)
	}
	if printCalled {
		t.Fatalf("did not expect result printer to run after translation error")
	}
}
