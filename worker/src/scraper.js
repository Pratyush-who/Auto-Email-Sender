// worker/src/scraper.js
import path from 'node:path'
import { chromium } from 'playwright'
import { buildLinkedInSearchUrl, extractPostsFromPage } from './parser.js'

function randomDelay(minMs, maxMs) {
  const delay = Math.floor(Math.random() * (maxMs - minMs + 1)) + minMs
  return new Promise((resolve) => setTimeout(resolve, delay))
}

/**
 * Scrapes LinkedIn search results using a persistent headed Chrome session.
 */
export async function scrapeLinkedInPosts({
  query,
  timeFilter = '24h',
  sortBy = 'relevance',
  maxPosts = 60,
  userDataDir = './user_data',
  guardrails,
}) {
  const resolvedUserDataDir = path.resolve(userDataDir)
  const targetUrl = buildLinkedInSearchUrl({ query, timeFilter, sortBy })

  let context = null
  let page = null
  const extractedPosts = []
  const seenUrls = new Set()

  try {
    // Launch persistent headed context so cookies and session persist across runs
    try {
      context = await chromium.launchPersistentContext(resolvedUserDataDir, {
        headless: false,
        channel: 'chrome',
        viewport: { width: 1280, height: 850 },
        args: [
          '--disable-blink-features=AutomationControlled',
          '--start-maximized',
        ],
      })
    } catch {
      // Fallback to bundled Chromium if Google Chrome channel is not found
      context = await chromium.launchPersistentContext(resolvedUserDataDir, {
        headless: false,
        viewport: { width: 1280, height: 850 },
        args: ['--disable-blink-features=AutomationControlled'],
      })
    }

    page = context.pages().length > 0 ? context.pages()[0] : await context.newPage()

    // Navigate to LinkedIn Content Search
    await page.goto(targetUrl, { waitUntil: 'domcontentloaded', timeout: 45000 })
    await randomDelay(2000, 3500)

    // Check for CAPTCHA / checkpoint / login wall
    const currentUrl = page.url()
    const pageContent = await page.content()
    const check = guardrails.detectCheckpointOrAuthWall(currentUrl, pageContent)
    if (check.detected) {
      return {
        success: false,
        alert: 'CAPTCHA_OR_LOGIN_WALL',
        message: check.message,
        url: currentUrl,
        posts: [],
      }
    }

    // Scroll loop with randomized human-like delays
    let previousCount = 0
    let noNewContentRounds = 0
    const maxScrolls = 20

    for (let scroll = 0; scroll < maxScrolls; scroll++) {
      // Check auth wall or checkpoint before each action
      if (guardrails.detectCheckpointOrAuthWall(page.url()).detected) {
        return {
          success: false,
          alert: 'CAPTCHA_OR_LOGIN_WALL',
          message: 'Security challenge or login wall appeared during scrolling',
          posts: extractedPosts,
        }
      }

      // Extract newly visible posts
      const batch = await extractPostsFromPage(page, seenUrls)
      for (const p of batch) {
        extractedPosts.push(p)
        if (extractedPosts.length >= maxPosts) {
          break
        }
      }

      if (extractedPosts.length >= maxPosts) {
        break
      }

      if (extractedPosts.length === previousCount) {
        noNewContentRounds++
        if (noNewContentRounds >= 3) {
          // Reached bottom of search results or no more content
          break
        }
      } else {
        noNewContentRounds = 0
        previousCount = extractedPosts.length
      }

      // Human-like smooth scroll down
      const scrollDistance = Math.floor(Math.random() * 400) + 500
      await page.mouse.wheel(0, scrollDistance)

      // Randomized human delay: 1.2s - 2.8s
      await randomDelay(1200, 2800)
    }

    return {
      success: true,
      query,
      timeFilter,
      sortBy,
      totalExtracted: extractedPosts.length,
      posts: extractedPosts,
    }
  } finally {
    if (context) {
      await context.close().catch(() => {})
    }
  }
}
