#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${REPO_ROOT:-}"

if [ -z "${REPO_ROOT}" ]; then
  if [ -f "${SCRIPT_DIR}/docker/Dockerfile.launcher-local" ]; then
    REPO_ROOT="${SCRIPT_DIR}"
  elif [ -f "${SCRIPT_DIR}/picoclaw_cloudflare/docker/Dockerfile.launcher-local" ]; then
    REPO_ROOT="${SCRIPT_DIR}/picoclaw_cloudflare"
  elif [ -f "${SCRIPT_DIR}/picoclaw/docker/Dockerfile.launcher-local" ]; then
    REPO_ROOT="${SCRIPT_DIR}/picoclaw"
  elif [ -f "${SCRIPT_DIR}/../docker/Dockerfile.launcher-local" ]; then
    REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
  else
    echo "Could not find docker/Dockerfile.launcher-local."
    echo "Set REPO_ROOT=/path/to/repo, or put this script in the repo root, its scripts/ directory, or the parent directory that contains picoclaw_cloudflare/."
    exit 1
  fi
fi

REPO_ROOT="$(cd "${REPO_ROOT}" && pwd)"

PI_HOST="${PI_HOST:-93.7.116.223}"
PI_USER="${PI_USER:-haunt}"
PI_PORT="${PI_PORT:-22}"
PI_PROJECT_DIR="${PI_PROJECT_DIR:-/home/haunt/Projects/picoclaw_cloudflare}"
REMOTE_TAR_PATH="${REMOTE_TAR_PATH:-/home/haunt/picoclaw-launcher-local-linux-arm64.tar}"
IMAGE_TAG="${IMAGE_TAG:-picoclaw-launcher-local:latest}"
PLATFORM="${PLATFORM:-linux/arm64}"
LOCAL_TAR_PATH="${LOCAL_TAR_PATH:-${REPO_ROOT}/build/picoclaw-launcher-local-linux-arm64.tar}"
COMPOSE_FILE="${COMPOSE_FILE:-docker/docker-compose.yml}"
SERVICE_NAME="${SERVICE_NAME:-picoclaw-launcher}"
CONTAINER_NAME="${CONTAINER_NAME:-picoclaw-launcher-cloudflare}"

mkdir -p "$(dirname "${LOCAL_TAR_PATH}")"

echo "==> Building ${IMAGE_TAG} for ${PLATFORM} on this Mac"
docker buildx build \
  --platform "${PLATFORM}" \
  -f "${REPO_ROOT}/docker/Dockerfile.launcher-local" \
  -t "${IMAGE_TAG}" \
  --load \
  "${REPO_ROOT}"

echo "==> Saving image to ${LOCAL_TAR_PATH}"
docker save -o "${LOCAL_TAR_PATH}" "${IMAGE_TAG}"

echo "==> Copying tarball to ${PI_USER}@${PI_HOST}:${REMOTE_TAR_PATH}"
scp -P "${PI_PORT}" -o StrictHostKeyChecking=accept-new \
  "${LOCAL_TAR_PATH}" "${PI_USER}@${PI_HOST}:${REMOTE_TAR_PATH}"

read -r -d '' REMOTE_SCRIPT <<EOF_REMOTE || true
set -euo pipefail

if command -v docker-compose >/dev/null 2>&1; then
  COMPOSE_BIN="docker-compose"
else
  COMPOSE_BIN="docker compose"
fi

sudo docker load -i "${REMOTE_TAR_PATH}"
cd "${PI_PROJECT_DIR}"
sudo \$COMPOSE_BIN -f "${COMPOSE_FILE}" --profile launcher up -d --force-recreate --no-build "${SERVICE_NAME}"
sudo docker exec "${CONTAINER_NAME}" picoclaw version
rm -f "${REMOTE_TAR_PATH}"
EOF_REMOTE

echo "==> Loading image and restarting launcher on the Pi"
ssh -t -p "${PI_PORT}" -o StrictHostKeyChecking=accept-new \
  "${PI_USER}@${PI_HOST}" "${REMOTE_SCRIPT}"

echo "==> Deployment complete"
