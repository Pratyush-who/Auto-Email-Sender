# Autonomous Job Hunter & Email Auto Sender

A complete, full-stack autonomous job prospecting and cold outreach platform built with React 19, Go 1.22, PostgreSQL 16, Playwright, Gemini 2.5 Flash, and the Gmail API.

---

## 🏛 Architecture Overview

```
                      +-----------------------------+
                      |    Browser (React 19)       |
                      |  - "/"     : Manual Mailer  |
                      |  - "/auto" : Auto Dashboard |
                      +--------------+--------------+
                                     |
                           HTTP / REST (Port 8080)
                                     v
+------------------------------------+------------------------------------+
|                    Go Server (Docker :8080)                             |
| - Chi Router, CORS, Structured Slog                                     |
| - Pipeline (Scrape -> Extract -> Filter -> Verify -> Batch Compose)     |
| - Orchestrator (2h ticker, "Armed" day model, sleep/restart recovery)   |
| - 30-min Thread Poller (detects replies, bounces & drafts follow-ups)   |
| - Safe Dispatcher (100/day cap, send window, randomized gaps, dedupe)   |
+---------+--------------------------+-------------------------+----------+
          |                          |                         |
          v                          v                         v
+-------------------+      +-------------------+     +--------------------+
|  PostgreSQL 16    |      |  Gemini 2.5 Flash |     |  Gmail REST API    |
|  (Docker Compose) |      |  (Structured JSON)|     |  (OAuth 2.0 Code)  |
+-------------------+      +-------------------+     +--------------------+
          ^
          | HTTP (host.docker.internal:3001)
+------------------------------------+
|  Playwright Worker (Host Node.js)  |
| - Persistent Headed Chrome Profile |
| - LinkedIn Session Cookie Storage  |
+------------------------------------+
```

- **Frontend (`/` & `/auto`)**: React 19 + Vite 8 (runs in Docker `emailauto-web` on `:5173` or locally via `npm run dev`).
  - `/`: Original manual multi-recipient email sender.
  - `/auto`: Autonomous Job Hunter dashboard featuring live scrape/send telemetry, armed safety status, daily cap meters (default `100/day`), multiselect draft review, reply & bounce tracking, and run logs.
- **Go Server (`/server`)**: Go 1.22+ with Chi router, pgxpool, and golang-migrate (runs in Docker `emailauto-server` on `:8080` or natively).
- **Worker (`/worker`)**: Node.js + Playwright running directly on your host machine (`:3001`) with a persistent headed Chrome profile (`worker/user_data`) so LinkedIn logins survive restarts. The Dockerized Go server reaches it via `http://host.docker.internal:3001`.
- **LLM (`server/internal/llm`)**: Google Gemini 2.5 Flash via REST with schema enforcement (`application/json`), token & latency logging, and exponential backoff retry.
- **Gmail Service (`server/internal/gmail`)**: Server-side OAuth 2.0 with offline access, AES-GCM encrypted refresh token storage, RFC 2822 MIME compilation, and thread monitoring.

---

## 🚀 Quickstart & Setup Guide

### Prerequisites
- [Docker Desktop](https://www.docker.com/) (for PostgreSQL 16)
- [Node.js 18+](https://nodejs.org/) (for Vite frontend and Playwright worker)
- [Go 1.22+](https://golang.org/) (if running server locally outside Docker)
- Google Cloud Console project with Gmail API enabled
- Gemini API Key

---

### Step 1: Configure Environment Variables

Create `.env` in the root directory:
```env
# Frontend OAuth (for manual "/" route)
VITE_GOOGLE_CLIENT_ID=your-client-id.apps.googleusercontent.com
VITE_API_URL=http://localhost:8080
```

Create `server/.env` (or copy from `server/.env.example`):
```env
PORT=8080
DATABASE_URL=postgres://postgres:postgres@localhost:5432/emailauto?sslmode=disable
ENCRYPTION_KEY=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
GEMINI_API_KEY=your_gemini_api_key_here
GEMINI_MODEL=gemini-2.5-flash
WORKER_URL=http://localhost:3001
CORS_ALLOWED_ORIGINS=http://localhost:5173,http://127.0.0.1:5173
GOOGLE_CLIENT_ID=your-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-google-client-secret
GOOGLE_REDIRECT_URL=http://localhost:8080/auth/callback
FRONTEND_URL=http://localhost:5173
UPLOAD_DIR=./uploads
```

---

### ⚡ One-Command Startup (Recommended on Windows)

Once your `.env` and `server/.env` files are configured and Docker Desktop is running, start the entire stack (`postgres`, `server`, `web` in Docker + Playwright `worker` on host) with a single command from the root directory:

```bash
npm run start:all
```

What [`start.ps1`](start.ps1) does:
1. Runs `docker compose up -d --build` to start `emailauto-postgres` (`:5432`), `emailauto-server` (`:8080`), and `emailauto-web` (`:5173`).
2. Checks port `3001` and stops any stale worker process so startup never fails with `EADDRINUSE`.
3. Starts the Playwright Chrome Worker (`worker/src/index.js`) on `http://localhost:3001`.

Useful Docker commands:
```bash
# Rebuild and restart only the Go API server after backend edits
docker compose up -d --build server

# View logs for Docker services
docker compose logs -f server web

# Stop all Docker containers
docker compose down
```

---

### Manual Step-by-Step Setup

#### Step 2: Start Docker Services (PostgreSQL, Go Server, Web UI)

Start all containerized services using Docker Compose:
```bash
docker compose up -d --build
```
- `postgres` runs on `127.0.0.1:5432` with persistent volume `postgres_data` and automatic startup migrations.
- `server` runs on `127.0.0.1:8080` and connects to the host worker via `http://host.docker.internal:3001`.
- `web` runs on `127.0.0.1:5173`.

#### Step 3: Start the Playwright Worker on the Host

> **IMPORTANT**: The worker must run on your host machine (not in Docker) to display the headed Chrome browser for manual LinkedIn authentication.

```bash
cd worker
npm install
npm start
```
The worker will listen on `http://localhost:3001`.

##### 🔑 First-Time LinkedIn Authentication Flow
1. On the very first run, Playwright opens a persistent Chrome window pointing to `worker/user_data`.
2. Navigate to [linkedin.com](https://www.linkedin.com) in that window and log in to your account.
3. Complete any two-factor verification.
4. Your session cookies and credentials remain securely persisted in `worker/user_data/`. You will not need to log in again.

#### Step 4: (Optional) Run Server & Frontend Natively Outside Docker

If you prefer running the Go server or Vite frontend directly on your host:
```bash
# Go server (in ./server)
go run cmd/api/main.go

# Vite frontend (in root)
npm install
npm run dev
```

Open **[http://localhost:5173/auto](http://localhost:5173/auto)** in your browser.

---

### Step 6: Connect Google Gmail Account

1. Open `http://localhost:8080/auth/google` in your browser (or click the connect prompt in `/auto`).
2. Grant permissions for `gmail.send`, `gmail.readonly`, and `userinfo.email`.
3. You will be redirected back to the dashboard with your offline refresh token securely encrypted in PostgreSQL.

---

## ⚙️ How the Pipeline & Batching Work

1. **LinkedIn Scraper (`worker/src/scraper.js`, `worker/src/parser.js`)**:
   - Launches a persistent headed Chrome session (`worker/user_data`) and opens LinkedIn content search with your query, time filter (`24h` / `7d`), and sort order.
   - Scrolls through the feed (up to `250` scrolls or `maxPosts`, clicking `"Show more results"` when visible) and stops cleanly after 6 consecutive scrolls with no new posts.
   - Deduplicates by post URL and content fingerprint, extracting author, post text, timestamp, and direct post permalink.
2. **Extraction & Regex Cross-Check (`server/internal/pipeline/extractor.go`)**:
   - Runs fast deterministic regex & keyword rules alongside Gemini 2.5 Flash structured JSON extraction.
   - Extracts emails, phones, company, role, work mode, paid/unpaid status, stipend, required subject line, and flags `#OpenToWork` / job-seeker posts so they are never treated as hiring posts.
3. **Filtering & Company Verification (`server/internal/filter/filter.go`, `verifier.go`)**:
   - Applies your settings (skip unpaid, min stipend, work mode, allowed locations, experience bounds, include/exclude keywords).
   - Verifies email domain & computes a trust score (default threshold `40`).
4. **Batch Email Composition (`server/internal/pipeline/pipeline.go`, `server/internal/composer/composer.go`)**:
   - Deduplicates contacts by email and skips contacts who already have an initial outreach.
   - Groups eligible posts into batches of up to **15 posts per Gemini API call** (`ComposeBatch`) to save quota while generating tailored, company-specific subject lines and plain-text emails with your exact signature and no location/filler fluff.
5. **Approval, Queue & Gmail Dispatch (`server/internal/sender/sender.go`)**:
   - Drafts land in **Drafts & Approvals** (`approval_mode = true`) or directly in the send queue.
   - Dispatches via Gmail API with your uploaded PDF resume attached, enforcing the daily cap (default `100`), send window (`09:00-19:00 IST`), and randomized gaps (`2-6` min).

---

## 🛡️ Autonomous Safety Model & Controls

### 1. The "Armed" Safety Model
- **No Cold Start Sends**: Opening the app or restarting the server will **never** trigger autonomous outreach.
- **Explicit Daily Activation**: The 2-hour autonomous scheduler will execute **only if**:
  1. `Autopilot` toggle is switched **ON**.
  2. The engine has been **ARMED** by manually clicking **"Run Now"** at least once that calendar day.
- At midnight, the engine automatically disarms until your next manual run.

### 2. No Catch-Up On Sleep / Restart
- If your laptop goes to sleep, closes, or restarts, the scheduler will **never burst or catch up missed runs**.
- When restored, it immediately reschedules `next_run_at = now + 2h` and waits for the next clean tick.

### 3. Strict Human-in-the-Loop for Emails & Follow-ups
- By default, all generated outreach (initial emails and follow-ups) lands in the **Drafts & Approvals** tab.
- **Multiselect Approval**: You can select individual emails or click **"Select All"**:
  - `[✓ Approve Selected]`: Queues for scheduled dispatch respecting time gaps.
  - `[✈ Send Selected Now]`: Immediately dispatches via the Gmail API.
  - `[✕ Skip Selected]`: Skips unwanted opportunities.
- **No Automatic Follow-up Dispatch**: Follow-ups are drafted every 24h of no response, but are never sent without your approval.

### 4. Background Thread Poller (Every 30 min)
- Monitored Gmail threads are inspected for inbound replies.
- When an employer replies:
  - Outreach status changes to `REPLIED`.
  - Pending follow-up drafts are cancelled immediately.
  - The reply snippet appears in the **Replies & Tracker** tab.
- If a delivery failure / bounce occurs (`mailer-daemon`):
  - Outreach status changes to `FAILED`.
  - Recipient's trust score is set to `0` to permanently blacklist invalid addresses.

### 5. Emergency Kill Switch
- Clicking the red **KILL SWITCH** button immediately:
  - Halts all active and background pipeline runs.
  - Pauses the sender queue.
  - Disarms the engine and turns Autopilot **OFF**.

---

## 🧪 Testing & Verification

Run all test suites across the stack:

```bash
# 1. Run Go server tests (table-driven filter tests, orchestrator, poller, LLM, sender)
cd server
go test -v ./...

# 2. Run Playwright worker tests
cd ../worker
npm test

# 3. Lint and build React frontend
cd ..
npm run lint
npm run build
```

---

## 📄 License
MIT
