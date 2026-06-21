#!/bin/bash
# Convenience wrapper around docker compose. See SETUP.md for the full guide.
set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'

if [ ! -f .env ]; then
  echo -e "${YELLOW}No .env found — creating one from .env.example.${NC}"
  cp .env.example .env
  echo -e "${YELLOW}Tip: the ANTHROPIC_API_KEY is optional here — you can add it now in .env,${NC}"
  echo -e "${YELLOW}or later in the Owner console under AI Providers. Continuing…${NC}"
  echo ""
fi

set -a; source .env; set +a

if [ "$1" = "seed" ]; then
  echo -e "${CYAN}Seeding shop data (business profile, categories, listings) into postgres…${NC}"
  docker compose up -d postgres
  docker compose exec -T postgres pg_isready -U "${DB_USER:-storeuser}" -d "${DB_NAME:-storedb}" >/dev/null
  docker compose exec -T postgres psql -U "${DB_USER:-storeuser}" -d "${DB_NAME:-storedb}" < backend/seed/shop_seed.sql
  echo -e "${GREEN}Done. Shop data loaded — WhatsApp/bot-runtime tables (users, contacts, bot_activities, alert_requests) were left untouched.${NC}"
  exit 0
fi

if [ "$1" = "dev" ]; then
  echo -e "${CYAN}Building and starting everything except the frontend…${NC}"
  docker compose up -d --build --scale store-frontend=0

  echo ""
  docker compose ps
  echo ""
  echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${GREEN}  Backend services running. Starting frontend in dev mode (hot reload)…${NC}"
  echo -e "  Owner console → ${CYAN}http://localhost:${FRONTEND_PORT:-3000}${NC}"
  echo -e "  Connect WhatsApp → ${CYAN}docker compose logs -f picoclaw${NC}  (scan the QR)"
  echo -e "  Stop backend services → ${CYAN}docker compose down${NC}"
  echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo ""

  cd frontend
  [ -d node_modules ] || npm install
  NEXT_PUBLIC_API_URL="${PUBLIC_API_URL:-http://localhost:8080}" \
    exec npm run dev -- -p "${FRONTEND_PORT:-3000}"
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
echo -e "  Frontend hot-reload dev mode → ${CYAN}./start.sh dev${NC}"
echo -e "  Re-seed shop data → ${CYAN}./start.sh seed${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
