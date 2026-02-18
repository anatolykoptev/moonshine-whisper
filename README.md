# moonshine-whisper

Fast speech-to-text HTTP service powered by [Moonshine](https://github.com/usefulsensors/moonshine) (Useful Sensors) via [sherpa-onnx](https://github.com/k2-fsa/sherpa-onnx) Go bindings.

**Beats faster-whisper Python 2.4× on ARM64 CPU.**

## Benchmark (JFK ~11 sec audio, ARM64, avg 3 runs)

| Solution | avg | min |
|---|---|---|
| **moonshine-whisper (this)** | **0.34s** | **0.29s** ⭐ |
| faster-whisper tiny.en int8 (Python) | 0.82s | 0.69s |
| whisper.cpp tiny-q8_0 (native CLI) | 1.45s | 1.30s |
| whisper.cpp tiny.en (native CLI) | 2.05s | 1.57s |

Model loaded once at startup — no reload overhead per request.

## Why Moonshine?

Moonshine is a speech recognition model from Useful Sensors (2024) optimized for real-time edge inference. Unlike Whisper, it processes only the actual audio duration instead of padding to 30 seconds, making it significantly faster for short-to-medium clips.

## Requirements

- Docker (recommended)
- **OR** Go 1.22+, CGO enabled, Linux ARM64 / AMD64

## Quick Start (Docker)

### 1. Download the model

```bash
curl -L -o moonshine-tiny.tar.bz2 \
  https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-moonshine-tiny-en-int8.tar.bz2
tar -xjf moonshine-tiny.tar.bz2
```

### 2. Run

```bash
docker run -d \
  --name moonshine-whisper \
  -p 127.0.0.1:8092:8092 \
  -v $(pwd)/sherpa-onnx-moonshine-tiny-en-int8:/models:ro \
  -v /path/to/audio:/audio:ro \
  ghcr.io/anatolykoptev/moonshine-whisper:latest
```

Or with docker compose — see [`docker-compose.example.yml`](docker-compose.example.yml).

### 3. Build from source

```bash
git clone https://github.com/anatolykoptev/moonshine-whisper
cd moonshine-whisper
go build -o moonshine-whisper .
MOONSHINE_MODELS_DIR=./sherpa-onnx-moonshine-tiny-en-int8 ./moonshine-whisper
```

## API

### `GET /health`

```json
{"status":"ok","model":"moonshine-tiny-en-int8","engine":"sherpa-onnx"}
```

### `POST /transcribe`

Transcribe a file accessible inside the container.

```bash
curl -s -X POST http://localhost:8092/transcribe \
  -H "Content-Type: application/json" \
  -d '{"audio_path": "/audio/sample.wav"}'
```

```json
{
  "text": "And so my fellow Americans, ask not what your country can do for you.",
  "duration_ms": 310
}
```

Supports any format ffmpeg can decode (mp3, mp4, ogg, flac, wav…).

### `POST /transcribe/upload`

Upload an audio file directly.

```bash
curl -s -X POST http://localhost:8092/transcribe/upload \
  -F "audio=@/path/to/audio.mp3"
```

## Configuration

| Env var | Default | Description |
|---|---|---|
| `MOONSHINE_PORT` | `8092` | HTTP listen port |
| `MOONSHINE_MODELS_DIR` | `/models` | Path to Moonshine model directory |

## Model directory structure

```
models/
├── preprocess.onnx
├── encode.int8.onnx
├── uncached_decode.int8.onnx
├── cached_decode.int8.onnx
└── tokens.txt
```

Download from sherpa-onnx releases:
- [`sherpa-onnx-moonshine-tiny-en-int8`](https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-moonshine-tiny-en-int8.tar.bz2) — 103 MB, English only
- [`sherpa-onnx-moonshine-base-en-int8`](https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-moonshine-base-en-int8.tar.bz2) — larger, higher accuracy

## Dependencies

- [sherpa-onnx-go](https://github.com/k2-fsa/sherpa-onnx-go) — Go bindings (bundles ONNX Runtime + sherpa-onnx-c-api)
- [ffmpeg](https://ffmpeg.org) — audio format conversion

## License

MIT
