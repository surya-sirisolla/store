#!/usr/bin/env bash
# deploy.sh — build the production images on your Mac and push them to Docker Hub.
# Run from the repo root. Prereqs: `docker login` (Docker Hub) + Docker buildx.
#
# The EC2 server then pulls these images (see DEPLOY.md for the server-side steps).
#
# Override the target platform if your EC2 is Graviton/arm64:
#   PLATFORM=linux/arm64 ./deploy.sh
set -euo pipefail
cd "$(dirname "$0")"

# Pull DOCKERHUB_USER + PUBLIC_API_URL from .env.prod.
set -a; . ./.env.prod; set +a
: "${DOCKERHUB_USER:?set DOCKERHUB_USER in .env.prod}"
: "${PUBLIC_API_URL:?set PUBLIC_API_URL in .env.prod}"

# EC2 x86_64 -> linux/amd64 (default); AWS Graviton -> linux/arm64.
PLATFORM="${PLATFORM:-linux/amd64}"

echo "▶ Building for $PLATFORM   user=$DOCKERHUB_USER   frontend api=$PUBLIC_API_URL"
echo "  (cross-building for a different arch than your Mac is emulated and slow — be patient)"

# Backend image (also runs the MCP server via a different command in compose).
docker buildx build --platform "$PLATFORM" \
  -t "$DOCKERHUB_USER/store-backend:latest" ./backend --push

# Frontend image — bake the public API URL in at build time.
docker buildx build --platform "$PLATFORM" \
  --build-arg NEXT_PUBLIC_API_URL="$PUBLIC_API_URL" \
  -t "$DOCKERHUB_USER/store-frontend:latest" ./frontend --push

# PicoClaw WhatsApp agent (prod Dockerfile: whatsapp_native build tag + log seds).
docker buildx build --platform "$PLATFORM" \
  -f ./picoclaw/Dockerfile.prod \
  -t "$DOCKERHUB_USER/store-picoclaw:latest" ./picoclaw --push

echo "✅ Pushed $DOCKERHUB_USER/{store-backend,store-frontend,store-picoclaw}:latest"
echo "   Next: run the server-side steps in DEPLOY.md on the EC2 box."
