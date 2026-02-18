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

func transcribeFile(audioPath, lang string, vadOverride *bool) (TranscribeResponse, int) {
	start := time.Now()

	wavPath := audioPath
	var cleanup string
	if ext := strings.ToLower(filepath.Ext(audioPath)); ext != ".wav" {
		wavPath = fmt.Sprintf("/tmp/moonshine_%s.wav", uuid.New().String()[:8])
		cmd := exec.Command("ffmpeg", "-i", audioPath, "-ar", "16000", "-ac", "1", "-f", "wav", wavPath, "-y", "-loglevel", "error")
		if out, err := cmd.CombinedOutput(); err != nil {
			return TranscribeResponse{Error: fmt.Sprintf("ffmpeg: %s %s", err, out)}, http.StatusUnprocessableEntity
		}
		cleanup = wavPath
	}
	if cleanup != "" {
		defer os.Remove(cleanup)
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

	// VAD: auto-enable for long audio, respect explicit override
	useVAD := vadDetector != nil && audioDurS >= cfg.VADMinDurationS
	if vadOverride != nil {
		useVAD = *vadOverride && vadDetector != nil
	}

	// Build list of chunks to transcribe
	var chunks [][]float32
	var speechMs float64

	if useVAD {
		chunks = applyVADChunked(samples)
		if len(chunks) == 0 {
			return TranscribeResponse{DurationMs: float64(time.Since(start).Milliseconds())}, http.StatusOK
		}
		for _, c := range chunks {
			speechMs += float64(len(c)) / 16.0
		}
		log.Printf("VAD: %.0fms speech / %.0fms total (%.0f%%), %d chunk(s)",
			speechMs, audioDurS*1000, 100*speechMs/(audioDurS*1000), len(chunks))
	} else {
		chunks = [][]float32{samples}
	}

	// Transcribe each chunk, filter hallucinations, join
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
	text := strings.Join(parts, " ")

	resp := TranscribeResponse{
		Text:       text,
		DurationMs: float64(time.Since(start).Milliseconds()),
	}
	if speechMs > 0 {
		resp.SpeechMs = speechMs
	}
	return resp, http.StatusOK
}

// applyVADChunked feeds samples into VAD and returns speech segments
// grouped into chunks of at most 25 seconds each.
func applyVADChunked(samples []float32) [][]float32 {
	const windowSize = 512
	const maxChunkSamples = 25 * 16000 // 25s × 16kHz

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

func compressionRatio(text string) float64 {
	if len(text) < 10 {
		return 0
	}
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	w.Write([]byte(text)) //nolint:errcheck
	w.Close()
	return float64(len(text)) / float64(b.Len())
}

func loadWav(path string) ([]float32, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

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

	var samples []float32
	switch {
	case bitsPerSample == 16 && numChannels == 1:
		for i := 0; i+1 < len(data); i += 2 {
			s := int16(binary.LittleEndian.Uint16(data[i : i+2]))
			samples = append(samples, float32(s)/32768.0)
		}
	case bitsPerSample == 16 && numChannels == 2:
		for i := 0; i+3 < len(data); i += 4 {
			l := int16(binary.LittleEndian.Uint16(data[i : i+2]))
			rr := int16(binary.LittleEndian.Uint16(data[i+2 : i+4]))
			samples = append(samples, (float32(l)+float32(rr))/2.0/32768.0)
		}
	default:
		return nil, 0, fmt.Errorf("unsupported WAV: %dbit %dch", bitsPerSample, numChannels)
	}

	return samples, sampleRate, nil
}
