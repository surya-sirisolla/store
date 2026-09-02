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

## Step 2 — Create your database

Your business data lives in a **Postgres database**, which is not included in this
system — you point it at one you own. The free tier of
[Neon](https://neon.tech) is the easiest option and takes about two minutes:

1. Go to https://neon.tech and sign up (free).
2. Create a project. Any name and region is fine — pick a region near you.
3. On the project dashboard, find the **connection string**. It looks like:
   ```
   postgresql://user:password@ep-something.aws.neon.tech/neondb?sslmode=require
   ```
4. **Copy it.** You'll paste it into your settings file in the next step.

> Already have a Postgres database (RDS, or one you run yourself)? Use its
> connection string instead — nothing else changes.

---

## Step 3 — Create your settings file

In this folder:

1. Find the file **`.env.example`**. Make a copy of it and rename the copy to **`.env`**.
2. Open `.env` in any text editor (Notepad, TextEdit, VS Code).

Now fill in three things.

**a) Your database** — paste the connection string from Step 2:

```
DATABASE_URL=postgresql://user:password@ep-something.aws.neon.tech/neondb?sslmode=require
```

**b) Your Claude key** — the bot uses Claude to understand messages. Go to
https://console.anthropic.com → **API Keys** → **Create Key**, copy it (it starts
with `sk-ant-...`), and paste it:

```
ANTHROPIC_API_KEY=sk-ant-xxxxxxxxxxxxxxxxxxxxx
```

**c) Your console password** — this is what you'll use to sign in:

```
ADMIN_USER=admin
ADMIN_PASSWORD=pick-a-long-password-here
```

Then **save the file**.

> `DATABASE_URL` and `ADMIN_PASSWORD` are both **required** — the system refuses
> to start without them, on purpose, so your console is never left open with a
> default password. `ADMIN_USER` can stay `admin` if you like.

> You only set the password here once. After the first start it's stored (safely
> hashed) in the database, and you change it later from the console's
> **Security** page instead of this file.

---

## Step 4 — Start everything

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

## Step 5 — Add your business info & products

1. Open your browser to **http://localhost:3000**
2. Log in with the **username** (`admin` unless you changed it) and the
   **password** you set in `.env`.
3. Go to **Business Profile** → fill in your name, address, hours, phone → **Save**.
4. Go to **Categories** → add a few categories (e.g. "Fans", "Lighting").
5. Go to **Listings** → add your products/services, OR use **Bulk Upload** to
   import an Excel file (the system auto-detects your columns with AI).

---

## Step 6 — Connect WhatsApp (scan once, from the console)

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

## Step 7 — Test it

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

- **Says `set DATABASE_URL in .env` and won't start?** Your `DATABASE_URL` line is
  missing or empty. Go back to Step 2 and Step 3 — the system has no database of
  its own and can't start without one.
- **Won't start, mentions `ADMIN_PASSWORD`?** You left the password blank. Set it
  in `.env` and run `docker compose up -d` again.
- **Can't log in?** Sign in with the **username** (`admin` by default), not an
  email address.
- **Port already in use?** Edit the `FRONTEND_PORT` / `BACKEND_PORT` values in `.env`,
  then run `docker compose up -d` again.
- **Bot doesn't reply?** Make sure your `ANTHROPIC_API_KEY` in `.env` is correct,
  then `docker compose restart picoclaw`.
- **Need the QR again?** `docker compose logs -f picoclaw` (it reappears until scanned).
- **Start fresh WhatsApp pairing?** `docker compose down` then
  `docker volume rm business-bot_picoclaw_data` then `docker compose up -d`.
