# EmailAuto Playwright Worker

Host-based Node.js + Playwright worker with persistent headed Chrome automation.

## Purpose
Scrapes LinkedIn content search results for hiring posts while applying strict guardrails:
* **Persistent Session**: Uses a persistent Chrome user data directory (`./user_data`). You log in manually once in the headed browser.
* **Single Run Concurrency**: Rejects concurrent scrape requests.
* **Daily Quota**: Strictly limits scraping to maximum 3 runs per day.
* **Post Cap**: Automatically limits extraction to max 60 posts per run.
* **Security Guardrail**: Immediately stops on any CAPTCHA, verification checkpoint, or login wall.
* **Manual Import**: Provides a fallback endpoint (`/manual-import`) to paste posts directly without scraping.

## Running the Worker

```bash
cd worker
npm install
npm start
```

The worker starts on `http://localhost:3001`.
