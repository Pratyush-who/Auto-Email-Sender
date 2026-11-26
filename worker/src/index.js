// worker/src/index.js
import 'dotenv/config'
import process from 'node:process'
import express from 'express'
import cors from 'cors'
import { Guardrails } from './guardrails.js'
import { scrapeLinkedInPosts, cancelCurrentScrape } from './scraper.js'
import { processManualImport } from './manual.js'

const app = express()
const PORT = Number(process.env.PORT || process.env.WORKER_PORT || 3001)
const USER_DATA_DIR = process.env.USER_DATA_DIR || './user_data'

app.use(cors())
app.use(express.json({ limit: '10mb' }))

const guardrails = new Guardrails('./data', { maxRunsPerDay: 100, maxPostsPerRun: 500 })

let liveProgress = {
  active: false,
  query: '',
  scrolls: 0,
  extractedCount: 0,
  message: '',
}

// Health check
app.get('/health', (req, res) => {
  res.json({
    status: 'ok',
    service: 'emailauto-worker',
    port: PORT,
  })
})

// Worker status & quota usage
app.get('/status', (req, res) => {
  res.json({
    isBusy: guardrails.isLocked,
    activeRunId: guardrails.activeRunId,
    runsToday: guardrails.getTodayRunCount(),
    maxRunsPerDay: guardrails.maxRunsPerDay,
    maxPostsPerRun: guardrails.maxPostsPerRun,
    progress: liveProgress,
  })
})

// Scrape LinkedIn posts with guardrails
app.post('/scrape', async (req, res) => {
  const { query, timeFilter, sortBy, maxPosts } = req.body

  if (!query || !query.trim()) {
    return res.status(400).json({ error: "Missing required field 'query'" })
  }

  // 1. Guardrails check: Concurrency lock and 3 runs/day quota
  const lock = guardrails.acquireRunLock()
  if (!lock.allowed) {
    const statusCode = lock.code === 'DAILY_LIMIT_EXCEEDED' ? 429 : 409
    return res.status(statusCode).json({
      error: lock.code,
      message: lock.reason,
      runsToday: guardrails.getTodayRunCount(),
    })
  }

  const clampedMaxPosts = guardrails.clampMaxPosts(maxPosts)

  try {
    console.log(`[Worker] Starting scrape run: query="${query}", filter=${timeFilter}, sort=${sortBy}, max=${clampedMaxPosts}`)
    liveProgress = {
      active: true,
      query: query.trim(),
      scrolls: 0,
      extractedCount: 0,
      message: 'Opening LinkedIn search results...',
    }

    const result = await scrapeLinkedInPosts({
      query: query.trim(),
      timeFilter: timeFilter || '24h',
      sortBy: sortBy || 'relevance',
      maxPosts: clampedMaxPosts,
      userDataDir: USER_DATA_DIR,
      guardrails,
      onProgress: (prog) => {
        liveProgress = {
          active: true,
          query: query.trim(),
          scrolls: prog.scroll,
          extractedCount: prog.extractedCount,
          message: prog.message,
        }
      },
    })

    if (!result.success && result.alert === 'CAPTCHA_OR_LOGIN_WALL') {
      console.warn(`[Worker] Security challenge or login wall detected: ${result.message}`)
      // Refund the run count so login failures don't consume user's daily quota
      guardrails.refundRun()
      return res.status(403).json({
        error: 'CAPTCHA_OR_LOGIN_WALL_DETECTED',
        message: result.message,
        url: result.url,
      })
    }

    res.json(result)
  } catch (err) {
    console.error('[Worker] Scrape failed with error:', err)
    guardrails.refundRun()
    res.status(500).json({
      error: 'SCRAPE_EXECUTION_ERROR',
      message: err.message,
    })
  } finally {
    liveProgress = { ...liveProgress, active: false }
    guardrails.releaseRunLock()
  }
})

// Cancel active scrape endpoint
app.post('/cancel', async (req, res) => {
  const cancelled = await cancelCurrentScrape()
  guardrails.releaseRunLock()
  res.json({
    success: true,
    cancelled,
    message: cancelled ? 'Scrape cancelled and Chrome closed' : 'No active scrape was running',
  })
})

// Reset quota endpoint (e.g. for testing / debugging)
app.post('/reset-quota', (req, res) => {
  guardrails.resetTodayQuota()
  res.json({
    success: true,
    message: 'Worker daily quota reset to 0 runs.',
    runsToday: guardrails.getTodayRunCount(),
  })
})

// Open persistent Chrome window on demand for manual LinkedIn login
app.post('/open-login', async (req, res) => {
  try {
    const { chromium } = await import('playwright')
    const path = await import('node:path')
    const resolvedUserDataDir = path.resolve(USER_DATA_DIR)

    let context
    try {
      context = await chromium.launchPersistentContext(resolvedUserDataDir, {
        headless: false,
        channel: 'chrome',
        viewport: { width: 1280, height: 850 },
        args: ['--disable-blink-features=AutomationControlled', '--start-maximized'],
      })
    } catch {
      context = await chromium.launchPersistentContext(resolvedUserDataDir, {
        headless: false,
        viewport: { width: 1280, height: 850 },
        args: ['--disable-blink-features=AutomationControlled'],
      })
    }

    const page = context.pages().length > 0 ? context.pages()[0] : await context.newPage()
    await page.goto('https://www.linkedin.com/login', { waitUntil: 'domcontentloaded' })

    res.json({ success: true, message: 'Chrome login window opened successfully' })
  } catch (err) {
    res.status(500).json({ error: 'FAILED_TO_OPEN_BROWSER', message: err.message })
  }
})

// Manual import endpoint (no-scrape fallback)
app.post('/manual-import', (req, res) => {
  try {
    const post = processManualImport(req.body)
    res.json({
      success: true,
      post,
    })
  } catch (err) {
    res.status(400).json({
      error: 'INVALID_MANUAL_IMPORT',
      message: err.message,
    })
  }
})

app.listen(PORT, '0.0.0.0', () => {
  console.log(`[EmailAuto Worker] Running on http://localhost:${PORT}`)
  console.log(`[EmailAuto Worker] Persistent Chrome User Data Dir: ${USER_DATA_DIR}`)
})
