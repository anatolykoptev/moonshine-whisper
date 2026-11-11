# moonshine-whisper — notes for coding agents

Index of every document: [docs/INDEX.md](docs/INDEX.md).

This project is deprecated in favour of ox-whisper (Rust); change it only to fix
a bug or keep the image building.

## Layout

A single Go `main` package: `main.go` (config from environment, model loading,
HTTP routes), `handlers.go` (`/health`, `/transcribe`, `/transcribe/upload`),
`transcribe.go` (sherpa-onnx recognition, VAD chunking, hallucination guard),
`punctuate.go` (CNN-BiLSTM punctuation), `chunking.go` (`max_chunk_len`).
`patches/` holds replacement files for the sherpa-onnx Go bindings that the
Dockerfile copies in.

## Gate

CI (`.github/workflows/release.yml`) runs only on `v*` tags and on manual
dispatch: it builds and pushes the multi-arch image to ghcr.io and creates the
GitHub release. It does not run tests, so run them yourself:

```sh
go vet ./...
go test ./...      # CGO and the sherpa-onnx native libraries are required
```

## Invariants easy to break

- The binary needs CGO and Linux; the sherpa-onnx native libraries come from the
  Go bindings, so a build on another OS will not link.
- Config is read from environment variables in `main.go` (`envOr`): the model
  directories and the optional VAD and punctuation models have container paths
  as defaults, and a missing optional model disables that feature rather than
  failing startup.
- Request fields `language`, `vad`, `punctuate` and `max_chunk_len` are part of
  the API in the README; keep the README table in step with the handlers.
- `.goreleaser.yaml` builds no binary (`skip: true`); distribution is the Docker
  image only.
