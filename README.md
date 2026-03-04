# moonshine-whisper

[![Release](https://img.shields.io/github/v/release/anatolykoptev/moonshine-whisper)](https://github.com/anatolykoptev/moonshine-whisper/releases)
[![Docker](https://img.shields.io/badge/docker-ghcr.io-blue)](https://ghcr.io/anatolykoptev/moonshine-whisper)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go 1.26+](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](go.mod)

Fast **multilingual** (EN + RU) speech-to-text HTTP service powered by [Moonshine v2](https://github.com/usefulsensors/moonshine) (English) and [Zipformer](https://github.com/k2-fsa/sherpa-onnx) (Russian) via [sherpa-onnx](https://github.com/k2-fsa/sherpa-onnx) Go bindings.

**Both models loaded in-memory — no per-request cold start.**

## Benchmark (ARM64 CPU)

| Model | Language | Avg | RTF | Size |
|---|---|---|---|---|
| **Moonshine v2 base** (this) | 🇺🇸 EN | **0.57s** / 11s audio | **0.052** | 135 MB |
| **Zipformer-RU INT8** (this) | 🇷🇺 RU | **0.19s** / 7s audio | **0.026** ⭐ | 66 MB |
| faster-whisper tiny int8 (Python) | EN/RU | 0.82s / 11s audio | 0.075 | — |
| whisper.cpp tiny-q8_0 | EN | 1.45s | 0.132 | — |

> **Why not Whisper for Russian?** Whisper pads all audio to 30 seconds internally → slow on short clips. Zipformer-RU processes only actual audio duration.

## Install

### Option 1 — Docker manually

```bash
# Download Moonshine v2 base EN model
mkdir -p moonshine-models/en && cd moonshine-models/en
for f in encoder_model.ort decoder_model_merged.ort tokens.txt; do
  curl -LO "https://huggingface.co/csukuangfj2/sherpa-onnx-moonshine-base-en-quantized-2026-02-27/resolve/main/$f"
done
cd ../..

# Run (multi-arch: linux/amd64 + linux/arm64)
docker run -d \
  --name moonshine-whisper \
  --restart unless-stopped \
  -p 127.0.0.1:8092:8092 \
  -v $(pwd)/moonshine-models/en:/models:ro \
  ghcr.io/anatolykoptev/moonshine-whisper:latest
```

### Option 2 — docker compose

See [`docker-compose.example.yml`](docker-compose.example.yml).

### Option 3 — Build from source

Requires: Go 1.26+, CGO enabled, Linux (ARM64 or AMD64)

```bash
git clone https://github.com/anatolykoptev/moonshine-whisper
cd moonshine-whisper
go build -o moonshine-whisper .
MOONSHINE_MODELS_DIR=./models/en ./moonshine-whisper
```

## API

### `GET /health`

```bash
curl http://localhost:8092/health
```
```json
{"status":"ok","engine":"sherpa-onnx","version":"2.0.0",
 "languages":{"en":{"model":"moonshine-v2-base-en","ready":true},"ru":{"model":"zipformer-ru-int8","ready":true}}}
```

### `POST /transcribe` — path-based

Transcribe a file accessible inside the container (mount it via `-v`).

```bash
# English (default)
curl -s -X POST http://localhost:8092/transcribe \
  -H "Content-Type: application/json" \
  -d '{"audio_path": "/audio/sample.wav", "language": "en"}'

# Russian
curl -s -X POST http://localhost:8092/transcribe \
  -H "Content-Type: application/json" \
  -d '{"audio_path": "/audio/sample.wav", "language": "ru"}'
```

```json
{"text": "And so my fellow Americans, ask not what your country can do for you.", "duration_ms": 310}
```

Accepts any format ffmpeg can decode: mp3, mp4, ogg, flac, m4a, wav…

### `POST /transcribe/upload` — file upload

```bash
curl -s -X POST http://localhost:8092/transcribe/upload \
  -F "audio=@recording.mp3" \
  -F "language=ru"
```

## Configuration

| Env var | Default | Description |
|---|---|---|
| `MOONSHINE_PORT` | `8092` | HTTP listen port |
| `MOONSHINE_MODELS_DIR` | `/models` | Path to Moonshine v2 EN model directory |
| `ZIPFORMER_RU_DIR` | `/ru-models` | Path to Zipformer RU model directory (optional) |

## Models

| Model | Var | Size | Language | Download |
|---|---|---|---|---|
| moonshine-v2-base-en (quantized) | `MOONSHINE_MODELS_DIR` | 135 MB | 🇺🇸 EN | [HuggingFace](https://huggingface.co/csukuangfj2/sherpa-onnx-moonshine-base-en-quantized-2026-02-27) |
| zipformer-ru-2024-09-18 int8 | `ZIPFORMER_RU_DIR` | 66 MB | 🇷🇺 RU | [download](https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-zipformer-ru-2024-09-18.tar.bz2) |

## Stack

- [Moonshine v2](https://github.com/usefulsensors/moonshine) — ASR model (Useful Sensors)
- [sherpa-onnx](https://github.com/k2-fsa/sherpa-onnx) — inference framework
- [sherpa-onnx-go](https://github.com/k2-fsa/sherpa-onnx-go) — Go CGO bindings (bundles ONNX Runtime)
- [ffmpeg](https://ffmpeg.org) — audio format conversion

## License

MIT
