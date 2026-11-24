// worker/src/index.js
import 'dotenv/config'
import process from 'node:process'
import express from 'express'
import cors from 'cors'
import { Guardrails } from './guardrails.js'
import { scrapeLinkedInPosts } from './scraper.js'
import { processManualImport } from './manual.js'

const app = express()
const PORT = Number(process.env.PORT || process.env.WORKER_PORT || 3001)
const USER_DATA_DIR = process.env.USER_DATA_DIR || './user_data'

app.use(cors())
app.use(express.json({ limit: '10mb' }))

const guardrails = new Guardrails('./data')

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
    maxRunsPerDay: 3,
    maxPostsPerRun: 60,
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

    const result = await scrapeLinkedInPosts({
      query: query.trim(),
      timeFilter: timeFilter || '24h',
      sortBy: sortBy || 'relevance',
      maxPosts: clampedMaxPosts,
      userDataDir: USER_DATA_DIR,
      guardrails,
    })

    if (!result.success && result.alert === 'CAPTCHA_OR_LOGIN_WALL') {
      console.warn(`[Worker] Security challenge or login wall detected: ${result.message}`)
      return res.status(403).json({
        error: 'CAPTCHA_OR_LOGIN_WALL_DETECTED',
        message: result.message,
        url: result.url,
      })
    }

    res.json(result)
  } catch (err) {
    console.error('[Worker] Scrape failed with error:', err)
    res.status(500).json({
      error: 'SCRAPE_EXECUTION_ERROR',
      message: err.message,
    })
  } finally {
    guardrails.releaseRunLock()
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

app.listen(PORT, () => {
  console.log(`[EmailAuto Worker] Running on http://localhost:${PORT}`)
  console.log(`[EmailAuto Worker] Persistent Chrome User Data Dir: ${USER_DATA_DIR}`)
})
