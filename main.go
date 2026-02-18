package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

// injected via -ldflags at build time
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

var (
	recognizerEN *sherpa.OfflineRecognizer
	recognizerRU *sherpa.OfflineRecognizer
	muEN         sync.Mutex
	muRU         sync.Mutex
)

type TranscribeRequest struct {
	AudioPath string `json:"audio_path"`
	Language  string `json:"language,omitempty"` // "en" (default) or "ru"
}

type TranscribeResponse struct {
	Text       string  `json:"text"`
	DurationMs float64 `json:"duration_ms"`
	Error      string  `json:"error,omitempty"`
}

func main() {
	modelsDir := envOr("MOONSHINE_MODELS_DIR", "/models")
	ruModelsDir := envOr("ZIPFORMER_RU_DIR", "/ru-models")
	port := envOr("MOONSHINE_PORT", "8092")
	numThreads := 4

	// Load EN model (Moonshine)
	log.Printf("Loading Moonshine EN model from %s...", modelsDir)
	t0 := time.Now()
	cfgEN := &sherpa.OfflineRecognizerConfig{}
	cfgEN.FeatConfig.SampleRate = 16000
	cfgEN.FeatConfig.FeatureDim = 80
	cfgEN.ModelConfig.Moonshine.Preprocessor = filepath.Join(modelsDir, "preprocess.onnx")
	cfgEN.ModelConfig.Moonshine.Encoder = filepath.Join(modelsDir, "encode.int8.onnx")
	cfgEN.ModelConfig.Moonshine.UncachedDecoder = filepath.Join(modelsDir, "uncached_decode.int8.onnx")
	cfgEN.ModelConfig.Moonshine.CachedDecoder = filepath.Join(modelsDir, "cached_decode.int8.onnx")
	cfgEN.ModelConfig.Tokens = filepath.Join(modelsDir, "tokens.txt")
	cfgEN.ModelConfig.NumThreads = numThreads
	cfgEN.ModelConfig.Provider = "cpu"
	cfgEN.DecodingMethod = "greedy_search"
	recognizerEN = sherpa.NewOfflineRecognizer(cfgEN)
	if recognizerEN == nil {
		log.Fatalf("Failed to load EN model from %s", modelsDir)
	}
	defer sherpa.DeleteOfflineRecognizer(recognizerEN)
	log.Printf("EN model loaded in %.2fs", time.Since(t0).Seconds())

	// Load RU model (Zipformer) if available
	ruEncoder := filepath.Join(ruModelsDir, "encoder.int8.onnx")
	if _, err := os.Stat(ruEncoder); err == nil {
		log.Printf("Loading Zipformer RU model from %s...", ruModelsDir)
		t1 := time.Now()
		cfgRU := &sherpa.OfflineRecognizerConfig{}
		cfgRU.FeatConfig.SampleRate = 16000
		cfgRU.FeatConfig.FeatureDim = 80
		cfgRU.ModelConfig.Transducer.Encoder = ruEncoder
		cfgRU.ModelConfig.Transducer.Decoder = filepath.Join(ruModelsDir, "decoder.int8.onnx")
		cfgRU.ModelConfig.Transducer.Joiner = filepath.Join(ruModelsDir, "joiner.int8.onnx")
		cfgRU.ModelConfig.Tokens = filepath.Join(ruModelsDir, "tokens.txt")
		cfgRU.ModelConfig.NumThreads = numThreads
		cfgRU.ModelConfig.Provider = "cpu"
		cfgRU.DecodingMethod = "greedy_search"
		recognizerRU = sherpa.NewOfflineRecognizer(cfgRU)
		if recognizerRU != nil {
			defer sherpa.DeleteOfflineRecognizer(recognizerRU)
			log.Printf("RU model loaded in %.2fs", time.Since(t1).Seconds())
		} else {
			log.Printf("WARNING: failed to load RU model, RU transcription unavailable")
		}
	} else {
		log.Printf("RU model not found at %s, RU transcription unavailable", ruModelsDir)
	}

	warmup()

	http.HandleFunc("/transcribe", handleTranscribe)
	http.HandleFunc("/transcribe/upload", handleUpload)
	http.HandleFunc("/health", handleHealth)

	ruStatus := "unavailable"
	if recognizerRU != nil {
		ruStatus = "ready"
	}
	log.Printf("Service on :%s | EN: ready (~0.26s) | RU: %s (~0.19s)", port, ruStatus)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func warmup() {
	samples := make([]float32, 16000) // 1 sec silence

	muEN.Lock()
	ws := sherpa.NewOfflineStream(recognizerEN)
	ws.AcceptWaveform(16000, samples)
	recognizerEN.Decode(ws)
	sherpa.DeleteOfflineStream(ws)
	muEN.Unlock()

	if recognizerRU != nil {
		muRU.Lock()
		ws2 := sherpa.NewOfflineStream(recognizerRU)
		ws2.AcceptWaveform(16000, samples)
		recognizerRU.Decode(ws2)
		sherpa.DeleteOfflineStream(ws2)
		muRU.Unlock()
	}
	log.Println("Warmup complete")
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ruReady := recognizerRU != nil
	fmt.Fprintf(w, `{"status":"ok","engine":"sherpa-onnx","version":%q,"commit":%q,"languages":{"en":{"model":"moonshine-tiny-en-int8","ready":true},"ru":{"model":"zipformer-ru-int8","ready":%v}}}`,
		version, commit, ruReady)
}

func handleTranscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	var req TranscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(TranscribeResponse{Error: "invalid json: " + err.Error()})
		return
	}

	if req.AudioPath == "" {
		json.NewEncoder(w).Encode(TranscribeResponse{Error: "audio_path required"})
		return
	}

	lang := strings.ToLower(strings.TrimSpace(req.Language))
	if lang == "" {
		lang = "en"
	}
	result := transcribeFile(req.AudioPath, lang)
	json.NewEncoder(w).Encode(result)
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		json.NewEncoder(w).Encode(TranscribeResponse{Error: "parse form: " + err.Error()})
		return
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		json.NewEncoder(w).Encode(TranscribeResponse{Error: "audio file required"})
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
		json.NewEncoder(w).Encode(TranscribeResponse{Error: "save temp: " + err.Error()})
		return
	}
	io.Copy(out, file)
	out.Close()
	defer os.Remove(tmpFile)

	lang := strings.ToLower(strings.TrimSpace(r.FormValue("language")))
	if lang == "" {
		lang = "en"
	}
	result := transcribeFile(tmpFile, lang)
	json.NewEncoder(w).Encode(result)
}

func transcribeFile(audioPath, lang string) TranscribeResponse {
	start := time.Now()

	wavPath := audioPath
	var cleanup string
	if ext := strings.ToLower(filepath.Ext(audioPath)); ext != ".wav" {
		wavPath = fmt.Sprintf("/tmp/moonshine_%s.wav", uuid.New().String()[:8])
		cmd := exec.Command("ffmpeg", "-i", audioPath, "-ar", "16000", "-ac", "1", "-f", "wav", wavPath, "-y", "-loglevel", "error")
		if out, err := cmd.CombinedOutput(); err != nil {
			return TranscribeResponse{Error: fmt.Sprintf("ffmpeg: %s %s", err, out)}
		}
		cleanup = wavPath
	}
	if cleanup != "" {
		defer os.Remove(cleanup)
	}

	samples, sampleRate, err := loadWav(wavPath)
	if err != nil {
		return TranscribeResponse{Error: "load wav: " + err.Error()}
	}

	var text string
	if lang == "ru" {
		if recognizerRU == nil {
			return TranscribeResponse{Error: "RU model not loaded; set ZIPFORMER_RU_DIR"}
		}
		muRU.Lock()
		stream := sherpa.NewOfflineStream(recognizerRU)
		stream.AcceptWaveform(sampleRate, samples)
		recognizerRU.Decode(stream)
		text = stream.GetResult().Text
		sherpa.DeleteOfflineStream(stream)
		muRU.Unlock()
	} else {
		muEN.Lock()
		stream := sherpa.NewOfflineStream(recognizerEN)
		stream.AcceptWaveform(sampleRate, samples)
		recognizerEN.Decode(stream)
		text = stream.GetResult().Text
		sherpa.DeleteOfflineStream(stream)
		muEN.Unlock()
	}

	return TranscribeResponse{
		Text:       strings.TrimSpace(text),
		DurationMs: float64(time.Since(start).Milliseconds()),
	}
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
	if bitsPerSample == 16 && numChannels == 1 {
		for i := 0; i+1 < len(data); i += 2 {
			s := int16(binary.LittleEndian.Uint16(data[i : i+2]))
			samples = append(samples, float32(s)/32768.0)
		}
	} else if bitsPerSample == 16 && numChannels == 2 {
		for i := 0; i+3 < len(data); i += 4 {
			l := int16(binary.LittleEndian.Uint16(data[i : i+2]))
			r := int16(binary.LittleEndian.Uint16(data[i+2 : i+4]))
			samples = append(samples, (float32(l)+float32(r))/2.0/32768.0)
		}
	} else {
		return nil, 0, fmt.Errorf("unsupported WAV: %dbit %dch", bitsPerSample, numChannels)
	}

	return samples, sampleRate, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
