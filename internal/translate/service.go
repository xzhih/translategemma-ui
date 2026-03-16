package translate

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"translategemma-ui/internal/runtime"
)

const (
	requestTimeout         = 120 * time.Second
	defaultTextChunkTokens = 900
	minRetryChunkTokens    = 200
	minGeneratedTokens     = 256
	maxGeneratedTokens     = 4096
	defaultImageMaxTokens  = 768
)

// Request is the normalized text translation input shape.
type Request struct {
	SourceLang             string
	TargetLang             string
	Text                   string
	TranslationInstruction string
}

// ImageRequest is the normalized image translation input shape.
type ImageRequest struct {
	SourceLang             string
	TargetLang             string
	ImageMime              string
	ImageBytes             []byte
	TranslationInstruction string
}

// ProgressUpdate describes coarse translation progress for chunked requests.
type ProgressUpdate struct {
	Message string
	Percent float64
}

type translatePayload struct {
	Text                   string `json:"text,omitempty"`
	ImageDataURL           string `json:"image_data_url,omitempty"`
	SourceLang             string `json:"source_lang,omitempty"`
	TargetLang             string `json:"target_lang,omitempty"`
	TranslationInstruction string `json:"translation_instruction,omitempty"`
	Stream                 bool   `json:"stream,omitempty"`
	MaxTokens              int    `json:"max_tokens,omitempty"`
}

type tokenCountResponse struct {
	Tokens []int `json:"tokens"`
}

type segmentTranslation struct {
	Output       string
	FinishReason string
}

type tokenCounter struct {
	ctx          context.Context
	service      Service
	cache        map[string]int
	fallbackOnly bool
}

func newTokenCounter(ctx context.Context, service Service) *tokenCounter {
	return &tokenCounter{
		ctx:     ctx,
		service: service,
		cache:   make(map[string]int),
	}
}

func (c *tokenCounter) Count(text string) int {
	if cached, ok := c.cache[text]; ok {
		return cached
	}

	count := fallbackTokenEstimate(text)
	if !c.fallbackOnly {
		if resolved, err := c.service.countTokensWithContext(c.ctx, text); err == nil && resolved > 0 {
			count = resolved
		} else {
			c.fallbackOnly = true
		}
	}

	c.cache[text] = count
	return count
}

// Service handles request normalization and TranslateGemma adaptation.
type Service struct {
	BackendURL    string
	requestClient *http.Client
	streamClient  *http.Client
}

// NewService creates a translation service backed by local TranslateGemma HTTP API.
func NewService(backendURL string) *Service {
	return &Service{
		BackendURL: runtime.NormalizeBackendURL(backendURL),
		requestClient: &http.Client{
			Timeout: requestTimeout,
		},
		streamClient: &http.Client{},
	}
}

// SetBackendURL updates the local TranslateGemma backend URL.
func (s *Service) SetBackendURL(backendURL string) {
	if s == nil {
		return
	}
	s.BackendURL = runtime.NormalizeBackendURL(backendURL)
}

// Translate calls the local TranslateGemma text endpoint.
func (s Service) Translate(req Request) (string, error) {
	return s.TranslateWithContext(context.Background(), req)
}

// TranslateWithContext calls the local TranslateGemma text endpoint.
func (s Service) TranslateWithContext(ctx context.Context, req Request) (string, error) {
	return s.translateTextWithContext(ctx, req, nil, nil, false)
}

// TranslateImage calls the local TranslateGemma image endpoint.
func (s Service) TranslateImage(req ImageRequest) (string, error) {
	return s.TranslateImageWithContext(context.Background(), req)
}

// TranslateImageWithContext calls the local TranslateGemma image endpoint.
func (s Service) TranslateImageWithContext(ctx context.Context, req ImageRequest) (string, error) {
	req, dataURL, err := normalizeImageRequest(req)
	if err != nil {
		return "", err
	}

	parsed, err := s.translateRequestWithContext(ctx, translatePayload{
		ImageDataURL:           dataURL,
		SourceLang:             req.SourceLang,
		TargetLang:             req.TargetLang,
		TranslationInstruction: req.TranslationInstruction,
		MaxTokens:              defaultImageMaxTokens,
	})
	if err != nil {
		return "", err
	}

	choice, err := firstChoice(parsed)
	if err != nil {
		return "", err
	}
	output := normalizeResponseText(choice.Message.Content)
	if output == "" {
		return "", fmt.Errorf("image translation output is empty")
	}
	return output, nil
}

// StreamTranslate requests streaming text chunks and calls onDelta for every chunk.
func (s Service) StreamTranslate(req Request, onDelta func(string) error) (string, error) {
	return s.StreamTranslateWithContext(context.Background(), req, onDelta)
}

// StreamTranslateWithContext requests streaming text chunks and calls onDelta for every chunk.
func (s Service) StreamTranslateWithContext(ctx context.Context, req Request, onDelta func(string) error) (string, error) {
	return s.StreamTranslateWithContextAndProgress(ctx, req, onDelta, nil)
}

// StreamTranslateWithContextAndProgress streams translated output and reports chunk progress.
func (s Service) StreamTranslateWithContextAndProgress(
	ctx context.Context,
	req Request,
	onDelta func(string) error,
	onProgress func(ProgressUpdate) error,
) (string, error) {
	return s.translateTextWithContext(ctx, req, onDelta, onProgress, true)
}

func (s Service) translateTextWithContext(
	ctx context.Context,
	req Request,
	onDelta func(string) error,
	onProgress func(ProgressUpdate) error,
	stream bool,
) (string, error) {
	req, err := normalizeRequest(req)
	if err != nil {
		return "", err
	}

	counter := newTokenCounter(ctx, s)
	segments := segmentForTranslation(req.Text, defaultTextChunkTokens, counter.Count)
	if len(segments) == 0 {
		return "", fmt.Errorf("translation output is empty")
	}

	totalParts := 0
	for _, segment := range segments {
		if segment.Translate {
			totalParts++
		}
	}
	liveStreaming := stream && totalParts == 1

	var full strings.Builder
	partIndex := 0
	for _, segment := range segments {
		if !segment.Translate {
			full.WriteString(segment.Text)
			if stream && onDelta != nil && segment.Text != "" {
				if err := onDelta(segment.Text); err != nil {
					return "", err
				}
			}
			continue
		}

		partIndex++
		if totalParts > 1 && onProgress != nil {
			progress := ProgressUpdate{
				Message: fmt.Sprintf("Translating part %d/%d", partIndex, totalParts),
				Percent: float64(partIndex-1) * 100 / float64(totalParts),
			}
			if err := onProgress(progress); err != nil {
				return "", err
			}
		}

		output, err := s.translateSegmentWithRetry(ctx, req, segment.Text, counter, stream, onDelta, liveStreaming)
		if err != nil {
			return "", err
		}
		full.WriteString(output)

		if stream && onDelta != nil && !liveStreaming && output != "" {
			if err := onDelta(output); err != nil {
				return "", err
			}
		}
	}

	output := normalizeResponseText(full.String())
	if output == "" {
		return "", fmt.Errorf("translation output is empty")
	}
	return output, nil
}

func (s Service) translateSegmentWithRetry(
	ctx context.Context,
	req Request,
	text string,
	counter *tokenCounter,
	stream bool,
	onDelta func(string) error,
	emitLive bool,
) (string, error) {
	if strings.TrimSpace(text) == "" {
		if stream && emitLive && onDelta != nil && text != "" {
			if err := onDelta(text); err != nil {
				return "", err
			}
		}
		return text, nil
	}

	tokenCount := counter.Count(text)
	translated, err := s.translateSegmentOnce(ctx, req, text, tokenCount, stream, onDelta, emitLive)
	if err != nil {
		return "", err
	}
	if !isLengthFinishReason(translated.FinishReason) {
		return translated.Output, nil
	}
	if tokenCount <= minRetryChunkTokens {
		return "", fmt.Errorf("translation chunk still exceeds the output limit at %d input tokens", tokenCount)
	}

	retryParts := splitRetrySegment(text)
	if len(retryParts) <= 1 {
		return "", fmt.Errorf("translation output hit the length limit and the chunk could not be split")
	}

	var full strings.Builder
	for _, part := range retryParts {
		if !part.Translate {
			full.WriteString(part.Text)
			if stream && emitLive && onDelta != nil && part.Text != "" {
				if err := onDelta(part.Text); err != nil {
					return "", err
				}
			}
			continue
		}

		output, err := s.translateSegmentWithRetry(ctx, req, part.Text, counter, stream, onDelta, emitLive)
		if err != nil {
			return "", err
		}
		full.WriteString(output)
	}
	return full.String(), nil
}

func (s Service) translateSegmentOnce(
	ctx context.Context,
	req Request,
	text string,
	inputTokens int,
	stream bool,
	onDelta func(string) error,
	emitLive bool,
) (segmentTranslation, error) {
	payload := translatePayload{
		Text:                   text,
		SourceLang:             req.SourceLang,
		TargetLang:             req.TargetLang,
		TranslationInstruction: req.TranslationInstruction,
		MaxTokens:              estimateMaxTokens(inputTokens),
		Stream:                 stream,
	}

	if stream {
		deltaCallback := onDelta
		if !emitLive {
			deltaCallback = nil
		}
		return s.translateStreamWithContext(ctx, payload, deltaCallback)
	}

	parsed, err := s.translateRequestWithContext(ctx, payload)
	if err != nil {
		return segmentTranslation{}, err
	}
	choice, err := firstChoice(parsed)
	if err != nil {
		return segmentTranslation{}, err
	}

	output := normalizeResponseText(choice.Message.Content)
	if output == "" && !isLengthFinishReason(choice.FinishReason) {
		return segmentTranslation{}, fmt.Errorf("translation output is empty")
	}
	return segmentTranslation{
		Output:       output,
		FinishReason: choice.FinishReason,
	}, nil
}

func (s Service) countTokensWithContext(ctx context.Context, text string) (int, error) {
	if text == "" {
		return 0, nil
	}

	body, err := json.Marshal(map[string]any{
		"content":       text,
		"add_special":   false,
		"parse_special": false,
	})
	if err != nil {
		return 0, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BackendURL+"/tokenize", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer no-key")

	resp, err := s.requestHTTPClient().Do(httpReq)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("tokenize endpoint returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	var parsed tokenCountResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return 0, fmt.Errorf("decode tokenize response: %w", err)
	}
	return len(parsed.Tokens), nil
}

func (s Service) translateRequest(payload translatePayload) (chatCompletionResponse, error) {
	return s.translateRequestWithContext(context.Background(), payload)
}

func (s Service) translateRequestWithContext(ctx context.Context, payload translatePayload) (chatCompletionResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return chatCompletionResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BackendURL+"/v1/translate", bytes.NewReader(body))
	if err != nil {
		return chatCompletionResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer no-key")

	resp, err := s.requestHTTPClient().Do(httpReq)
	if err != nil {
		return chatCompletionResponse{}, fmt.Errorf("backend request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return chatCompletionResponse{}, err
	}
	if resp.StatusCode >= 400 {
		return chatCompletionResponse{}, fmt.Errorf("backend returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return chatCompletionResponse{}, fmt.Errorf("decode backend response: %w", err)
	}
	return parsed, nil
}

func (s Service) translateStreamWithContext(
	ctx context.Context,
	payload translatePayload,
	onDelta func(string) error,
) (segmentTranslation, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return segmentTranslation{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BackendURL+"/v1/translate", bytes.NewReader(body))
	if err != nil {
		return segmentTranslation{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer no-key")

	resp, err := s.streamHTTPClient().Do(httpReq)
	if err != nil {
		return segmentTranslation{}, fmt.Errorf("backend request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return segmentTranslation{}, fmt.Errorf("backend returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	reader := bufio.NewReader(resp.Body)
	peeked, _ := reader.Peek(64)
	trimmedPeek := bytes.TrimSpace(peeked)
	if len(trimmedPeek) > 0 && (trimmedPeek[0] == '{' || trimmedPeek[0] == '[') {
		return s.translateStreamJSONFallback(reader, onDelta)
	}

	var (
		full         strings.Builder
		finishReason string
	)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}

		payloadLine := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payloadLine == "" {
			continue
		}
		if payloadLine == "[DONE]" {
			break
		}

		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(payloadLine), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]
		if choice.FinishReason != "" {
			finishReason = choice.FinishReason
		}

		delta := choice.Delta.Content
		if delta == "" {
			delta = choice.Message.Content
		}
		if delta == "" {
			continue
		}

		full.WriteString(delta)
		if onDelta != nil {
			if err := onDelta(delta); err != nil {
				return segmentTranslation{}, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return segmentTranslation{}, fmt.Errorf("read stream: %w", err)
	}

	output := normalizeResponseText(full.String())
	if output == "" && !isLengthFinishReason(finishReason) {
		return segmentTranslation{}, fmt.Errorf("translation output is empty")
	}
	return segmentTranslation{
		Output:       output,
		FinishReason: finishReason,
	}, nil
}

func (s Service) translateStreamJSONFallback(body io.Reader, onDelta func(string) error) (segmentTranslation, error) {
	respBody, err := io.ReadAll(body)
	if err != nil {
		return segmentTranslation{}, err
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return segmentTranslation{}, fmt.Errorf("decode backend response: %w", err)
	}

	choice, err := firstChoice(parsed)
	if err != nil {
		return segmentTranslation{}, err
	}
	output := normalizeResponseText(choice.Message.Content)
	if output == "" && !isLengthFinishReason(choice.FinishReason) {
		return segmentTranslation{}, fmt.Errorf("translation output is empty")
	}
	if output != "" && onDelta != nil {
		if err := onDelta(output); err != nil {
			return segmentTranslation{}, err
		}
	}
	return segmentTranslation{
		Output:       output,
		FinishReason: choice.FinishReason,
	}, nil
}

type chatCompletionResponse struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	FinishReason string `json:"finish_reason"`
}

type chatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func firstChoice(parsed chatCompletionResponse) (chatChoice, error) {
	if len(parsed.Choices) == 0 {
		return chatChoice{}, fmt.Errorf("backend returned no choices")
	}
	return parsed.Choices[0], nil
}

func normalizeRequest(req Request) (Request, error) {
	req.Text = strings.TrimSpace(req.Text)
	req.TranslationInstruction = strings.TrimSpace(req.TranslationInstruction)
	if req.Text == "" {
		return Request{}, fmt.Errorf("input text is empty")
	}
	if req.SourceLang == "" {
		req.SourceLang = "auto"
	}
	if req.TargetLang == "" {
		req.TargetLang = "zh-CN"
	}
	return req, nil
}

func normalizeImageRequest(req ImageRequest) (ImageRequest, string, error) {
	if len(req.ImageBytes) == 0 {
		return ImageRequest{}, "", fmt.Errorf("image file is empty")
	}
	req.ImageMime = strings.TrimSpace(strings.ToLower(req.ImageMime))
	req.TranslationInstruction = strings.TrimSpace(req.TranslationInstruction)
	if !strings.HasPrefix(req.ImageMime, "image/") {
		return ImageRequest{}, "", fmt.Errorf("invalid image MIME type: %s", req.ImageMime)
	}
	if req.SourceLang == "" {
		req.SourceLang = "auto"
	}
	if req.TargetLang == "" {
		req.TargetLang = "zh-CN"
	}
	dataURL := fmt.Sprintf("data:%s;base64,%s", req.ImageMime, base64.StdEncoding.EncodeToString(req.ImageBytes))
	return req, dataURL, nil
}

func normalizeResponseText(raw string) string {
	return strings.TrimSpace(raw)
}

func fallbackTokenEstimate(text string) int {
	count := utf8.RuneCountInString(text)
	if count < 1 {
		return 1
	}
	return count
}

func estimateMaxTokens(inputTokens int) int {
	if inputTokens < 1 {
		inputTokens = 1
	}
	return clampInt(inputTokens*2+96, minGeneratedTokens, maxGeneratedTokens)
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func isLengthFinishReason(reason string) bool {
	return strings.TrimSpace(reason) == "length"
}

func (s Service) requestHTTPClient() *http.Client {
	if s.requestClient != nil {
		return s.requestClient
	}
	return &http.Client{Timeout: requestTimeout}
}

func (s Service) streamHTTPClient() *http.Client {
	if s.streamClient != nil {
		return s.streamClient
	}
	return &http.Client{}
}
