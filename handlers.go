package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

type TranscribeRequest struct {
	AudioPath string `json:"audio_path"`
	Language  string `json:"language,omitempty"`
	VAD       *bool  `json:"vad,omitempty"` // nil=auto, false=skip
}

type TranscribeResponse struct {
	Text       string  `json:"text"`
	DurationMs float64 `json:"duration_ms"`
	SpeechMs   float64 `json:"speech_ms,omitempty"`
	Error      string  `json:"error,omitempty"`
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %dms", r.Method, r.URL.Path, sw.status, time.Since(start).Milliseconds())
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, TranscribeResponse{Error: msg})
}

func normLang(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "en"
	}
	return s
}

func parseBoolPtr(s string) *bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes":
		t := true
		return &t
	case "false", "0", "no":
		f := false
		return &f
	}
	return nil
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"engine":  "sherpa-onnx",
		"version": version,
		"commit":  commit,
		"vad":     vadDetector != nil,
		"languages": map[string]any{
			"en": map[string]any{"model": "moonshine-tiny-en-int8", "ready": true},
			"ru": map[string]any{"model": "zipformer-ru-int8", "ready": recognizerRU != nil},
		},
	})
}

func handleTranscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req TranscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.AudioPath == "" {
		writeError(w, http.StatusBadRequest, "audio_path required")
		return
	}
	resp, status := transcribeFile(req.AudioPath, normLang(req.Language), req.VAD)
	writeJSON(w, status, resp)
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "parse form: "+err.Error())
		return
	}
	file, header, err := r.FormFile("audio")
	if err != nil {
		writeError(w, http.StatusBadRequest, "audio file required")
		return
	}
	defer file.Close()

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".wav"
	}
	tmpFile := fmt.Sprintf("/tmp/moonshine_%s%s", uuid.New().String()[:8], ext)
	out, err := os.Create(tmpFile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save temp: "+err.Error())
		return
	}
	io.Copy(out, file) //nolint:errcheck
	out.Close()
	defer os.Remove(tmpFile)

	resp, status := transcribeFile(tmpFile, normLang(r.FormValue("language")), parseBoolPtr(r.FormValue("vad")))
	writeJSON(w, status, resp)
}
