# Setup Guide — get your business bot running

This guide assumes **no technical background**. Follow the steps in order.
You will end up with:

- A **web console** to manage your business info and your product/service list.
- A **WhatsApp bot** that answers your customers from that list.
- A **monitor** page to see what the bot is doing.

You only need to do this once.

---

## Step 1 — Install Docker Desktop

Docker runs the whole system for you. Download and install it:

- **Windows / Mac:** https://www.docker.com/products/docker-desktop/
- After installing, **open Docker Desktop** and wait until it says "Engine running".

> You do not need to learn Docker. You just need it open in the background.

---

## Step 2 — Add your Anthropic (Claude) key

The bot uses Claude to understand messages. Get a key:

1. Go to https://console.anthropic.com → **API Keys** → **Create Key**.
2. Copy the key (it starts with `sk-ant-...`).

Now, in this folder:

1. Find the file **`.env.example`**. Make a copy of it and rename the copy to **`.env`**.
2. Open `.env` in any text editor (Notepad, TextEdit, VS Code).
3. Paste your key after `ANTHROPIC_API_KEY=` like this:
   ```
   ANTHROPIC_API_KEY=sk-ant-xxxxxxxxxxxxxxxxxxxxx
   ```
4. (Optional) Change `OWNER_NAME` / `OWNER_EMAIL` — just labels shown in the console, no login involved.
5. Save the file.

---

## Step 3 — Start everything

Open a terminal **in this folder** and run **one command**:

```bash
docker compose up -d --build
```

The first time, this takes a few minutes (it's building everything). When it
finishes, all parts are running. Check anytime with:

```bash
docker compose ps
```

---

## Step 4 — Add your business info & products

1. Open your browser to **http://localhost:3000**
2. Log in with the email/password you set in `.env`.
3. Go to **Business Profile** → fill in your name, address, hours, phone → **Save**.
4. Go to **Categories** → add a few categories (e.g. "Fans", "Lighting").
5. Go to **Listings** → add your products/services, OR use **Bulk Upload** to
   import an Excel file (the system auto-detects your columns with AI).

---

## Step 5 — Connect WhatsApp (scan once, from the console)

1. In the web console, open the **WhatsApp** page (left menu).
2. A **QR code** appears on screen, with a live status.
3. On your phone: open **WhatsApp → Settings → Linked Devices → Link a Device**,
   and **scan the QR code** shown in the console.
4. The page flips to **"WhatsApp is connected"** automatically. Done.

> Use the phone number you want customers to message. The QR refreshes itself, so
> if it expires just scan the new one.
>
> Prefer the terminal? `docker compose logs -f picoclaw` also shows the QR.

---

## Step 6 — Test it

- From another phone, send a WhatsApp message to your number, e.g.
  *"Do you have ceiling fans?"*
- The bot replies using your product list.
- Back in the web console, open **Bot Monitor** to see the message activity and any
  "notify me when available" requests customers leave.

---

## Everyday commands

| What you want | Command |
|---|---|
| Start everything | `docker compose up -d` |
| Stop everything | `docker compose down` |
| See if it's running | `docker compose ps` |
| Watch the bot / get the QR again | `docker compose logs -f picoclaw` |
| Update after changing `.env` | `docker compose up -d` |

---

## Troubleshooting

- **Port already in use?** Edit the `FRONTEND_PORT` / `BACKEND_PORT` values in `.env`,
  then run `docker compose up -d` again.
- **Bot doesn't reply?** Make sure your `ANTHROPIC_API_KEY` in `.env` is correct,
  then `docker compose restart picoclaw`.
- **Need the QR again?** `docker compose logs -f picoclaw` (it reappears until scanned).
- **Start fresh WhatsApp pairing?** `docker compose down` then
  `docker volume rm business-bot_picoclaw_data` then `docker compose up -d`.
