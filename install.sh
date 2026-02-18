#!/usr/bin/env sh
# moonshine-whisper installer
# Usage: curl -fsSL https://raw.githubusercontent.com/anatolykoptev/moonshine-whisper/main/install.sh | sh
set -e

REPO="anatolykoptev/moonshine-whisper"
IMAGE="ghcr.io/${REPO}"
MODEL_URL="https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-moonshine-tiny-en-int8.tar.bz2"
MODEL_DIR="${HOME}/moonshine-model"
PORT="${MOONSHINE_PORT:-8092}"

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

# ─── Download Moonshine model ─────────────────────────────────────────────────
if [ -f "${MODEL_DIR}/tokens.txt" ]; then
  info "Model already present at ${MODEL_DIR}, skipping download."
else
  info "Downloading Moonshine tiny-en INT8 model (~103 MB) ..."
  TMP=$(mktemp -d)
  curl -fsSL --progress-bar -o "${TMP}/model.tar.bz2" "${MODEL_URL}"
  tar -xjf "${TMP}/model.tar.bz2" -C "${TMP}"
  mkdir -p "${MODEL_DIR}"
  cp "${TMP}"/sherpa-onnx-moonshine-tiny-en-int8/* "${MODEL_DIR}/"
  rm -rf "${TMP}"
  success "Model downloaded to ${MODEL_DIR}"
fi

# ─── Remove old container if present ─────────────────────────────────────────
if docker ps -a --format '{{.Names}}' | grep -q '^moonshine-whisper$'; then
  warn "Removing existing container 'moonshine-whisper' ..."
  docker rm -f moonshine-whisper >/dev/null
fi

# ─── Start container ──────────────────────────────────────────────────────────
info "Starting moonshine-whisper on port ${PORT} ..."
docker run -d \
  --name moonshine-whisper \
  --restart unless-stopped \
  -p "127.0.0.1:${PORT}:8092" \
  -v "${MODEL_DIR}:/models:ro" \
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
echo "  Transcribe:  curl -s -X POST http://127.0.0.1:${PORT}/transcribe \\"
echo "                 -H 'Content-Type: application/json' \\"
echo "                 -d '{\"audio_path\":\"/audio/file.wav\"}'"
echo ""
echo "  Logs:        docker logs -f moonshine-whisper"
echo "  Stop:        docker stop moonshine-whisper"
echo "  Remove:      docker rm -f moonshine-whisper"
