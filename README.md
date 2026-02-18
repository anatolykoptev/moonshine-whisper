# moonshine-whisper

[![Release](https://img.shields.io/github/v/release/anatolykoptev/moonshine-whisper)](https://github.com/anatolykoptev/moonshine-whisper/releases)
[![Docker](https://img.shields.io/badge/docker-ghcr.io-blue)](https://ghcr.io/anatolykoptev/moonshine-whisper)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go 1.22+](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](go.mod)

Fast speech-to-text HTTP service powered by [Moonshine](https://github.com/usefulsensors/moonshine) (Useful Sensors) via [sherpa-onnx](https://github.com/k2-fsa/sherpa-onnx) Go bindings.

**2.4× faster than faster-whisper Python on ARM64 CPU. Model loaded once — no per-request overhead.**

## Benchmark

JFK speech sample (~11 sec), ARM64 CPU, avg of 3 runs:

| Solution | avg | min |
|---|---|---|
| **moonshine-whisper** (this) | **0.34s** | **0.29s** ⭐ |
| faster-whisper tiny.en int8 (Python) | 0.82s | 0.69s |
| whisper.cpp tiny-q8_0 | 1.45s | 1.30s |
| whisper.cpp tiny.en | 2.05s | 1.57s |

> **Why Moonshine?** Unlike Whisper, Moonshine processes only the actual audio duration (no 30-second padding), making it significantly faster on short-to-medium clips.

## Install

### Option 1 — One-line (Docker required)

```bash
curl -fsSL https://raw.githubusercontent.com/anatolykoptev/moonshine-whisper/main/install.sh | sh
```

Downloads the model (~103 MB) and starts the container on `127.0.0.1:8092`.

### Option 2 — Docker manually

```bash
# Download model
curl -L https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-moonshine-tiny-en-int8.tar.bz2 \
  | tar -xj

# Run (multi-arch: linux/amd64 + linux/arm64)
docker run -d \
  --name moonshine-whisper \
  --restart unless-stopped \
  -p 127.0.0.1:8092:8092 \
  -v $(pwd)/sherpa-onnx-moonshine-tiny-en-int8:/models:ro \
  -v /path/to/audio:/audio:ro \
  ghcr.io/anatolykoptev/moonshine-whisper:latest
```

### Option 3 — docker compose

See [`docker-compose.example.yml`](docker-compose.example.yml).

### Option 4 — Build from source

Requires: Go 1.22+, CGO enabled, Linux (ARM64 or AMD64)

```bash
git clone https://github.com/anatolykoptev/moonshine-whisper
cd moonshine-whisper
go build -o moonshine-whisper .
MOONSHINE_MODELS_DIR=./sherpa-onnx-moonshine-tiny-en-int8 ./moonshine-whisper
```

## API

### `GET /health`

```bash
curl http://localhost:8092/health
```
```json
{"status":"ok","model":"moonshine-tiny-en-int8","engine":"sherpa-onnx","version":"1.0.0","commit":"abc1234"}
```

### `POST /transcribe` — path-based

Transcribe a file accessible inside the container (mount it via `-v`).

```bash
curl -s -X POST http://localhost:8092/transcribe \
  -H "Content-Type: application/json" \
  -d '{"audio_path": "/audio/sample.wav"}'
```

```json
{"text": "And so my fellow Americans, ask not what your country can do for you.", "duration_ms": 310}
```

Accepts any format ffmpeg can decode: mp3, mp4, ogg, flac, m4a, wav…

### `POST /transcribe/upload` — file upload

```bash
curl -s -X POST http://localhost:8092/transcribe/upload \
  -F "audio=@recording.mp3"
```

## Configuration

| Env var | Default | Description |
|---|---|---|
| `MOONSHINE_PORT` | `8092` | HTTP listen port |
| `MOONSHINE_MODELS_DIR` | `/models` | Path to Moonshine model directory |

## Models

| Model | Size | Language | Download |
|---|---|---|---|
| moonshine-tiny-en-int8 | 103 MB | English | [download](https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-moonshine-tiny-en-int8.tar.bz2) |
| moonshine-base-en-int8 | ~200 MB | English | [download](https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-moonshine-base-en-int8.tar.bz2) |

Expected directory structure (`/models`):
```
preprocess.onnx
encode.int8.onnx
uncached_decode.int8.onnx
cached_decode.int8.onnx
tokens.txt
```

## Stack

- [Moonshine](https://github.com/usefulsensors/moonshine) — ASR model (Useful Sensors, 2024)
- [sherpa-onnx](https://github.com/k2-fsa/sherpa-onnx) — inference framework
- [sherpa-onnx-go](https://github.com/k2-fsa/sherpa-onnx-go) — Go CGO bindings (bundles ONNX Runtime)
- [ffmpeg](https://ffmpeg.org) — audio format conversion

## License

MIT
