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
- The server currently serves **plain HTTP** with a weak console password. Point a
  domain at `16.16.56.67` to enable free auto-HTTPS (change the `:80` block in
  `Caddyfile` to your domain), and raise `OWNER_PASSWORD` in `~/store/.env`.
