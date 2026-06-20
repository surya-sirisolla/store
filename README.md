# Business Directory + WhatsApp Agent

A **self-hosted, single-tenant** product for one business. Manage your directory
of products/services from a web console, and let an **AI WhatsApp bot** answer your
customers' questions from that data — all brought up with one `docker compose up`.

> **New here? Read [SETUP.md](SETUP.md)** — a step-by-step guide written for non-technical users.

---

## What you get

- **Owner console** (web) — business profile, categories/sub-categories, listings,
  bulk Excel import (AI auto-maps columns). No login — open the URL, you're in,
  like PicoClaw itself. "Staff" just registers a WhatsApp number so the bot can
  recognize that person for staff-only answers.
- **WhatsApp agent** (PicoClaw) — customers message your number; the agent answers
  from your directory, and logs "notify me when available" requests.
- **Bot Monitor** — see message counts, recent bot activity, and waitlist requests.

## Architecture

One `docker compose up` starts five services:

| Service | What it does |
|---|---|
| `postgres` | Stores your business data |
| `store-backend` | REST API for the Owner console |
| `store-mcp` | Exposes your directory to the bot as MCP tools (`search_listings`, `get_business_info`, `list_categories`, `request_alert`) and logs every call for monitoring |
| `store-frontend` | The Owner console web UI (Next.js) |
| `picoclaw` | The WhatsApp agent (native QR pairing). Calls `store-mcp` over MCP to answer questions |

```
Customer ──WhatsApp──▶ picoclaw (agent) ──MCP──▶ store-mcp ──┐
                                                              ├─▶ Postgres
Owner ──browser──▶ store-frontend ──REST──▶ store-backend ───┘
```

The agent and the console share one Postgres, so what the owner enters is exactly
what the bot answers from — and the bot's activity shows up in the console.

## Quick start

```bash
cp .env.example .env      # then paste your ANTHROPIC_API_KEY
docker compose up -d --build
```

- Owner console → http://localhost:3000
- Scan the WhatsApp QR → `docker compose logs -f picoclaw`

Full walkthrough: **[SETUP.md](SETUP.md)**.

## Tech

- **Backend / MCP:** Go (Gin, GORM, `mark3labs/mcp-go`), PostgreSQL.
- **Frontend:** Next.js 16 (standalone build), Tailwind.
- **Agent:** [PicoClaw](https://picoclaw.io) built with the `whatsapp_native` tag
  (whatsmeow QR pairing), Claude as the model, connected to `store-mcp`.
- The MCP server's logic (tokenized search, the alert/waitlist tool) is adapted
  from the Signet reference backend.

## Development (without Docker)

```bash
# Postgres must be running and reachable (see .env for DB_* values).
cd backend && go run ./cmd/server      # API on :8080
cd backend && go run ./cmd/mcp         # MCP on :9090
cd frontend && npm install && npm run dev   # console on :3000

# Tests (needs a Postgres test DB named storedb_test):
cd backend && TEST_DB_NAME=storedb_test go test ./...
```

## No login

There's no auth wall — the console opens straight to the dashboard, the same
way PicoClaw itself runs unauthenticated and is gated only by the API keys you
configure. Don't expose this server's port beyond a network you trust.
