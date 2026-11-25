// worker/src/scraper.js
import path from 'node:path'
import { chromium } from 'playwright'
import { buildLinkedInSearchUrl, extractPostsFromPage } from './parser.js'

function randomDelay(minMs, maxMs) {
  const delay = Math.floor(Math.random() * (maxMs - minMs + 1)) + minMs
  return new Promise((resolve) => setTimeout(resolve, delay))
}

let currentActiveContext = null

/**
 * Aborts and cancels any currently active scraping session immediately.
 */
export async function cancelCurrentScrape() {
  if (currentActiveContext) {
    try {
      await currentActiveContext.close()
    } catch {
      // Browser context already closing
    }
    currentActiveContext = null
    return true
  }
  return false
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
    currentActiveContext = context

    page = context.pages().length > 0 ? context.pages()[0] : await context.newPage()

    // Navigate to LinkedIn Content Search
    await page.goto(targetUrl, { waitUntil: 'domcontentloaded', timeout: 45000 })
    await randomDelay(2000, 3500)

    // Check for CAPTCHA / checkpoint / login wall
    const currentUrl = page.url()
    const pageContent = await page.content()
    const check = guardrails.detectCheckpointOrAuthWall(currentUrl, pageContent)
    if (check.detected) {
      console.log(`[Worker] Auth/Login challenge detected: ${currentUrl}`)
      console.log('[Worker] Keeping Chrome window open for up to 60 seconds. Please log in on the screen now...')

      // Wait up to 60 seconds for the user to complete login in the open Chrome window
      try {
        await page.waitForURL(
          (url) => !url.href.includes('/uas/login') && !url.href.includes('/checkpoint') && !url.href.includes('/login'),
          { timeout: 60000 }
        )
        console.log('[Worker] Successfully authenticated! Re-navigating to search query...')
        await page.goto(targetUrl, { waitUntil: 'domcontentloaded', timeout: 45000 })
        await randomDelay(2500, 4000)
      } catch {
        return {
          success: false,
          alert: 'CAPTCHA_OR_LOGIN_WALL',
          message: check.message,
          url: page.url(),
          posts: [],
        }
      }
    }

    // Scroll loop with randomized human-like delays
    let previousCount = 0
    let noNewContentRounds = 0
    const maxScrolls = 80

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

      // Check for and click LinkedIn "Show more results" / "See more results" button
      try {
        const showMoreBtn = page.locator(
          'button.scaffold-finite-scroll__load-button, button:has-text("Show more results"), button:has-text("See more results"), button:has-text("Load more")'
        ).first()
        if (await showMoreBtn.isVisible().catch(() => false)) {
          console.log('[Worker] Clicking "Show more results" button to continue scrolling feed...')
          await showMoreBtn.click({ timeout: 1500 }).catch(() => {})
          await randomDelay(1800, 3000)
        }
      } catch {
        // Non-fatal
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
        if (noNewContentRounds >= 5) {
          // Reached bottom of search results or no more content
          console.log(`[Worker] Reached bottom of search feed or end of results after ${scroll + 1} scrolls. Total extracted: ${extractedPosts.length}`)
          break
        }
      } else {
        noNewContentRounds = 0
        previousCount = extractedPosts.length
      }

      // Human-like smooth scroll down + DOM scroll
      await page.evaluate(() => {
        window.scrollBy({ top: 900, left: 0, behavior: 'smooth' })
      }).catch(() => {})
      const scrollDistance = Math.floor(Math.random() * 400) + 400
      await page.mouse.wheel(0, scrollDistance).catch(() => {})

      // Randomized human delay: 1.2s - 2.5s
      await randomDelay(1200, 2500)
    }

    return {
      success: true,
      query,
      timeFilter,
      sortBy,
      totalExtracted: extractedPosts.length,
      posts: extractedPosts,
    }
  } catch (err) {
    console.error('[Worker] Scrape execution error:', err.message)
    const isClosed = err.message.includes('closed') || err.message.includes('Target page')
    return {
      success: false,
      error: isClosed
        ? 'Chrome browser was closed during scraping. Please keep the Chrome window open until scraping completes.'
        : err.message,
      posts: extractedPosts,
    }
  } finally {
    currentActiveContext = null
    if (context) {
      await context.close().catch(() => {})
    }
  }
}
