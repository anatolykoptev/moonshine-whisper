package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

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

	vadDetector *sherpa.VoiceActivityDetector
	muVAD       sync.Mutex
)

type appConfig struct {
	Port              string
	ModelsDir         string
	RUModelsDir       string
	VADModel          string
	NumThreads        int
	VADMinDurationS   float64
	MaxAudioDurationS float64
}

var cfg appConfig

func loadConfig() appConfig {
	threads := 4
	if s := os.Getenv("MOONSHINE_THREADS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			threads = n
		}
	}
	vadMin := 10.0
	if s := os.Getenv("VAD_MIN_DURATION_S"); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil && f >= 0 {
			vadMin = f
		}
	}
	maxAudio := 300.0
	if s := os.Getenv("MAX_AUDIO_DURATION_S"); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
			maxAudio = f
		}
	}
	return appConfig{
		Port:              envOr("MOONSHINE_PORT", "8092"),
		ModelsDir:         envOr("MOONSHINE_MODELS_DIR", "/models"),
		RUModelsDir:       envOr("ZIPFORMER_RU_DIR", "/ru-models"),
		VADModel:          envOr("SILERO_VAD_MODEL", "/vad/silero_vad.onnx"),
		NumThreads:        threads,
		VADMinDurationS:   vadMin,
		MaxAudioDurationS: maxAudio,
	}
}

func main() {
	cfg = loadConfig()

	t0 := time.Now()
	var wg sync.WaitGroup

	cfgEN := &sherpa.OfflineRecognizerConfig{}
	cfgEN.FeatConfig.SampleRate = 16000
	cfgEN.FeatConfig.FeatureDim = 80
	cfgEN.ModelConfig.Moonshine.Preprocessor = filepath.Join(cfg.ModelsDir, "preprocess.onnx")
	cfgEN.ModelConfig.Moonshine.Encoder = filepath.Join(cfg.ModelsDir, "encode.int8.onnx")
	cfgEN.ModelConfig.Moonshine.UncachedDecoder = filepath.Join(cfg.ModelsDir, "uncached_decode.int8.onnx")
	cfgEN.ModelConfig.Moonshine.CachedDecoder = filepath.Join(cfg.ModelsDir, "cached_decode.int8.onnx")
	cfgEN.ModelConfig.Tokens = filepath.Join(cfg.ModelsDir, "tokens.txt")
	cfgEN.ModelConfig.NumThreads = cfg.NumThreads
	cfgEN.ModelConfig.Provider = "cpu"
	cfgEN.DecodingMethod = "greedy_search"

	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.Now()
		recognizerEN = sherpa.NewOfflineRecognizer(cfgEN)
		if recognizerEN == nil {
			log.Fatalf("Failed to load EN model from %s", cfg.ModelsDir)
		}
		log.Printf("EN model loaded in %.2fs", time.Since(t).Seconds())
	}()

	ruEncoder := filepath.Join(cfg.RUModelsDir, "encoder.int8.onnx")
	if _, err := os.Stat(ruEncoder); err == nil {
		cfgRU := &sherpa.OfflineRecognizerConfig{}
		cfgRU.FeatConfig.SampleRate = 16000
		cfgRU.FeatConfig.FeatureDim = 80
		cfgRU.ModelConfig.Transducer.Encoder = ruEncoder
		cfgRU.ModelConfig.Transducer.Decoder = filepath.Join(cfg.RUModelsDir, "decoder.int8.onnx")
		cfgRU.ModelConfig.Transducer.Joiner = filepath.Join(cfg.RUModelsDir, "joiner.int8.onnx")
		cfgRU.ModelConfig.Tokens = filepath.Join(cfg.RUModelsDir, "tokens.txt")
		cfgRU.ModelConfig.NumThreads = cfg.NumThreads
		cfgRU.ModelConfig.Provider = "cpu"
		cfgRU.DecodingMethod = "greedy_search"

		wg.Add(1)
		go func() {
			defer wg.Done()
			t := time.Now()
			recognizerRU = sherpa.NewOfflineRecognizer(cfgRU)
			if recognizerRU != nil {
				log.Printf("RU model loaded in %.2fs", time.Since(t).Seconds())
			} else {
				log.Printf("WARNING: failed to load RU model")
			}
		}()
	} else {
		log.Printf("RU model not found at %s, RU transcription unavailable", cfg.RUModelsDir)
	}

	wg.Wait()
	log.Printf("All models loaded in %.2fs", time.Since(t0).Seconds())
	if recognizerEN != nil {
		defer sherpa.DeleteOfflineRecognizer(recognizerEN)
	}
	if recognizerRU != nil {
		defer sherpa.DeleteOfflineRecognizer(recognizerRU)
	}

	if _, err := os.Stat(cfg.VADModel); err == nil {
		vadCfg := &sherpa.VadModelConfig{
			SileroVad: sherpa.SileroVadModelConfig{
				Model:              cfg.VADModel,
				Threshold:          0.5,
				MinSilenceDuration: 0.5,
				MinSpeechDuration:  0.25,
				WindowSize:         512,
			},
			SampleRate: 16000,
			NumThreads: 1,
			Provider:   "cpu",
		}
		vadDetector = sherpa.NewVoiceActivityDetector(vadCfg, 60)
		if vadDetector != nil {
			defer sherpa.DeleteVoiceActivityDetector(vadDetector)
			log.Printf("Silero VAD loaded (min_duration=%.0fs)", cfg.VADMinDurationS)
		}
	} else {
		log.Printf("Silero VAD not found at %s (set SILERO_VAD_MODEL to enable)", cfg.VADModel)
	}

	warmup()

	mux := http.NewServeMux()
	mux.HandleFunc("/transcribe", handleTranscribe)
	mux.HandleFunc("/transcribe/upload", handleUpload)
	mux.HandleFunc("/health", handleHealth)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      loggingMiddleware(mux),
		ReadTimeout:  35 * time.Second,
		WriteTimeout: 35 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ruStatus := "unavailable"
	if recognizerRU != nil {
		ruStatus = "ready"
	}
	vadStatus := "disabled"
	if vadDetector != nil {
		vadStatus = "ready"
	}
	log.Printf("Service on :%s | EN: ready | RU: %s | VAD: %s", cfg.Port, ruStatus, vadStatus)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down...")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("Shutdown complete")
}

func warmup() {
	samples := make([]float32, 16000)

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

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
