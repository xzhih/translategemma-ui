package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	lf "translategemma-ui/internal/runtime/llamafile"
	"translategemma-ui/internal/translate"
)

type translateRequestPayload struct {
	SourceLang             string `json:"source_lang"`
	TargetLang             string `json:"target_lang"`
	TranslationInstruction string `json:"translation_instruction"`
	InputText              string `json:"input_text"`
	Text                   string `json:"text"`
}

type translateResult struct {
	OK          bool            `json:"ok"`
	Output      string          `json:"output,omitempty"`
	Message     string          `json:"message,omitempty"`
	MessageCode string          `json:"messageCode,omitempty"`
	History     *historyPayload `json:"history,omitempty"`
	Count       int             `json:"count,omitempty"`
}

func (s *Server) handleTranslate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	source, target, instruction, inputText, err := parseTranslateRequest(r)
	if err != nil {
		http.Error(w, "invalid form payload", http.StatusBadRequest)
		return
	}

	if _, err := s.ensureRuntime(nil); err != nil {
		s.setStatus(err.Error())
		runtimeReady, runtimeStatus := s.runtimeView()
		s.render(w, s.viewData(source, target, inputText, instruction, "", runtimeReady, runtimeStatus))
		return
	}

	output, err := s.translator.TranslateWithContext(r.Context(), translate.Request{
		SourceLang:             source,
		TargetLang:             target,
		Text:                   inputText,
		TranslationInstruction: instruction,
	})
	if err != nil {
		s.setStatus(err.Error())
		output = ""
	} else {
		s.pushHistory(historyItem{
			Source: source,
			Target: target,
			Input:  inputText,
			Output: output,
			When:   s.now(),
		})
		s.setStatusCode(statusCodeTranslationCompleted, "Translation completed")
	}

	runtimeReady, runtimeStatus := s.runtimeView()
	s.render(w, s.viewData(source, target, inputText, instruction, output, runtimeReady, runtimeStatus))
}

func (s *Server) handleTranslateAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	source, target, instruction, inputText, err := parseTranslateRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, translateResult{
			OK:          false,
			Message:     "invalid translate payload",
			MessageCode: statusCodeInvalidFormPayload,
			Count:       s.historyCount(),
		})
		return
	}

	if _, err := s.ensureRuntime(nil); err != nil {
		s.setStatus(err.Error())
		writeJSON(w, http.StatusServiceUnavailable, translateResult{
			OK:      false,
			Message: err.Error(),
			Count:   s.historyCount(),
		})
		return
	}

	output, err := s.translator.TranslateWithContext(r.Context(), translate.Request{
		SourceLang:             source,
		TargetLang:             target,
		Text:                   inputText,
		TranslationInstruction: instruction,
	})
	if err != nil {
		s.setStatus(err.Error())
		writeJSON(w, http.StatusBadRequest, translateResult{
			OK:      false,
			Message: err.Error(),
			Count:   s.historyCount(),
		})
		return
	}

	entry := s.pushHistory(historyItem{
		Source: source,
		Target: target,
		Input:  inputText,
		Output: output,
		When:   s.now(),
	})
	s.setStatusCode(statusCodeTranslationCompleted, "Translation completed")
	writeJSON(w, http.StatusOK, translateResult{
		OK:          true,
		Output:      output,
		Message:     "Translation completed",
		MessageCode: statusCodeTranslationCompleted,
		History:     historyToPayload(entry),
		Count:       s.historyCount(),
	})
}

func (s *Server) handleTranslateStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	source, target, instruction, inputText, err := parseTranslateRequest(r)
	if err != nil {
		http.Error(w, "invalid form payload", http.StatusBadRequest)
		return
	}

	writeEvent, err := prepareStreamWriter(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := s.ensureRuntime(func(p lf.Progress) {
		_ = writeEvent(streamEvent{Type: "progress", Stage: p.Stage, Message: p.Message, Percent: p.Percent})
	}); err != nil {
		s.setStatus(err.Error())
		_ = writeEvent(streamEvent{Type: "error", Stage: "load", Message: err.Error()})
		return
	}

	s.setStatusCode(statusCodeStreamingTranslation, "Streaming translation")
	_ = writeEvent(streamEvent{Type: "status", Message: "Streaming translation", MessageCode: statusCodeStreamingTranslation})

	finalOutput, streamErr := s.translator.StreamTranslateWithContextAndProgress(r.Context(), translate.Request{
		SourceLang:             source,
		TargetLang:             target,
		Text:                   inputText,
		TranslationInstruction: instruction,
	}, func(delta string) error {
		return writeEvent(streamEvent{Type: "delta", Delta: delta})
	}, func(update translate.ProgressUpdate) error {
		return writeEvent(streamEvent{
			Type:    "progress",
			Stage:   "translate",
			Message: update.Message,
			Percent: update.Percent,
		})
	})
	if streamErr != nil {
		s.setStatus(streamErr.Error())
		_ = writeEvent(streamEvent{Type: "error", Message: streamErr.Error()})
		return
	}

	entry := s.pushHistory(historyItem{
		Source: source,
		Target: target,
		Input:  inputText,
		Output: finalOutput,
		When:   s.now(),
	})
	s.setStatusCode(statusCodeTranslationCompleted, "Translation completed")
	_ = writeEvent(streamEvent{
		Type:        "done",
		Message:     "Translation completed",
		MessageCode: statusCodeTranslationCompleted,
		Output:      finalOutput,
		History:     historyToPayload(entry),
		Count:       s.historyCount(),
	})
}

func (s *Server) handleImageTranslate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.visionEnabled() {
		writeJSON(w, http.StatusBadRequest, imageResult{
			OK:          false,
			Message:     "the active runtime does not support image translation. Switch to q8_0_vision first.",
			MessageCode: statusCodeActiveRuntimeNoImageSupport,
			Count:       s.historyCount(),
		})
		return
	}
	if _, err := s.ensureRuntime(nil); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, imageResult{OK: false, Message: err.Error(), Count: s.historyCount()})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxImageUploadMB*1024*1024+1024)
	if err := r.ParseMultipartForm(maxImageUploadMB * 1024 * 1024); err != nil {
		writeJSON(w, http.StatusBadRequest, imageResult{OK: false, Message: "invalid multipart payload or file too large", MessageCode: statusCodeInvalidMultipartPayloadOrFileTooLarge, Count: s.historyCount()})
		return
	}

	source := strings.TrimSpace(r.FormValue("source_lang"))
	target := strings.TrimSpace(r.FormValue("target_lang"))
	instruction := strings.TrimSpace(r.FormValue("translation_instruction"))
	if source == "" {
		source = "auto"
	}
	if target == "" {
		target = "zh-CN"
	}

	file, header, err := r.FormFile("image_file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, imageResult{OK: false, Message: "missing image_file", MessageCode: statusCodeMissingImageFile, Count: s.historyCount()})
		return
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, maxImageUploadMB*1024*1024+1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, imageResult{OK: false, Message: "unable to read image file", MessageCode: statusCodeUnableToReadImageFile, Count: s.historyCount()})
		return
	}
	if len(content) == 0 {
		writeJSON(w, http.StatusBadRequest, imageResult{OK: false, Message: "image file is empty", MessageCode: statusCodeImageFileEmpty, Count: s.historyCount()})
		return
	}
	if len(content) > maxImageUploadMB*1024*1024 {
		writeJSON(w, http.StatusBadRequest, imageResult{OK: false, Message: "image file exceeds size limit", MessageCode: statusCodeImageFileExceedsSizeLimit, Count: s.historyCount()})
		return
	}

	mimeType := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	if mimeType == "" {
		mimeType = strings.ToLower(http.DetectContentType(content))
	}
	if !allowedImageMIME(mimeType) {
		writeJSON(w, http.StatusBadRequest, imageResult{OK: false, Message: "unsupported image format; only JPEG, PNG, GIF are allowed", MessageCode: statusCodeUnsupportedImageFormat, Count: s.historyCount()})
		return
	}

	output, err := s.translator.TranslateImageWithContext(r.Context(), translate.ImageRequest{
		SourceLang:             source,
		TargetLang:             target,
		ImageMime:              mimeType,
		ImageBytes:             content,
		TranslationInstruction: instruction,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, imageResult{OK: false, Message: err.Error(), Count: s.historyCount()})
		return
	}

	entry := s.pushHistory(historyItem{
		Source: source,
		Target: target,
		Input:  "[image] " + header.Filename,
		Output: output,
		When:   s.now(),
	})
	s.setStatusCode(statusCodeFileTranslationCompleted, "File translation completed")
	writeJSON(w, http.StatusOK, imageResult{
		OK:          true,
		Output:      output,
		Message:     "File translation completed",
		MessageCode: statusCodeFileTranslationCompleted,
		History:     historyToPayload(entry),
		Count:       s.historyCount(),
	})
}

func parseTranslateRequest(r *http.Request) (source, target, instruction, input string, err error) {
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "application/json") {
		var payload translateRequestPayload
		if err = json.NewDecoder(r.Body).Decode(&payload); err != nil {
			return "", "", "", "", err
		}
		source = strings.TrimSpace(payload.SourceLang)
		target = strings.TrimSpace(payload.TargetLang)
		instruction = strings.TrimSpace(payload.TranslationInstruction)
		input = strings.TrimSpace(payload.InputText)
		if input == "" {
			input = strings.TrimSpace(payload.Text)
		}
	} else {
		if err = r.ParseForm(); err != nil {
			return "", "", "", "", err
		}
		source = strings.TrimSpace(r.FormValue("source_lang"))
		target = strings.TrimSpace(r.FormValue("target_lang"))
		instruction = strings.TrimSpace(r.FormValue("translation_instruction"))
		input = strings.TrimSpace(r.FormValue("input_text"))
		if input == "" {
			input = strings.TrimSpace(r.FormValue("text"))
		}
	}
	if source == "" {
		source = "auto"
	}
	if target == "" {
		target = "zh-CN"
	}
	return source, target, instruction, input, nil
}

func allowedImageMIME(m string) bool {
	switch m {
	case "image/jpeg", "image/jpg", "image/png", "image/gif":
		return true
	default:
		return false
	}
}
