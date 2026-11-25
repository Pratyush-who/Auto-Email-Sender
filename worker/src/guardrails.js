// worker/src/guardrails.js
import fs from 'node:fs'
import path from 'node:path'

const DEFAULT_MAX_RUNS_PER_DAY = 3
const DEFAULT_MAX_POSTS_PER_RUN = 60

export class Guardrails {
  constructor(storageDir = './data', options = {}) {
    this.storageDir = storageDir
    this.statsFile = path.join(storageDir, 'run_stats.json')
    this.isLocked = false
    this.activeRunId = null
    this.maxRunsPerDay = options.maxRunsPerDay ?? DEFAULT_MAX_RUNS_PER_DAY
    this.maxPostsPerRun = options.maxPostsPerRun ?? DEFAULT_MAX_POSTS_PER_RUN
    this.ensureStorage()
  }

  ensureStorage() {
    if (!fs.existsSync(this.storageDir)) {
      fs.mkdirSync(this.storageDir, { recursive: true })
    }
  }

  getTodayKey() {
    const now = new Date()
    return now.toISOString().slice(0, 10) // YYYY-MM-DD
  }

  loadStats() {
    try {
      if (fs.existsSync(this.statsFile)) {
        const raw = fs.readFileSync(this.statsFile, 'utf8')
        return JSON.parse(raw)
      }
    } catch {
      // Fallback
    }
    return {}
  }

  saveStats(stats) {
    try {
      this.ensureStorage()
      fs.writeFileSync(this.statsFile, JSON.stringify(stats, null, 2), 'utf8')
    } catch (err) {
      console.warn('Failed to persist run stats:', err)
    }
  }

  getTodayRunCount() {
    const stats = this.loadStats()
    const today = this.getTodayKey()
    return stats[today] || 0
  }

  acquireRunLock(runId = `run_${Date.now()}`) {
    if (this.isLocked) {
      return {
        allowed: false,
        reason: 'A scrape run is already in progress. Only single runs are permitted.',
        code: 'CONCURRENT_RUN_REJECTED',
      }
    }

    const todayCount = this.getTodayRunCount()
    if (todayCount >= this.maxRunsPerDay) {
      return {
        allowed: false,
        reason: `Daily quota reached (${todayCount}/${this.maxRunsPerDay} runs today). Reset occurs at midnight UTC.`,
        code: 'DAILY_LIMIT_EXCEEDED',
      }
    }

    this.isLocked = true
    this.activeRunId = runId

    // Increment today's count
    const stats = this.loadStats()
    const today = this.getTodayKey()
    stats[today] = (stats[today] || 0) + 1
    this.saveStats(stats)

    return {
      allowed: true,
      runId,
      todayRuns: stats[today],
      maxRunsToday: this.maxRunsPerDay,
    }
  }

  refundRun() {
    const stats = this.loadStats()
    const today = this.getTodayKey()
    if (stats[today] && stats[today] > 0) {
      stats[today] -= 1
      this.saveStats(stats)
    }
  }

  resetTodayQuota() {
    const stats = this.loadStats()
    const today = this.getTodayKey()
    stats[today] = 0
    this.saveStats(stats)
  }

  releaseRunLock() {
    this.isLocked = false
    this.activeRunId = null
  }

  clampMaxPosts(requested) {
    const n = Number.parseInt(requested, 10)
    if (Number.isNaN(n) || n <= 0) {
      return this.maxPostsPerRun
    }
    return Math.min(n, this.maxPostsPerRun)
  }

  detectCheckpointOrAuthWall(url = '', pageContent = '') {
    const lowerUrl = url.toLowerCase()
    const lowerContent = pageContent.toLowerCase()

    // URL checkpoints
    const checkpointUrlPatterns = [
      '/checkpoint/',
      '/challenge/',
      '/login',
      '/authwall',
      'trk=guest_homepage-basic',
      '/uas/login',
    ]

    for (const pattern of checkpointUrlPatterns) {
      if (lowerUrl.includes(pattern)) {
        return {
          detected: true,
          type: 'URL_CHECKPOINT',
          pattern,
          message: `LinkedIn redirected to verification URL: ${url}`,
        }
      }
    }

    // Page text checkpoint patterns
    const checkpointTextPatterns = [
      'security verification',
      'quick security check',
      'verify you are a human',
      "let's do a quick security check",
      'sign in to linkedin',
      'join linkedin',
      'please enter the code sent to',
      'we noticed unusual activity',
    ]

    for (const pattern of checkpointTextPatterns) {
      if (lowerContent.includes(pattern)) {
        return {
          detected: true,
          type: 'CONTENT_CHECKPOINT',
          pattern,
          message: `LinkedIn presented security challenge or login screen ("${pattern}")`,
        }
      }
    }

    return { detected: false }
  }
}
