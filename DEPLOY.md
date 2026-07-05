# Deploying an update to the EC2 server

Updates the running production stack on the AWS EC2 box (`16.16.56.67`, served
HTTP over the IP via Caddy). Your data is preserved — Postgres, the WhatsApp
pairing, and saved API keys all live in named Docker volumes that survive the
update, and the backend auto-migrates new DB columns on boot.

The prod stack pulls **pre-built images from Docker Hub** (`sjsurya/store-*`).
Flow: **build+push from your Mac → scp any changed config → server pulls & restarts.**

Setup facts (fill in once, reused every deploy):

| Thing            | Value                                  |
| ---------------- | -------------------------------------- |
| Server IP        | `16.16.56.67`                          |
| SSH key          | `~/Downloads/store-key-2.pem`          |
| SSH user         | `ubuntu`                               |
| Server directory | `~/store`                              |
| Env file         | `~/store/.env`                         |
| Docker Hub user  | `sjsurya`                              |

---

## Step 1 — On your Mac: build & push the images

```bash
cd "$HOME/Working Directory/store"

docker login              # once per machine, as sjsurya
./deploy.sh               # builds all 3 images for linux/amd64 and pushes them
```

`deploy.sh` reads `DOCKERHUB_USER` + `PUBLIC_API_URL` from `.env.prod` and pushes
`store-backend`, `store-frontend`, `store-picoclaw`. (Cross-building amd64 on an
Apple-Silicon Mac is emulated and takes a few minutes — that's normal.)

## Step 2 — On your Mac: copy changed config to the server

Only needed when `docker-compose.prod.yml`, `Caddyfile`, or anything in
`picoclaw-config/` changed. Harmless to run every time.

```bash
cd "$HOME/Working Directory/store"

scp -i ~/Downloads/store-key-2.pem docker-compose.prod.yml Caddyfile \
    ubuntu@16.16.56.67:~/store/
scp -i ~/Downloads/store-key-2.pem picoclaw-config/* \
    ubuntu@16.16.56.67:~/store/picoclaw-config/
```

## Step 3 — On the server: pull & restart

```bash
ssh -i ~/Downloads/store-key-2.pem ubuntu@16.16.56.67

cd ~/store
sudo docker compose -f docker-compose.prod.yml --env-file .env pull
sudo docker compose -f docker-compose.prod.yml --env-file .env up -d

# Refresh the bot persona — AGENT.md is cached in the picoclaw volume and is NOT
# updated automatically (init only seeds it when the file is missing).
sudo docker cp picoclaw-config/AGENT.md bb_picoclaw:/root/.picoclaw/workspace/AGENT.md
sudo docker compose -f docker-compose.prod.yml restart picoclaw
```

## Step 4 — Verify

Run on the server (or swap `localhost` for `16.16.56.67` from your Mac):

```bash
sudo docker compose -f docker-compose.prod.yml ps      # every container Up
curl -s http://localhost/healthz                       # {"ok":true}
```

Then open `http://16.16.56.67` → log in. Check **WhatsApp** still shows
**connected** (pairing persists across updates); if it shows logged out, disable
then re-enable the bot on `/admin/whatsapp` and re-scan the QR.

---

## Notes

- **No re-pairing / no data loss** on a normal update — the `pgdata`,
  `picoclaw_data`, and `bot_state` volumes are untouched.
- **One-liner from your Mac** (build+push, copy config, then restart over SSH):
  ```bash
  cd "$HOME/Working Directory/store" && ./deploy.sh && \
  scp -i ~/Downloads/store-key-2.pem docker-compose.prod.yml Caddyfile ubuntu@16.16.56.67:~/store/ && \
  scp -i ~/Downloads/store-key-2.pem picoclaw-config/* ubuntu@16.16.56.67:~/store/picoclaw-config/ && \
  ssh -i ~/Downloads/store-key-2.pem ubuntu@16.16.56.67 \
    'cd ~/store && sudo docker compose -f docker-compose.prod.yml --env-file .env pull && \
     sudo docker compose -f docker-compose.prod.yml --env-file .env up -d && \
     sudo docker cp picoclaw-config/AGENT.md bb_picoclaw:/root/.picoclaw/workspace/AGENT.md && \
     sudo docker compose -f docker-compose.prod.yml restart picoclaw'
  ```
- **Rebuild only one service?** e.g. just the frontend:
  ```bash
  # Mac:
  docker buildx build --platform linux/amd64 \
    --build-arg NEXT_PUBLIC_API_URL=http://16.16.56.67 \
    -t sjsurya/store-frontend:latest ./frontend --push
  # Server:
  sudo docker compose -f docker-compose.prod.yml --env-file .env pull store-frontend
  sudo docker compose -f docker-compose.prod.yml --env-file .env up -d store-frontend
  ```
- **Rollback**: tag releases (e.g. `:v2`) instead of only `:latest`, so rollback
  is `docker pull sjsurya/store-backend:v1 && ... up -d`.
- **Console password**: set a strong `OWNER_PASSWORD` in `~/store/.env` for the
  **first boot** — it's hashed (bcrypt) into the database and the DB becomes the
  source of truth. After first boot the env value is ignored; rotate the password
  from the console under **Settings → Security** (you can then clear it from
  `.env`). Login is rate-limited to 10 attempts / 5 min per IP. A stable
  `JWT_SECRET` is still required (`openssl rand -hex 32`).

---

## Enable HTTPS (auto-TLS via Caddy)

Caddy provisions a free Let's Encrypt certificate automatically once it's given a
domain. You need a domain you control (a bare IP can't get a public cert).

1. **DNS**: add an `A` record for your domain (e.g. `store.example.com`) pointing
   at `16.16.56.67`, and make sure ports **80 and 443** are open in the EC2
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
