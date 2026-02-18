#!/usr/bin/env sh
# moonshine-whisper installer
# Usage: curl -fsSL https://raw.githubusercontent.com/anatolykoptev/moonshine-whisper/main/install.sh | sh
set -e

REPO="anatolykoptev/moonshine-whisper"
IMAGE="ghcr.io/${REPO}"
MODEL_URL="https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-moonshine-tiny-en-int8.tar.bz2"
RU_MODEL_URL="https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-zipformer-ru-2024-09-18.tar.bz2"
VAD_MODEL_URL="https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/silero_vad.onnx"
MODELS_BASE="${HOME}/moonshine-models"
MODEL_DIR="${MODELS_BASE}/en"
RU_MODEL_DIR="${MODELS_BASE}/ru"
VAD_DIR="${MODELS_BASE}/vad"
PORT="${MOONSHINE_PORT:-8092}"
INSTALL_RU="${INSTALL_RU:-1}"
INSTALL_VAD="${INSTALL_VAD:-1}"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'

info()    { printf "${BLUE}[moonshine-whisper]${NC} %s\n" "$1"; }
success() { printf "${GREEN}[moonshine-whisper]${NC} %s\n" "$1"; }
warn()    { printf "${YELLOW}[moonshine-whisper]${NC} %s\n" "$1"; }
die()     { printf "${RED}[moonshine-whisper] ERROR:${NC} %s\n" "$1" >&2; exit 1; }

# ─── Check dependencies ────────────────────────────────────────────────────────
command -v docker >/dev/null 2>&1 || die "Docker is required. Install from https://docs.docker.com/get-docker/"

# ─── Detect architecture ──────────────────────────────────────────────────────
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64)   ARCH_TAG="amd64" ;;
  aarch64|arm64)  ARCH_TAG="arm64" ;;
  *) die "Unsupported architecture: $ARCH" ;;
esac
info "Detected architecture: ${ARCH} (${ARCH_TAG})"

# ─── Pull Docker image ────────────────────────────────────────────────────────
info "Pulling Docker image ${IMAGE}:latest ..."
docker pull "${IMAGE}:latest"
success "Image pulled."

# ─── Download EN model (Moonshine) ───────────────────────────────────────────
if [ -f "${MODEL_DIR}/tokens.txt" ]; then
  info "EN model already present at ${MODEL_DIR}, skipping."
else
  info "Downloading Moonshine tiny-en INT8 model (~103 MB) ..."
  TMP=$(mktemp -d)
  curl -fsSL --progress-bar -o "${TMP}/model.tar.bz2" "${MODEL_URL}"
  tar -xjf "${TMP}/model.tar.bz2" -C "${TMP}"
  mkdir -p "${MODEL_DIR}"
  cp "${TMP}"/sherpa-onnx-moonshine-tiny-en-int8/* "${MODEL_DIR}/"
  rm -rf "${TMP}"
  success "EN model downloaded to ${MODEL_DIR}"
fi

# ─── Download RU model (Zipformer) ────────────────────────────────────────────
if [ "${INSTALL_RU}" = "1" ]; then
  if [ -f "${RU_MODEL_DIR}/tokens.txt" ]; then
    info "RU model already present at ${RU_MODEL_DIR}, skipping."
  else
    info "Downloading Zipformer RU INT8 model (~70 MB) ..."
    TMP=$(mktemp -d)
    curl -fsSL --progress-bar -o "${TMP}/ru-model.tar.bz2" "${RU_MODEL_URL}"
    tar -xjf "${TMP}/ru-model.tar.bz2" -C "${TMP}"
    mkdir -p "${RU_MODEL_DIR}"
    cp "${TMP}"/sherpa-onnx-zipformer-ru-2024-09-18/encoder.int8.onnx "${RU_MODEL_DIR}/encoder.int8.onnx"
    cp "${TMP}"/sherpa-onnx-zipformer-ru-2024-09-18/decoder.int8.onnx "${RU_MODEL_DIR}/decoder.int8.onnx"
    cp "${TMP}"/sherpa-onnx-zipformer-ru-2024-09-18/joiner.int8.onnx "${RU_MODEL_DIR}/joiner.int8.onnx"
    cp "${TMP}"/sherpa-onnx-zipformer-ru-2024-09-18/tokens.txt "${RU_MODEL_DIR}/tokens.txt"
    rm -rf "${TMP}"
    success "RU model downloaded to ${RU_MODEL_DIR}"
  fi
else
  info "Skipping RU model (INSTALL_RU=0). Russian transcription will be unavailable."
fi

# ─── Download Silero VAD model ───────────────────────────────────────────────
if [ "${INSTALL_VAD}" = "1" ]; then
  if [ -f "${VAD_DIR}/silero_vad.onnx" ]; then
    info "VAD model already present at ${VAD_DIR}, skipping."
  else
    info "Downloading Silero VAD model (~630 KB) ..."
    mkdir -p "${VAD_DIR}"
    curl -fsSL --progress-bar -o "${VAD_DIR}/silero_vad.onnx" "${VAD_MODEL_URL}"
    success "VAD model downloaded to ${VAD_DIR}"
  fi
else
  info "Skipping VAD model (INSTALL_VAD=0). Voice activity detection will be disabled."
fi

# ─── Remove old container if present ─────────────────────────────────────────
if docker ps -a --format '{{.Names}}' | grep -q '^moonshine-whisper$'; then
  warn "Removing existing container 'moonshine-whisper' ..."
  docker rm -f moonshine-whisper >/dev/null
fi

# ─── Start container ──────────────────────────────────────────────────────────
info "Starting moonshine-whisper on port ${PORT} ..."
RU_MOUNT=""
if [ "${INSTALL_RU}" = "1" ] && [ -f "${RU_MODEL_DIR}/tokens.txt" ]; then
  RU_MOUNT="-v ${RU_MODEL_DIR}:/ru-models:ro"
fi
VAD_MOUNT=""
if [ "${INSTALL_VAD}" = "1" ] && [ -f "${VAD_DIR}/silero_vad.onnx" ]; then
  VAD_MOUNT="-v ${VAD_DIR}:/vad:ro"
fi

docker run -d \
  --name moonshine-whisper \
  --restart unless-stopped \
  -p "127.0.0.1:${PORT}:8092" \
  -v "${MODEL_DIR}:/models:ro" \
  ${RU_MOUNT} \
  ${VAD_MOUNT} \
  "${IMAGE}:latest"

# ─── Health check ─────────────────────────────────────────────────────────────
info "Waiting for service to be ready ..."
TRIES=0
until curl -sf "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1; do
  TRIES=$((TRIES + 1))
  [ $TRIES -ge 30 ] && die "Service did not start in 30s. Run: docker logs moonshine-whisper"
  sleep 1
done

success "moonshine-whisper is running!"
echo ""
echo "  Health:      http://127.0.0.1:${PORT}/health"
echo "  Transcribe EN: curl -s -X POST http://127.0.0.1:${PORT}/transcribe \\"
echo "                   -H 'Content-Type: application/json' \\"
echo '                   -d '"'"'{"audio_path":"/audio/file.wav","language":"en"}'"'"
echo "  Transcribe RU: curl -s -X POST http://127.0.0.1:${PORT}/transcribe \\"
echo "                   -H 'Content-Type: application/json' \\"
echo '                   -d '"'"'{"audio_path":"/audio/file.wav","language":"ru"}'"'"
echo ""
echo "  Logs:        docker logs -f moonshine-whisper"
echo "  Stop:        docker stop moonshine-whisper"
echo "  Remove:      docker rm -f moonshine-whisper"
