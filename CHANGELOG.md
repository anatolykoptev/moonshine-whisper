# Changelog

All notable changes to this project are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). The project is
deprecated in favour of ox-whisper; no further releases are planned.

## [Unreleased]

### Added

- Repository documents: CLAUDE.md, CONTRIBUTING, SECURITY, code of conduct, issue
  and pull request templates, documentation index.

### Changed

- README marks the project as deprecated in favour of ox-whisper.

## 2.1.0 - 2026-03-04

### Changed

- Punctuation switched from the 73 MB ct-transformer to a 7 MB CNN-BiLSTM model.

## 2.0.0 - 2026-03-04

### Changed

- English model migrated from Moonshine v1 tiny to Moonshine v2 base; eight
  languages supported (seven via Moonshine v2 plus Russian).

## 1.5.0 - 2026-03-03

### Added

- Text chunking (`max_chunk_len`), UTF-8 sanitization and a larger test suite.

### Fixed

- All golangci-lint findings; model directories consolidated.

### Changed

- Release notes are generated from Conventional Commits.

## 1.4.0 - 2026-02-18

### Added

- VAD chunked inference (chunks of up to 25 s) with a per-chunk hallucination guard.

### Fixed

- The VAD buffer size follows `MAX_AUDIO_DURATION_S` instead of a fixed 60 s.

## 1.3.0 - 2026-02-17

### Changed

- Production hardening refactor.

## 1.2.0 - 2026-02-17

### Added

- Silero VAD and a compression-ratio hallucination guard.

### Changed

- EN and RU models load in parallel (startup 7 s to 6 s).

## 1.1.0 - 2026-02-17

### Added

- Russian support through the Zipformer-RU INT8 model.

### Changed

- Release workflow: title format and manual dispatch.

## 1.0.1 - 2026-02-17

### Added

- CI/CD release workflow, one-line installer, multi-arch Dockerfile and
  goreleaser configuration.

## 1.0.0 - 2026-02-17

### Added

- Initial release: Moonshine speech-to-text HTTP service.
