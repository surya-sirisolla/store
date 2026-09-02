# Business Directory + WhatsApp AI Agent

A self-hosted product for a single business: manage your catalog of products or
services from a web console, and let an **AI WhatsApp agent** answer customers'
questions directly from that catalog.

Your staff enter listings in the console. Customers message your WhatsApp number
and get answers about what you stock, what it costs, where your branches are, and
get added to a waitlist when something is out of stock — all without a human
replying.

Built with Go, Next.js, PostgreSQL and [PicoClaw](https://picoclaw.io).

> **Single-tenant by design.** One deployment serves one business, with one
> WhatsApp number and one admin login. To run a second business, deploy a second
> stack. See [One business per deployment](#one-business-per-deployment).

---

## What you get

- **Owner console** (web) — business profile, categories/sub-categories, listings,
  and bulk Excel import where an LLM auto-maps your spreadsheet columns. One admin
  login, set at install time via `ADMIN_USER` / `ADMIN_PASSWORD`. "Staff" just
  registers a WhatsApp number so the bot recognizes that person for staff-only
  answers.
- **WhatsApp agent** — customers message your number; the agent answers from your
  directory and logs "notify me when available" requests.
- **Bot Monitor** — message counts, recent bot activity, and the waitlist.
- **Stock sync** — optional Livekeeping integration to keep quantities current.

### Privacy built into the bot

Customers never see prices or stock **numbers**. `search_listings` returns a
redacted view — name, category, description and an `in_stock` boolean — plus a
contact number for pricing. Staff and the owner console see the full record.

---

## Prerequisites

- **Docker** and Docker Compose
- **A PostgreSQL database** — this stack has no bundled Postgres. Use
  [Neon](https://neon.tech) (free tier works), RDS, or your own server.
- **An LLM API key** — Anthropic by default. Optional at install time; you can add
  it later in the console's **AI Providers** page.
- **A phone with WhatsApp** to pair the agent by scanning a QR code.

---

## Quick start

```bash
cp .env.example .env
```

Edit `.env` and set the two required values:

```bash
DATABASE_URL=postgresql://user:pass@host.neon.tech/db?sslmode=require
ADMIN_PASSWORD=pick-a-long-password
```

Then start everything:

```bash
docker compose up -d --build
```

- **Owner console** → http://localhost:3000 — sign in with `ADMIN_USER`
  (default `admin`) and your `ADMIN_PASSWORD`
- **Pair WhatsApp** → `docker compose logs -f picoclaw` and scan the QR code

A step-by-step walkthrough written for non-technical users is in
**[SETUP.md](SETUP.md)**.

---

## Configuration

Everything is set in `.env`. Only the first two are required.

| Variable | Default | What it does |
|---|---|---|
| `DATABASE_URL` | — | **Required.** Postgres connection string. Compose refuses to start without it |
| `ADMIN_PASSWORD` | — | **Required.** Console password. The server refuses to start without it |
| `ADMIN_USER` | `admin` | Console username |
| `JWT_SECRET` | generated | Signs session tokens. Generated and persisted to `/shared/jwt_secret` when blank |
| `ANTHROPIC_API_KEY` | — | LLM key. Can be set in the console instead |
| `FRONTEND_PORT` | `3000` | Console port |
| `BACKEND_PORT` | `8080` | API port |
| `PICOCLAW_PORT` | `18790` | Agent gateway port |
| `PUBLIC_API_URL` | `http://localhost:8080` | The URL your **browser** uses to reach the API. Baked in at build time |
| `SESSION_IDLE_HOURS` | `24` | How long a chat may idle before the cleanup job clears it |
| `INTERNAL_TOKEN` | `store-internal` | Guards internal-only endpoints on the Docker network |
| `SITE_ADDRESS` | — | Your domain, to turn on automatic HTTPS via Caddy (prod compose only) |
| `TZ` | `UTC` | Timezone for reminders and timestamps |

---

## Architecture

One `docker compose up` starts four services. **The database is not one of them** —
you supply a managed Postgres via `DATABASE_URL`.

| Service | What it does |
|---|---|
| `store-backend` | REST API for the console. Runs schema migrations on boot |
| `store-mcp` | Exposes your directory to the bot as MCP tools, and logs every call for monitoring |
| `store-frontend` | The console web UI (Next.js) |
| `picoclaw` | The WhatsApp agent (native QR pairing). Calls `store-mcp` over MCP |

```
Customer ──WhatsApp──▶ picoclaw (agent) ──MCP──▶ store-mcp ──┐
                                                              ├─▶ Postgres
Owner ──browser──▶ store-frontend ──REST──▶ store-backend ───┘   (managed, via
                                                                  DATABASE_URL)
```

The agent and the console share one database, so what the owner enters is exactly
what the bot answers from — and the bot's activity shows up in the console.

`store-backend` and `store-mcp` are the **same image** run with different commands
(`server` and `mcp`). Only the backend migrates; the MCP server waits on it so it
can't race that migration. PicoClaw in turn waits for the MCP server to report
healthy, because it connects once at startup and does not retry.

### MCP tools

The bot gets six tools. Four are for anyone who messages you:

| Tool | Purpose |
|---|---|
| `get_business_info` | Contact profile + branch locations with addresses |
| `search_listings` | Find items/services by keyword or category |
| `list_categories` | Help the customer browse |
| `request_alert` | Log a "notify me when available" request |

Two more are **staff-only** — `staff_recent_contacts` and `staff_pending_alerts`.
They're gated on a `_caller` argument that PicoClaw injects from the
WhatsApp-authenticated sender, so the model cannot grant itself access by asking.

---

## Login

The console has **one admin account**, configured at install time the way Grafana
takes `GF_SECURITY_ADMIN_USER` / `GF_SECURITY_ADMIN_PASSWORD`:

```bash
ADMIN_USER=admin          # optional, defaults to "admin"
ADMIN_PASSWORD=…          # REQUIRED — the server won't start without it
```

Both are read on the **first boot only**: the password is bcrypt-hashed into the
database, after which the database is the source of truth and the env value is
ignored. Rotate it from the console's **Security** page — a rotated password
survives restarts. Changing `ADMIN_USER` renames the existing account.

Login is rate-limited to 10 attempts per 5 minutes per IP.

To reset a forgotten password, clear the credential and restart — the account is
re-seeded from `ADMIN_PASSWORD`:

```bash
docker run --rm postgres:16-alpine psql "$DATABASE_URL" -c 'DELETE FROM auth_credentials;'
```

```bash
docker compose restart store-backend
```

---

## One business per deployment

This is a **single-tenant** product: one deployment serves one business, with one
WhatsApp number, one PicoClaw agent and one catalog. That's deliberate — the
infrastructure is single-tenant anyway (one pairing store, one set of LLM keys),
and collapsing the data model to match means the bot can never answer from the
wrong business's data.

To run a second business, deploy a second stack: its own Compose project, its own
database, and its own WhatsApp number.

---

## Security notes

- **Never commit your `.env`.** It's gitignored, along with `.env.prod` and
  `*.local.md`. Secrets belong in environment variables, not the repo.
- **`ADMIN_PASSWORD` has no default** and the server refuses to start without it,
  so a deployment can never come up on a well-known password.
- **Put it behind HTTPS** before exposing it publicly — set `SITE_ADDRESS` to your
  domain and Caddy provisions a Let's Encrypt certificate automatically. See
  [DEPLOY.md](DEPLOY.md).
- **The agent replies to anyone who messages your number** by default. Restrict it
  with PicoClaw's `allow_from` if you want a closed pilot.
- **Don't point the test suite at your real database** — it truncates every table.

---

## Development

```bash
# Backend (needs a reachable Postgres — DATABASE_URL, or the DB_* fallbacks)
cd backend && go run ./cmd/server      # API on :8080
cd backend && go run ./cmd/mcp         # MCP on :9090

# Frontend
cd frontend && npm install && npm run dev   # console on :3000
```

### Tests

Start a throwaway database — **never point the tests at your real `DATABASE_URL`,
they truncate every table**:

```bash
docker run -d --rm --name bb_testdb -e POSTGRES_USER=storeuser -e POSTGRES_PASSWORD=storepass -e POSTGRES_DB=storedb_test -p 5432:5432 postgres:16-alpine
```

```bash
cd backend && TEST_DB_NAME=storedb_test go test ./...
```

```bash
docker stop bb_testdb
```

> The tests **skip** (they don't fail) when the database is unreachable, so check
> for `ok` rather than the absence of `FAIL`.

---

## Project layout

```
backend/          Go API + MCP server (one binary each, one image)
  cmd/server/     REST API for the console
  cmd/mcp/        MCP tools the bot calls
  internal/       store (data access), handlers, services, models
frontend/         Next.js console
picoclaw/         Vendored PicoClaw agent (MIT) — see Credits
picoclaw-config/  Agent persona (AGENT.md) + startup script
docker-compose.yml       Local / single-server stack
docker-compose.prod.yml  Production stack (bundles its own Postgres)
```

---

## Deployment

[DEPLOY.md](DEPLOY.md) covers building and pushing images, updating a server, and
enabling HTTPS with Caddy. Replace the `YOUR_SERVER_IP` / `YOUR_DOCKERHUB_USER`
placeholders with your own.

> Note: `docker-compose.prod.yml` bundles its own Postgres and does **not** read
> `DATABASE_URL`. If you want production on a managed database, add `DATABASE_URL`
> to the `store-backend` and `store-mcp` environments there and drop the `postgres`
> service.

---

## Credits

The WhatsApp agent is [**PicoClaw**](https://picoclaw.io)
([github.com/sipeed/picoclaw](https://github.com/sipeed/picoclaw)), vendored under
`picoclaw/` and used under the MIT License — see
[`picoclaw/LICENSE`](picoclaw/LICENSE).

It's **vendored rather than pulled as a submodule** on purpose: this project builds
it with the `whatsapp_native` build tag (whatsmeow QR pairing, which the stock
Dockerfile omits) and pins its own tested dependency versions — whatsmeow in
particular is sensitive to WhatsApp protocol changes. Vendoring keeps a plain
`git clone` buildable and the pairing known-good.

The MCP server's tokenized search and waitlist tool are adapted from the Signet
reference backend.

## License

[MIT](LICENSE) © 2026 Jaya Surya Sirisolla.

The vendored PicoClaw agent under `picoclaw/` is separately licensed under MIT by
its own authors — see [`picoclaw/LICENSE`](picoclaw/LICENSE).
