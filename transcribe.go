package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

// transcribeFile is the main entry point: converts audio, runs VAD, transcribes, and returns results.
func transcribeFile(audioPath, lang string, vadOverride, punctOverride *bool) (TranscribeResponse, int) {
	start := time.Now()

	wavPath, cleanupPath, err := ensureWav(audioPath)
	if err != nil {
		return TranscribeResponse{Error: err.Error()}, http.StatusUnprocessableEntity
	}
	if cleanupPath != "" {
		defer os.Remove(cleanupPath) //nolint:errcheck
	}

	samples, sampleRate, err := loadWav(wavPath)
	if err != nil {
		return TranscribeResponse{Error: "load wav: " + err.Error()}, http.StatusBadRequest
	}
	if sampleRate != 16000 {
		return TranscribeResponse{Error: fmt.Sprintf("unsupported sample rate %d (need 16000)", sampleRate)}, http.StatusBadRequest
	}

	audioDurS := float64(len(samples)) / 16000.0
	if audioDurS > cfg.MaxAudioDurationS {
		return TranscribeResponse{
			Error: fmt.Sprintf("audio too long: %.1fs > max %.0fs", audioDurS, cfg.MaxAudioDurationS),
		}, http.StatusBadRequest
	}

	if lang == "ru" && recognizerRU == nil {
		return TranscribeResponse{Error: "RU model not loaded; set ZIPFORMER_RU_DIR"}, http.StatusServiceUnavailable
	}

	chunks, speechMs := buildAudioChunks(samples, audioDurS, vadOverride)
	if len(chunks) == 0 {
		return TranscribeResponse{DurationMs: float64(time.Since(start).Milliseconds())}, http.StatusOK
	}

	text := transcribeChunks(chunks, sampleRate, lang)

	// Apply punctuation: auto (nil) = yes if EN and model loaded; explicit override respected.
	doPunct := punctuator != nil && lang == "en"
	if punctOverride != nil {
		doPunct = *punctOverride && punctuator != nil
	}
	if doPunct {
		text = addPunctuation(text)
	}

	resp := TranscribeResponse{
		Text:       text,
		DurationMs: float64(time.Since(start).Milliseconds()),
	}
	if speechMs > 0 {
		resp.SpeechMs = speechMs
	}
	return resp, http.StatusOK
}

// ensureWav converts audioPath to 16kHz mono WAV if it is not already WAV.
// Returns the WAV path and an optional cleanup path to remove after use.
func ensureWav(audioPath string) (wavPath, cleanupPath string, err error) {
	if ext := strings.ToLower(filepath.Ext(audioPath)); ext == ".wav" {
		return audioPath, "", nil
	}
	wavPath = fmt.Sprintf("/tmp/moonshine_%s.wav", uuid.New().String()[:8])
	cmd := exec.Command("ffmpeg", "-i", audioPath, "-ar", "16000", "-ac", "1",
		"-f", "wav", wavPath, "-y", "-loglevel", "error")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("ffmpeg: %s %s", err, out)
	}
	return wavPath, wavPath, nil
}

// buildAudioChunks decides whether to use VAD and returns audio chunks with speech duration.
func buildAudioChunks(samples []float32, audioDurS float64, vadOverride *bool) ([][]float32, float64) {
	useVAD := vadDetector != nil && audioDurS >= cfg.VADMinDurationS
	if vadOverride != nil {
		useVAD = *vadOverride && vadDetector != nil
	}

	if !useVAD {
		return [][]float32{samples}, 0
	}

	chunks := applyVADChunked(samples)
	if len(chunks) == 0 {
		return nil, 0
	}

	var speechMs float64
	for _, c := range chunks {
		speechMs += float64(len(c)) / 16.0
	}
	log.Printf("VAD: %.0fms speech / %.0fms total (%.0f%%), %d chunk(s)",
		speechMs, audioDurS*1000, 100*speechMs/(audioDurS*1000), len(chunks))

	return chunks, speechMs
}

// transcribeChunks recognizes each audio chunk and joins results,
// filtering hallucinations by compression ratio.
func transcribeChunks(chunks [][]float32, sampleRate int, lang string) string {
	var parts []string
	for _, chunk := range chunks {
		t := strings.TrimSpace(recognizeChunk(chunk, sampleRate, lang))
		if ratio := compressionRatio(t); ratio > 2.4 {
			log.Printf("WARNING: chunk compression ratio %.2f > 2.4, skipping hallucination", ratio)
			continue
		}
		if t != "" {
			parts = append(parts, t)
		}
	}
	return sanitizeUTF8(strings.Join(parts, " "))
}

// applyVADChunked feeds samples into VAD and returns speech segments
// grouped into chunks of at most 25 seconds each.
func applyVADChunked(samples []float32) [][]float32 {
	const windowSize = 512
	const maxChunkSamples = 25 * 16000 // 25s x 16kHz

	muVAD.Lock()
	defer muVAD.Unlock()

	for i := 0; i+windowSize <= len(samples); i += windowSize {
		vadDetector.AcceptWaveform(samples[i : i+windowSize])
	}
	if rem := len(samples) % windowSize; rem != 0 {
		pad := make([]float32, windowSize)
		copy(pad, samples[len(samples)-rem:])
		vadDetector.AcceptWaveform(pad)
	}
	vadDetector.Flush()

	var chunks [][]float32
	var current []float32
	for !vadDetector.IsEmpty() {
		seg := vadDetector.Front()
		if len(current)+len(seg.Samples) > maxChunkSamples && len(current) > 0 {
			chunks = append(chunks, current)
			current = nil
		}
		current = append(current, seg.Samples...)
		vadDetector.Pop()
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	vadDetector.Reset()
	return chunks
}

// recognizeChunk runs inference on a single audio chunk using the specified language model.
func recognizeChunk(samples []float32, sampleRate int, lang string) string {
	switch lang {
	case "ru":
		muRU.Lock()
		s := sherpa.NewOfflineStream(recognizerRU)
		s.AcceptWaveform(sampleRate, samples)
		recognizerRU.Decode(s)
		text := s.GetResult().Text
		sherpa.DeleteOfflineStream(s)
		muRU.Unlock()
		return text
	default:
		muEN.Lock()
		s := sherpa.NewOfflineStream(recognizerEN)
		s.AcceptWaveform(sampleRate, samples)
		recognizerEN.Decode(s)
		text := s.GetResult().Text
		sherpa.DeleteOfflineStream(s)
		muEN.Unlock()
		return text
	}
}

// compressionRatio returns the zlib compression ratio of text.
// High values (>2.4) indicate repetitive/hallucinated output.
func compressionRatio(text string) float64 {
	if len(text) < 10 {
		return 0
	}
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	w.Write([]byte(text)) //nolint:errcheck
	_ = w.Close()
	return float64(len(text)) / float64(b.Len())
}

// loadWav reads a WAV file and returns PCM samples as float32 in [-1, +1] range.
func loadWav(path string) ([]float32, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close() //nolint:errcheck

	header := make([]byte, 44)
	if _, err := io.ReadFull(f, header); err != nil {
		return nil, 0, fmt.Errorf("read header: %w", err)
	}

	sampleRate := int(binary.LittleEndian.Uint32(header[24:28]))
	numChannels := int(binary.LittleEndian.Uint16(header[22:24]))
	bitsPerSample := int(binary.LittleEndian.Uint16(header[34:36]))

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, 0, err
	}

	return parsePCM(data, numChannels, bitsPerSample, sampleRate)
}

// parsePCM converts raw PCM bytes to float32 samples normalized to [-1, +1].
func parsePCM(data []byte, numChannels, bitsPerSample, sampleRate int) ([]float32, int, error) {
	bytesPerFrame := numChannels * (bitsPerSample / 8)
	if bytesPerFrame == 0 {
		return nil, 0, fmt.Errorf("unsupported WAV: %dbit %dch", bitsPerSample, numChannels)
	}
	numSamples := len(data) / bytesPerFrame
	samples := make([]float32, 0, numSamples)

	switch {
	case bitsPerSample == 16 && numChannels == 1:
		for i := 0; i+1 < len(data); i += 2 {
			s := int16(binary.LittleEndian.Uint16(data[i : i+2]))
			samples = append(samples, float32(s)/32768.0)
		}
	case bitsPerSample == 16 && numChannels == 2:
		for i := 0; i+3 < len(data); i += 4 {
			l := int16(binary.LittleEndian.Uint16(data[i : i+2]))
			r := int16(binary.LittleEndian.Uint16(data[i+2 : i+4]))
			samples = append(samples, (float32(l)+float32(r))/2.0/32768.0)
		}
	default:
		return nil, 0, fmt.Errorf("unsupported WAV: %dbit %dch", bitsPerSample, numChannels)
	}
	return samples, sampleRate, nil
}
