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
	recognizer *sherpa.OfflineRecognizer
	mu         sync.Mutex // sherpa-onnx is not thread-safe
)

type TranscribeRequest struct {
	AudioPath string `json:"audio_path"`
}

type TranscribeResponse struct {
	Text       string  `json:"text"`
	DurationMs float64 `json:"duration_ms"`
	Error      string  `json:"error,omitempty"`
}

func main() {
	modelsDir := envOr("MOONSHINE_MODELS_DIR", "/models")
	port := envOr("MOONSHINE_PORT", "8092")
	numThreads := 4

	log.Printf("Loading Moonshine model from %s...", modelsDir)
	t0 := time.Now()

	config := &sherpa.OfflineRecognizerConfig{}
	config.FeatConfig.SampleRate = 16000
	config.FeatConfig.FeatureDim = 80
	config.ModelConfig.Moonshine.Preprocessor = filepath.Join(modelsDir, "preprocess.onnx")
	config.ModelConfig.Moonshine.Encoder = filepath.Join(modelsDir, "encode.int8.onnx")
	config.ModelConfig.Moonshine.UncachedDecoder = filepath.Join(modelsDir, "uncached_decode.int8.onnx")
	config.ModelConfig.Moonshine.CachedDecoder = filepath.Join(modelsDir, "cached_decode.int8.onnx")
	config.ModelConfig.Tokens = filepath.Join(modelsDir, "tokens.txt")
	config.ModelConfig.NumThreads = numThreads
	config.ModelConfig.Provider = "cpu"
	config.DecodingMethod = "greedy_search"

	recognizer = sherpa.NewOfflineRecognizer(config)
	if recognizer == nil {
		log.Fatalf("Failed to create recognizer from %s", modelsDir)
	}
	defer sherpa.DeleteOfflineRecognizer(recognizer)

	log.Printf("Model loaded in %.2fs", time.Since(t0).Seconds())

	// Warm up with a short silence to initialize ONNX Runtime kernels
	warmup()

	http.HandleFunc("/transcribe", handleTranscribe)
	http.HandleFunc("/transcribe/upload", handleUpload)
	http.HandleFunc("/health", handleHealth)

	log.Printf("Moonshine service listening on :%s (model-in-memory, ~0.26s inference)", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func warmup() {
	samples := make([]float32, 16000) // 1 sec silence
	mu.Lock()
	defer mu.Unlock()
	stream := sherpa.NewOfflineStream(recognizer)
	stream.AcceptWaveform(16000, samples)
	recognizer.Decode(stream)
	sherpa.DeleteOfflineStream(stream)
	log.Println("Warmup complete")
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","model":"moonshine-tiny-en-int8","engine":"sherpa-onnx","version":%q,"commit":%q}`,
		version, commit)
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

	result := transcribeFile(req.AudioPath)
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

	result := transcribeFile(tmpFile)
	json.NewEncoder(w).Encode(result)
}

func transcribeFile(audioPath string) TranscribeResponse {
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

	mu.Lock()
	stream := sherpa.NewOfflineStream(recognizer)
	stream.AcceptWaveform(sampleRate, samples)
	recognizer.Decode(stream)
	result := stream.GetResult()
	sherpa.DeleteOfflineStream(stream)
	mu.Unlock()

	text := strings.TrimSpace(result.Text)
	return TranscribeResponse{
		Text:       text,
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
