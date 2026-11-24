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
|                        Go Server (:8080)                                |
| - Chi Router, CORS, Structured Slog                                     |
| - Pipeline (Scrape -> Extract -> Filter -> Verify -> Compose -> Queue)  |
| - Orchestrator (2h ticker, "Armed" day model, sleep/restart recovery)   |
| - 30-min Thread Poller (detects replies, bounces & drafts follow-ups)   |
| - Safe Dispatcher (daily cap, send window, randomized gaps, dedupe)     |
+---------+--------------------------+-------------------------+----------+
          |                          |                         |
          v                          v                         v
+-------------------+      +-------------------+     +--------------------+
|  PostgreSQL 16    |      |  Gemini 2.5 Flash |     |  Gmail REST API    |
|  (Docker Compose) |      |  (Structured JSON)|     |  (OAuth 2.0 Code)  |
+-------------------+      +-------------------+     +--------------------+
          ^
          | HTTP (:3001)
+------------------------------------+
|  Playwright Worker (Host Node.js)  |
| - Persistent Headed Chrome Profile |
| - LinkedIn Session Cookie Storage  |
+------------------------------------+
```

- **Frontend (`/` & `/auto`)**: React 19 + Vite 8.
  - `/`: Original manual multi-recipient email sender.
  - `/auto`: Autonomous Job Hunter dashboard featuring live telemetry, armed safety status, daily cap meters, multiselect draft review, reply & bounce tracking, and run logs.
- **Go Server (`/server`)**: Go 1.22 with Chi router, pgxpool, and golang-migrate. Runs in Docker or as a native binary.
- **Worker (`/worker`)**: Node.js + Playwright running directly on your host machine with a persistent headed Chrome profile directory (`worker/user_data`) so LinkedIn logins survive restarts.
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

### Step 2: Start PostgreSQL & Database

Start the PostgreSQL 16 database using Docker Compose:
```bash
docker compose up -d postgres
```

The database container runs on `localhost:5432` and mounts migrations automatically on startup.

---

### Step 3: Start the Playwright Worker on the Host

> **IMPORTANT**: The worker must run on your host machine (not in Docker) to display the headed Chrome browser for manual LinkedIn authentication.

```bash
cd worker
npm install
npm start
```
The worker will listen on `http://localhost:3001`.

#### 🔑 First-Time LinkedIn Authentication Flow
1. On the very first run, Playwright opens a persistent Chrome window pointing to `worker/user_data`.
2. Navigate to [linkedin.com](https://www.linkedin.com) in that window and log in to your account.
3. Complete any two-factor verification.
4. Your session cookies and credentials remain securely persisted in `worker/user_data/`. You will not need to log in again.

---

### Step 4: Start the Go Server

You can run the Go server either directly or via Docker:

**Option A: Running Locally (Recommended for Development)**
```bash
cd server
go run cmd/api/main.go
```

**Option B: Running via Docker Compose**
```bash
docker compose up -d
```
The server will start on `http://localhost:8080`.

---

### Step 5: Start the Frontend UI

In the root directory:
```bash
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
