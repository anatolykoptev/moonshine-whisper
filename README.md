# moonshine-whisper

[![Release](https://img.shields.io/github/v/release/anatolykoptev/moonshine-whisper)](https://github.com/anatolykoptev/moonshine-whisper/releases)
[![Docker](https://img.shields.io/badge/docker-ghcr.io-blue)](https://ghcr.io/anatolykoptev/moonshine-whisper)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go 1.26+](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](go.mod)

Fast **multilingual** speech-to-text HTTP service — 8 languages, Silero VAD, auto-punctuation. Powered by [Moonshine v2](https://github.com/usefulsensors/moonshine) and [Zipformer](https://github.com/k2-fsa/sherpa-onnx) via [sherpa-onnx](https://github.com/k2-fsa/sherpa-onnx) Go bindings.

All models loaded in-memory — no per-request cold start.

## Features

- **8 languages** — AR, EN, ES, JA, UK, VI, ZH (Moonshine v2) + RU (Zipformer)
- **Silero VAD** — auto-detects speech segments, skips silence
- **Punctuation** — CNN-BiLSTM model (7 MB INT8) with truecasing, auto for English
- **Hallucination guard** — compression ratio filter on each chunk
- **Text chunking** — split long transcripts via `max_chunk_len`
- **Any audio format** — ffmpeg converts mp3, ogg, flac, m4a, mp4, wav...

## Supported Languages

| Language | Code | Model |
|---|---|---|
| Arabic | `ar` | Moonshine v2 base (multilingual, 135 MB) |
| English | `en` | Moonshine v2 base — default, best quality |
| Spanish | `es` | Moonshine v2 base |
| Japanese | `ja` | Moonshine v2 base |
| Ukrainian | `uk` | Moonshine v2 base |
| Vietnamese | `vi` | Moonshine v2 base |
| Chinese | `zh` | Moonshine v2 base |
| Russian | `ru` | Zipformer-RU INT8 (dedicated, 66 MB) |

## Benchmark (ARM64 CPU)

| Model | Language | Latency | RTF | Size |
|---|---|---|---|---|
| Moonshine v2 base | EN | 0.57s / 11s audio | 0.052 | 135 MB |
| Zipformer-RU INT8 | RU | 0.19s / 7s audio | 0.026 | 66 MB |
| faster-whisper tiny int8 | EN/RU | 0.82s / 11s audio | 0.075 | — |
| whisper.cpp tiny-q8_0 | EN | 1.45s | 0.132 | — |

## Install

### Docker

```bash
docker run -d \
  --name moonshine-whisper \
  --restart unless-stopped \
  -p 127.0.0.1:8092:8092 \
  -v /path/to/models/en:/models:ro \
  -v /path/to/models/ru:/ru-models:ro \
  -v /path/to/models/vad:/vad:ro \
  -v /path/to/models/punct:/punct:ro \
  ghcr.io/anatolykoptev/moonshine-whisper:latest
```

### docker compose

See [`docker-compose.example.yml`](docker-compose.example.yml).

### Build from source

Requires: Go 1.26+, CGO enabled, Linux (ARM64 or AMD64)

```bash
git clone https://github.com/anatolykoptev/moonshine-whisper
cd moonshine-whisper
go build -o moonshine-whisper .
MOONSHINE_MODELS_DIR=./models/en ./moonshine-whisper
```

## API

### `GET /health`

```json
{"status":"ok","engine":"sherpa-onnx","version":"2.0.0",
 "vad":true,"punctuation":true,
 "languages":{"en":{"model":"moonshine-v2-base-en","ready":true},
              "ru":{"model":"zipformer-ru-int8","ready":true}}}
```

### `POST /transcribe` — path-based

```bash
curl -s -X POST http://localhost:8092/transcribe \
  -H "Content-Type: application/json" \
  -d '{"audio_path":"/audio/sample.wav","language":"en"}'
```

Optional fields: `language` (default: `en`), `vad` (bool, default: auto), `punctuate` (bool, default: auto for EN), `max_chunk_len` (int, split text into chunks).

### `POST /transcribe/upload` — file upload

```bash
curl -s -X POST http://localhost:8092/transcribe/upload \
  -F "audio=@recording.ogg" \
  -F "language=ru"
```

Optional form fields: `language`, `vad`, `punctuate`, `max_chunk_len`.

### Response

```json
{"text":"transcribed text","duration_ms":310,"speech_ms":8500,"chunks":["chunk1","chunk2"]}
```

`speech_ms` — present when VAD is active. `chunks` — present when `max_chunk_len` is set.

## Configuration

| Env var | Default | Description |
|---|---|---|
| `MOONSHINE_PORT` | `8092` | HTTP listen port |
| `MOONSHINE_MODELS_DIR` | `/models` | Moonshine v2 model directory |
| `ZIPFORMER_RU_DIR` | `/ru-models` | Zipformer RU model directory (optional) |
| `SILERO_VAD_MODEL` | `/vad/silero_vad.onnx` | Silero VAD model path (optional) |
| `PUNCT_MODEL` | `/punct/model.int8.onnx` | Punctuation model path (optional) |
| `PUNCT_VOCAB` | `/punct/bpe.vocab` | Punctuation BPE vocab path (optional) |
| `MOONSHINE_THREADS` | `4` | Inference threads per model |
| `VAD_MIN_DURATION_S` | `10` | Min audio duration (sec) to auto-enable VAD |
| `MAX_AUDIO_DURATION_S` | `300` | Max audio duration (sec), rejects longer files |

## Models

| Model | Env var | Size | Download |
|---|---|---|---|
| Moonshine v2 base (quantized) | `MOONSHINE_MODELS_DIR` | 135 MB | [HuggingFace](https://huggingface.co/csukuangfj2/sherpa-onnx-moonshine-base-en-quantized-2026-02-27) |
| Zipformer-RU INT8 | `ZIPFORMER_RU_DIR` | 66 MB | [sherpa-onnx releases](https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-zipformer-ru-2024-09-18.tar.bz2) |
| Silero VAD | `SILERO_VAD_MODEL` | 2 MB | bundled in Docker image |
| CNN-BiLSTM punct (EN) | `PUNCT_MODEL` + `PUNCT_VOCAB` | 7 MB | [sherpa-onnx releases](https://github.com/k2-fsa/sherpa-onnx/releases/download/punctuation-models/sherpa-onnx-online-punct-en-2024-08-06.tar.bz2) |

## Stack

- [Moonshine v2](https://github.com/usefulsensors/moonshine) — multilingual ASR model (Useful Sensors)
- [sherpa-onnx](https://github.com/k2-fsa/sherpa-onnx) — inference framework
- [sherpa-onnx-go](https://github.com/k2-fsa/sherpa-onnx-go) — Go CGO bindings (bundles ONNX Runtime)
- [Silero VAD](https://github.com/snakers4/silero-vad) — voice activity detection
- [ffmpeg](https://ffmpeg.org) — audio format conversion

## License

MIT
