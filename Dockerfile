# ─── Stage 1: Go builder ──────────────────────────────────────────────────────
FROM golang:1.22-bookworm AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

# Extract sherpa-onnx shared libs — detect arch at build time (aarch64 / x86_64)
RUN ARCH=$(uname -m) && \
    case "$ARCH" in \
      aarch64) LIB_ARCH="aarch64-unknown-linux-gnu" ;; \
      x86_64)  LIB_ARCH="x86_64-unknown-linux-gnu" ;; \
      armv7l)  LIB_ARCH="arm-unknown-linux-gnueabihf" ;; \
      *) echo "Unsupported arch: $ARCH" && exit 1 ;; \
    esac && \
    LIB_SRC=$(find /go/pkg/mod -path "*/sherpa-onnx-go-linux*/lib/${LIB_ARCH}" -type d 2>/dev/null | head -1) && \
    echo "Sherpa libs: $LIB_SRC  (arch: $ARCH)" && \
    mkdir -p /sherpa-libs && \
    cp "$LIB_SRC"/libsherpa-onnx-c-api.so /sherpa-libs/ && \
    cp "$LIB_SRC"/libonnxruntime.so /sherpa-libs/

COPY . .
RUN CGO_ENABLED=1 go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" \
    -o /moonshine-whisper .

# ─── Stage 2: Runtime ─────────────────────────────────────────────────────────
FROM debian:bookworm-slim

ARG VERSION=dev
LABEL org.opencontainers.image.title="moonshine-whisper" \
      org.opencontainers.image.description="Fast speech-to-text HTTP service via Moonshine + sherpa-onnx" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.source="https://github.com/anatolykoptev/moonshine-whisper" \
      org.opencontainers.image.licenses="MIT"

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates ffmpeg curl && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /sherpa-libs/libsherpa-onnx-c-api.so /usr/lib/
COPY --from=builder /sherpa-libs/libonnxruntime.so /usr/lib/
RUN ldconfig

COPY --from=builder /moonshine-whisper /usr/local/bin/moonshine-whisper

VOLUME /models
VOLUME /ru-models
VOLUME /vad
EXPOSE 8092

ENV MOONSHINE_PORT=8092
ENV MOONSHINE_MODELS_DIR=/models
ENV ZIPFORMER_RU_DIR=/ru-models
ENV SILERO_VAD_MODEL=/vad/silero_vad.onnx

HEALTHCHECK --interval=15s --timeout=5s --start-period=45s --retries=3 \
    CMD curl -sf http://localhost:8092/health || exit 1

ENTRYPOINT ["moonshine-whisper"]
