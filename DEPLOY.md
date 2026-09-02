# Deploying an update to the EC2 server

Updates the running production stack on the AWS EC2 box (`YOUR_SERVER_IP`, served
HTTP over the IP via Caddy). Your data is preserved — Postgres, the WhatsApp
pairing, and saved API keys all live in named Docker volumes that survive the
update, and the backend auto-migrates new DB columns on boot.

> **Production runs its own Postgres.** Unlike the local `docker-compose.yml` —
> which has no database and points at a managed one via `DATABASE_URL` — the prod
> stack in `docker-compose.prod.yml` includes a `postgres` service and wires the
> backend to it with the discrete `DB_USER` / `DB_PASSWORD` / `DB_NAME` values.
> **`DATABASE_URL` is not passed through in prod and is ignored if you set it in
> `~/store/.env`.** To move production onto a managed database, add
> `DATABASE_URL` to the `store-backend` and `store-mcp` environments in
> `docker-compose.prod.yml` (it takes precedence over `DB_*`) and drop the
> `postgres` service and its `depends_on` entries.

The prod stack pulls **pre-built images from Docker Hub** (`YOUR_DOCKERHUB_USER/store-*`).
Flow: **build+push from your Mac → scp any changed config → server pulls & restarts.**

Setup facts (fill in once, reused every deploy):

| Thing            | Value                                  |
| ---------------- | -------------------------------------- |
| Server IP        | `YOUR_SERVER_IP`                          |
| SSH key          | `~/.ssh/your-key.pem`          |
| SSH user         | `ubuntu`                               |
| Server directory | `~/store`                              |
| Env file         | `~/store/.env`                         |
| Docker Hub user  | `YOUR_DOCKERHUB_USER`                              |

---

## Step 1 — On your Mac: build & push the images

```bash
cd /path/to/store

docker login              # once per machine, as YOUR_DOCKERHUB_USER
./deploy.sh               # builds all 3 images for linux/amd64 and pushes them
```

`deploy.sh` reads `DOCKERHUB_USER` + `PUBLIC_API_URL` from `.env.prod` and pushes
`store-backend`, `store-frontend`, `store-picoclaw`. (Cross-building amd64 on an
Apple-Silicon Mac is emulated and takes a few minutes — that's normal.)

## Step 2 — On your Mac: copy changed config to the server

Only needed when `docker-compose.prod.yml`, `Caddyfile`, or anything in
`picoclaw-config/` changed. Harmless to run every time.

```bash
cd /path/to/store

scp -i ~/.ssh/your-key.pem docker-compose.prod.yml Caddyfile \
    ubuntu@YOUR_SERVER_IP:~/store/
scp -i ~/.ssh/your-key.pem picoclaw-config/* \
    ubuntu@YOUR_SERVER_IP:~/store/picoclaw-config/
```

## Step 3 — On the server: pull & restart

```bash
ssh -i ~/.ssh/your-key.pem ubuntu@YOUR_SERVER_IP

cd ~/store
sudo docker compose -f docker-compose.prod.yml --env-file .env pull
sudo docker compose -f docker-compose.prod.yml --env-file .env up -d

# Refresh the bot persona — AGENT.md is cached in the picoclaw volume and is NOT
# updated automatically (init only seeds it when the file is missing).
sudo docker cp picoclaw-config/AGENT.md bb_picoclaw:/root/.picoclaw/workspace/AGENT.md
sudo docker compose -f docker-compose.prod.yml restart picoclaw
```

## Step 4 — Verify

Run on the server (or swap `localhost` for `YOUR_SERVER_IP` from your Mac):

```bash
sudo docker compose -f docker-compose.prod.yml ps      # every container Up
curl -s http://localhost/healthz                       # {"ok":true}
```

Then open `http://YOUR_SERVER_IP` → log in. Check **WhatsApp** still shows
**connected** (pairing persists across updates); if it shows logged out, disable
then re-enable the bot on `/admin/whatsapp` and re-scan the QR.

---

## Notes

- **No re-pairing / no data loss** on a normal update — the `pgdata`,
  `picoclaw_data`, and `bot_state` volumes are untouched.
- **One-liner from your Mac** (build+push, copy config, then restart over SSH):
  ```bash
  cd /path/to/store && ./deploy.sh && \
  scp -i ~/.ssh/your-key.pem docker-compose.prod.yml Caddyfile ubuntu@YOUR_SERVER_IP:~/store/ && \
  scp -i ~/.ssh/your-key.pem picoclaw-config/* ubuntu@YOUR_SERVER_IP:~/store/picoclaw-config/ && \
  ssh -i ~/.ssh/your-key.pem ubuntu@YOUR_SERVER_IP \
    'cd ~/store && sudo docker compose -f docker-compose.prod.yml --env-file .env pull && \
     sudo docker compose -f docker-compose.prod.yml --env-file .env up -d && \
     sudo docker cp picoclaw-config/AGENT.md bb_picoclaw:/root/.picoclaw/workspace/AGENT.md && \
     sudo docker compose -f docker-compose.prod.yml restart picoclaw'
  ```
- **Rebuild only one service?** e.g. just the frontend:
  ```bash
  # Mac:
  docker buildx build --platform linux/amd64 \
    --build-arg NEXT_PUBLIC_API_URL=http://YOUR_SERVER_IP \
    -t YOUR_DOCKERHUB_USER/store-frontend:latest ./frontend --push
  # Server:
  sudo docker compose -f docker-compose.prod.yml --env-file .env pull store-frontend
  sudo docker compose -f docker-compose.prod.yml --env-file .env up -d store-frontend
  ```
- **Rollback**: tag releases (e.g. `:v2`) instead of only `:latest`, so rollback
  is `docker pull YOUR_DOCKERHUB_USER/store-backend:v1 && ... up -d`.
- **Console login**: set `ADMIN_USER` (defaults to `admin`) and a strong
  `ADMIN_PASSWORD` in `~/store/.env`. `ADMIN_PASSWORD` is **required** — the
  server refuses to start without it, so a deployment can never come up on a
  default credential. It seeds the account on the **first boot** (bcrypt-hashed
  into the database, which then becomes the source of truth); afterwards the env
  value is ignored, so rotate the password from the console under
  **Settings → Security** and clear it from `.env`. Login is rate-limited to 10
  attempts / 5 min per IP.
  `JWT_SECRET` is optional — leave it blank and one is generated on first boot
  and persisted to `/shared/jwt_secret`; set it explicitly (`openssl rand -hex 32`)
  if you'd rather pin it.
- **Forgotten password**: clear the credential and restart to re-seed it from
  `ADMIN_PASSWORD`:
  ```bash
  cd ~/store
  sudo docker compose -f docker-compose.prod.yml --env-file .env exec postgres \
    psql -U "${DB_USER:-storeuser}" -d "${DB_NAME:-storedb}" \
    -c 'DELETE FROM auth_credentials;'
  sudo docker compose -f docker-compose.prod.yml --env-file .env restart store-backend
  ```
  (The `-f` flag matters: only `docker-compose.prod.yml` is copied to the server,
  so a bare `docker compose exec` finds no config file.)
- **One business per deployment**: this stack is single-tenant — one catalog, one
  WhatsApp number, one admin. Run a second business as a separate stack with its
  own database and number.

---

## Enable HTTPS (auto-TLS via Caddy)

Caddy provisions a free Let's Encrypt certificate automatically once it's given a
domain. You need a domain you control (a bare IP can't get a public cert).

1. **DNS**: add an `A` record for your domain (e.g. `store.example.com`) pointing
   at `YOUR_SERVER_IP`, and make sure ports **80 and 443** are open in the EC2
   security group. Wait for DNS to propagate (`dig +short store.example.com`).
2. **Rebuild the frontend** with the HTTPS URL baked in — set
   `PUBLIC_API_URL=https://store.example.com` in `.env.prod` on your Mac, then
   `./deploy.sh` (or just the frontend build). The API is same-origin (`/api`),
   so no port in the URL.
3. **Server**: set `SITE_ADDRESS=store.example.com` in `~/store/.env`, then:
   ```bash
   cd ~/store
   sudo docker compose -f docker-compose.prod.yml --env-file .env pull store-frontend
   sudo docker compose -f docker-compose.prod.yml --env-file .env up -d store-frontend caddy
   ```
   Caddy fetches the cert on first request (watch `sudo docker logs bb_caddy`);
   HTTP is redirected to HTTPS automatically. Leaving `SITE_ADDRESS` blank keeps
   plain HTTP on `:80` (dev / no domain).
