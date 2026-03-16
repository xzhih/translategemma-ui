package translate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeRequestDefaults(t *testing.T) {
	req, err := normalizeRequest(Request{
		SourceLang: "",
		TargetLang: "",
		Text:       "hello",
	})
	if err != nil {
		t.Fatalf("normalizeRequest returned error: %v", err)
	}
	if req.SourceLang != "auto" || req.TargetLang != "zh-CN" {
		t.Fatalf("unexpected defaults: source=%q target=%q", req.SourceLang, req.TargetLang)
	}
}

func TestNormalizeResponseTextTrimsWhitespace(t *testing.T) {
	got := normalizeResponseText("  你好\n")
	if got != "你好" {
		t.Fatalf("expected trimmed output %q, got %q", "你好", got)
	}
}

func TestNewServiceConfiguresSeparateHTTPClients(t *testing.T) {
	svc := NewService("http://127.0.0.1:8080")

	if svc.requestClient == nil || svc.streamClient == nil {
		t.Fatalf("expected both HTTP clients to be initialized")
	}
	if svc.requestClient.Timeout != requestTimeout {
		t.Fatalf("expected request timeout %s, got %s", requestTimeout, svc.requestClient.Timeout)
	}
	if svc.streamClient.Timeout != 0 {
		t.Fatalf("expected stream client timeout 0, got %s", svc.streamClient.Timeout)
	}
}

func TestEstimateMaxTokensClampsToExpectedRange(t *testing.T) {
	tests := []struct {
		name       string
		input      int
		wantTokens int
	}{
		{name: "minimum clamp", input: 0, wantTokens: 256},
		{name: "small input", input: 50, wantTokens: 256},
		{name: "medium input", input: 600, wantTokens: 1296},
		{name: "maximum clamp", input: 3000, wantTokens: 4096},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := estimateMaxTokens(tc.input); got != tc.wantTokens {
				t.Fatalf("estimateMaxTokens(%d) = %d, want %d", tc.input, got, tc.wantTokens)
			}
		})
	}
}

func TestTranslateImageUsesTranslateEndpoint(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/translate" {
			t.Fatalf("expected /v1/translate, got %s", r.URL.Path)
		}

		var payload struct {
			ImageDataURL           string `json:"image_data_url"`
			SourceLang             string `json:"source_lang"`
			TargetLang             string `json:"target_lang"`
			TranslationInstruction string `json:"translation_instruction"`
			MaxTokens              int    `json:"max_tokens"`
			Stream                 bool   `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if !strings.HasPrefix(payload.ImageDataURL, "data:image/jpeg;base64,") {
			t.Fatalf("expected image data URI, got %q", payload.ImageDataURL)
		}
		if payload.SourceLang != "cs" || payload.TargetLang != "zh-CN" {
			t.Fatalf("unexpected language payload: %#v", payload)
		}
		if payload.TranslationInstruction != "Preserve capitalization." {
			t.Fatalf("unexpected instruction payload: %#v", payload)
		}
		if payload.MaxTokens != defaultImageMaxTokens {
			t.Fatalf("expected max_tokens=%d, got %d", defaultImageMaxTokens, payload.MaxTokens)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":" 翻译结果 "}}]}`))
	}))
	defer ts.Close()

	svc := NewService(ts.URL)
	out, err := svc.TranslateImage(ImageRequest{
		SourceLang:             "cs",
		TargetLang:             "zh-CN",
		ImageMime:              "image/jpeg",
		ImageBytes:             []byte("fake-image"),
		TranslationInstruction: "Preserve capitalization.",
	})
	if err != nil {
		t.Fatalf("TranslateImage returned error: %v", err)
	}
	if out != "翻译结果" {
		t.Fatalf("expected trimmed image translation, got %q", out)
	}
}

func TestTranslateUsesDynamicTokenBudget(t *testing.T) {
	tokenizeCalls := 0
	translateCalls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tokenize":
			tokenizeCalls++
			_, _ = w.Write([]byte(`{"tokens":[1,2,3,4,5]}`))
		case "/v1/translate":
			translateCalls++
			var payload struct {
				Text                   string `json:"text"`
				SourceLang             string `json:"source_lang"`
				TargetLang             string `json:"target_lang"`
				TranslationInstruction string `json:"translation_instruction"`
				MaxTokens              int    `json:"max_tokens"`
				Stream                 bool   `json:"stream"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if payload.Text != "hello" {
				t.Fatalf("unexpected text payload: %#v", payload)
			}
			if payload.SourceLang != "auto" || payload.TargetLang != "zh-CN" {
				t.Fatalf("unexpected languages: %#v", payload)
			}
			if payload.TranslationInstruction != "Use Mainland China UI wording." {
				t.Fatalf("unexpected instruction payload: %#v", payload)
			}
			if payload.MaxTokens != 256 {
				t.Fatalf("expected max_tokens=256, got %d", payload.MaxTokens)
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":" 你好，世界 "}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	svc := NewService(ts.URL)
	out, err := svc.Translate(Request{
		Text:                   "hello",
		TranslationInstruction: "Use Mainland China UI wording.",
	})
	if err != nil {
		t.Fatalf("Translate returned error: %v", err)
	}
	if out != "你好，世界" {
		t.Fatalf("unexpected translation output %q", out)
	}
	if tokenizeCalls == 0 {
		t.Fatalf("expected /tokenize to be used before translating text")
	}
	if translateCalls != 1 {
		t.Fatalf("expected a single /v1/translate call, got %d", translateCalls)
	}
}

func TestTranslateFallsBackWhenTokenizeUnavailable(t *testing.T) {
	translateCalls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tokenize":
			http.Error(w, "missing tokenize endpoint", http.StatusNotFound)
		case "/v1/translate":
			translateCalls++
			var payload struct {
				Text      string `json:"text"`
				MaxTokens int    `json:"max_tokens"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if payload.MaxTokens != 296 {
				t.Fatalf("expected fallback max_tokens=296, got %d", payload.MaxTokens)
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"fallback ok"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	svc := NewService(ts.URL)
	out, err := svc.Translate(Request{Text: strings.Repeat("x", 100)})
	if err != nil {
		t.Fatalf("Translate returned error after tokenize fallback: %v", err)
	}
	if out != "fallback ok" {
		t.Fatalf("unexpected fallback translation %q", out)
	}
	if translateCalls != 1 {
		t.Fatalf("expected one translate call, got %d", translateCalls)
	}
}

func TestTranslateRetriesChunksAfterLengthFinishReason(t *testing.T) {
	left := strings.Repeat("a", 150)
	right := strings.Repeat("b", 150)
	original := left + " " + right
	seenTexts := make([]string, 0, 3)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tokenize":
			var payload struct {
				Content string `json:"content"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode tokenize body: %v", err)
			}
			tokens := make([]int, len([]rune(payload.Content)))
			if err := json.NewEncoder(w).Encode(map[string]any{"tokens": tokens}); err != nil {
				t.Fatalf("encode tokenize response: %v", err)
			}
		case "/v1/translate":
			var payload struct {
				Text string `json:"text"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode translate body: %v", err)
			}
			seenTexts = append(seenTexts, payload.Text)
			switch payload.Text {
			case original:
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`))
			case left:
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"LEFT"}}]}`))
			case right:
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"RIGHT"}}]}`))
			default:
				t.Fatalf("unexpected translation chunk %q", payload.Text)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	svc := NewService(ts.URL)
	out, err := svc.Translate(Request{Text: original})
	if err != nil {
		t.Fatalf("Translate returned error: %v", err)
	}
	if out != "LEFT RIGHT" {
		t.Fatalf("expected merged translation after retry, got %q", out)
	}
	if len(seenTexts) != 3 {
		t.Fatalf("expected three translate calls (original + two retries), got %d", len(seenTexts))
	}
}

func TestTranslateReturnsErrorWhenSmallChunkStillHitsLengthLimit(t *testing.T) {
	text := strings.Repeat("x", minRetryChunkTokens)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tokenize":
			tokens := make([]int, minRetryChunkTokens)
			if err := json.NewEncoder(w).Encode(map[string]any{"tokens": tokens}); err != nil {
				t.Fatalf("encode tokenize response: %v", err)
			}
		case "/v1/translate":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	svc := NewService(ts.URL)
	_, err := svc.Translate(Request{Text: text})
	if err == nil {
		t.Fatalf("expected an error when a <=200 token chunk still hits the length limit")
	}
	if !strings.Contains(err.Error(), "still exceeds the output limit") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStreamTranslateFallsBackToJSONResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tokenize":
			_, _ = w.Write([]byte(`{"tokens":[1,2,3]}`))
		case "/v1/translate":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"你好"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	svc := NewService(ts.URL)
	var deltas []string
	out, err := svc.StreamTranslateWithContextAndProgress(
		context.Background(),
		Request{Text: "hello", SourceLang: "en", TargetLang: "zh-CN"},
		func(delta string) error {
			deltas = append(deltas, delta)
			return nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("StreamTranslateWithContextAndProgress returned error: %v", err)
	}
	if out != "你好" {
		t.Fatalf("expected JSON fallback output %q, got %q", "你好", out)
	}
	if len(deltas) != 1 || deltas[0] != "你好" {
		t.Fatalf("expected JSON fallback to emit one delta, got %#v", deltas)
	}
}

func TestStreamTranslateReadsEventStreamResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tokenize":
			_, _ = w.Write([]byte(`{"tokens":[1,2,3]}`))
		case "/v1/translate":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"好\"},\"finish_reason\":\"stop\"}]}\n\n")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	svc := NewService(ts.URL)
	var deltas []string
	out, err := svc.StreamTranslateWithContextAndProgress(
		context.Background(),
		Request{Text: "hello", SourceLang: "en", TargetLang: "zh-CN"},
		func(delta string) error {
			deltas = append(deltas, delta)
			return nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("StreamTranslateWithContextAndProgress returned error: %v", err)
	}
	if out != "你好" {
		t.Fatalf("expected streamed output %q, got %q", "你好", out)
	}
	if strings.Join(deltas, "") != "你好" {
		t.Fatalf("expected streamed deltas to form output, got %#v", deltas)
	}
}
