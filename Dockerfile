# ─── Stage 1: Go builder ──────────────────────────────────────────────────────
FROM golang:1.22-bookworm AS builder

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

# Extract the sherpa-onnx shared libs from the Go module cache so we can
# copy them into the runtime image with a predictable path.
RUN LIB_SRC=$(find /go/pkg/mod -path "*/sherpa-onnx-go-linux*/lib/aarch64-unknown-linux-gnu" -type d 2>/dev/null | head -1) && \
    echo "Sherpa lib dir: $LIB_SRC" && \
    mkdir -p /sherpa-libs && \
    cp "$LIB_SRC"/libsherpa-onnx-c-api.so /sherpa-libs/ && \
    cp "$LIB_SRC"/libonnxruntime.so /sherpa-libs/

COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /moonshine-service .

# ─── Stage 2: Runtime ─────────────────────────────────────────────────────────
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates ffmpeg curl && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /sherpa-libs/libsherpa-onnx-c-api.so /usr/lib/
COPY --from=builder /sherpa-libs/libonnxruntime.so /usr/lib/
RUN ldconfig

COPY --from=builder /moonshine-service /usr/local/bin/moonshine-service

VOLUME /models
EXPOSE 8092

ENV MOONSHINE_PORT=8092
ENV MOONSHINE_MODELS_DIR=/models

HEALTHCHECK --interval=15s --timeout=5s --start-period=30s --retries=3 \
    CMD curl -sf http://localhost:8092/health || exit 1

ENTRYPOINT ["moonshine-service"]
