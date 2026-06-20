#!/bin/bash
# Convenience wrapper around docker compose. See SETUP.md for the full guide.
set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'

if [ ! -f .env ]; then
  echo -e "${YELLOW}No .env found — creating one from .env.example.${NC}"
  cp .env.example .env
  echo -e "${YELLOW}Edit .env and paste your ANTHROPIC_API_KEY, then run ./start.sh again.${NC}"
  exit 1
fi

echo -e "${CYAN}Building and starting all services…${NC}"
docker compose up -d --build

echo ""
docker compose ps
echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}  Running!${NC}"
echo -e "  Owner console → ${CYAN}http://localhost:${FRONTEND_PORT:-3000}${NC}"
echo -e "  Connect WhatsApp → ${CYAN}docker compose logs -f picoclaw${NC}  (scan the QR)"
echo -e "  Stop everything → ${CYAN}docker compose down${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
