// worker/src/parser.js

/**
 * Builds the canonical LinkedIn Content Search URL with query and filter parameters.
 */
export function buildLinkedInSearchUrl({ query, timeFilter = '24h', sortBy = 'relevance' }) {
  const base = 'https://www.linkedin.com/search/results/content/'
  const params = new URLSearchParams()

  params.set('keywords', query)

  // Map datePosted parameter
  // LinkedIn uses: datePosted="past-24h", "past-week", "past-month"
  let dateParam = 'past-24h'
  if (timeFilter === 'week' || timeFilter === '1w') {
    dateParam = 'past-week'
  } else if (timeFilter === 'month' || timeFilter === '1m') {
    dateParam = 'past-month'
  }
  params.set('datePosted', `"${dateParam}"`)

  // Map sortBy parameter
  // LinkedIn uses: sortBy="date_posted" for Latest, "relevance" for Top
  if (sortBy === 'latest' || sortBy === 'date_posted') {
    params.set('sortBy', '"date_posted"')
  } else {
    params.set('sortBy', '"relevance"')
  }

  return `${base}?${params.toString()}`
}

/**
 * In-page extractor executed inside the Playwright browser page context.
 * Expands "see more" buttons and extracts post elements into clean JSON objects.
 */
export async function extractPostsFromPage(page, existingUrls = new Set()) {
  // 1. Expand all visible "see more" / show more buttons
  try {
    const seeMoreButtons = page.locator(
      'button.feed-shared-inline-show-more-text__see-more-less-toggle, button:has-text("see more"), button:has-text("…see more")'
    )
    const count = await seeMoreButtons.count()
    for (let i = 0; i < Math.min(count, 20); i++) {
      try {
        const btn = seeMoreButtons.nth(i)
        if (await btn.isVisible()) {
          await btn.click({ timeout: 500 }).catch(() => {})
        }
      } catch {
        // Continue if single button click fails
      }
    }
  } catch {
    // Non-fatal
  }

  // 2. Extract DOM content of feed cards
  const rawPosts = await page.evaluate(() => {
    const cards = document.querySelectorAll(
      'div.feed-shared-update-v2, div[data-urn*="urn:li:activity"], li.reusable-search__result-container, .artdeco-card'
    )
    const results = []

    for (const card of cards) {
      // Find post URN or permalink
      let postUrl = ''
      const urn = card.getAttribute('data-urn') || card.getAttribute('data-id')
      if (urn && urn.includes('urn:li:activity:')) {
        const activityId = urn.split('urn:li:activity:')[1]
        postUrl = `https://www.linkedin.com/feed/update/urn:li:activity:${activityId}/`
      }

      if (!postUrl) {
        const permalinkAnchor = card.querySelector(
          'a[href*="/feed/update/"], a[href*="/posts/"], a.app-aware-link[href*="activity"]'
        )
        if (permalinkAnchor) {
          postUrl = permalinkAnchor.href.split('?')[0]
        }
      }

      // Author name
      let authorName = ''
      const authorEl = card.querySelector(
        '.update-components-actor__name, .feed-shared-actor__name, .app-aware-link span[dir="ltr"]'
      )
      if (authorEl) {
        authorName = authorEl.textContent.trim()
      }

      // Author headline / role
      let authorTitle = ''
      const titleEl = card.querySelector(
        '.update-components-actor__description, .feed-shared-actor__description'
      )
      if (titleEl) {
        authorTitle = titleEl.textContent.trim()
      }

      // Body text
      let postText = ''
      const textEl = card.querySelector(
        '.feed-shared-update-v2__description, .feed-shared-inline-show-more-text, .update-components-text'
      )
      if (textEl) {
        postText = textEl.textContent.trim()
      }

      if (postUrl && postText) {
        results.push({
          url: postUrl,
          author: authorTitle ? `${authorName} (${authorTitle})` : authorName,
          text: postText,
        })
      }
    }

    return results
  })

  // 3. Deduplicate by URL
  const uniquePosts = []
  for (const post of rawPosts) {
    if (!existingUrls.has(post.url)) {
      existingUrls.add(post.url)
      uniquePosts.push(post)
    }
  }

  return uniquePosts
}
