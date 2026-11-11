# Contributing

This project is deprecated: active development moved to
[ox-whisper](https://github.com/anatolykoptev/ox-whisper). Bug fixes and keeping
the Docker image building are welcome; new features belong in ox-whisper.

## Build and test

Requires Go (see `go.mod`), CGO and Linux (ARM64 or AMD64).

```sh
go build -o moonshine-whisper .
go vet ./...
go test ./...
```

Run it with `MOONSHINE_MODELS_DIR=./models/en ./moonshine-whisper` once the
models listed in the README are downloaded. The release workflow only runs on
tags, so run the tests yourself before opening a pull request.

## Commits and pull requests

Use Conventional Commits (`feat:`, `fix:`, `docs:`, `ci:`, `chore:`): release
notes are generated from them. Keep a change focused, describe how you tried it,
and update the README when an endpoint, field or environment variable changes.
The pull request template lists the checks.
