package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// TranscribeRequest is the JSON body for POST /transcribe.
type TranscribeRequest struct {
	AudioPath   string `json:"audio_path"`
	Language    string `json:"language,omitempty"`
	VAD         *bool  `json:"vad,omitempty"`          // nil=auto, false=skip
	MaxChunkLen int    `json:"max_chunk_len,omitempty"` // 0=no chunking
	Punctuate   *bool  `json:"punctuate,omitempty"`     // nil=auto, true=force
}

// TranscribeResponse is the JSON response returned by transcription endpoints.
type TranscribeResponse struct {
	Text       string   `json:"text"`
	Chunks     []string `json:"chunks,omitempty"`
	DurationMs float64  `json:"duration_ms"`
	SpeechMs   float64  `json:"speech_ms,omitempty"`
	Error      string   `json:"error,omitempty"`
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// loggingMiddleware logs every HTTP request with method, path, status, and latency.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %dms", r.Method, r.URL.Path, sw.status, time.Since(start).Milliseconds())
	})
}

// writeJSON encodes v as JSON and writes it with the given HTTP status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// writeError sends an error response with the given HTTP status and message.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, TranscribeResponse{Error: msg})
}

// normLang normalizes a language string to lowercase, defaulting to "en".
func normLang(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "en"
	}
	return s
}

// parseBoolPtr parses a string as a boolean pointer; returns nil for unrecognized values.
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

// handleHealth returns service status, model readiness, and version info.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"engine":      "sherpa-onnx",
		"version":     version,
		"commit":      commit,
		"vad":         vadDetector != nil,
		"punctuation": punctuator != nil,
		"languages": map[string]any{
			"en": map[string]any{"model": "moonshine-v2-base-en", "ready": true},
			"ru": map[string]any{"model": "zipformer-ru-int8", "ready": recognizerRU != nil},
		},
	})
}

// handleTranscribe handles POST /transcribe with a JSON body containing audio_path.
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
	resp, status := transcribeFile(req.AudioPath, normLang(req.Language), req.VAD, req.Punctuate)
	if status == http.StatusOK && req.MaxChunkLen > 0 {
		resp.Chunks = splitText(resp.Text, req.MaxChunkLen)
	}
	writeJSON(w, status, resp)
}

// handleUpload handles POST /transcribe/upload with multipart file upload.
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
	defer file.Close() //nolint:errcheck

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
	_ = out.Close()
	defer os.Remove(tmpFile) //nolint:errcheck

	resp, status := transcribeFile(tmpFile, normLang(r.FormValue("language")),
		parseBoolPtr(r.FormValue("vad")), parseBoolPtr(r.FormValue("punctuate")))
	if status == http.StatusOK {
		if maxChunk, err := strconv.Atoi(r.FormValue("max_chunk_len")); err == nil && maxChunk > 0 {
			resp.Chunks = splitText(resp.Text, maxChunk)
		}
	}
	writeJSON(w, status, resp)
}
