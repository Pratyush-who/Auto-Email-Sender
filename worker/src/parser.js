// worker/src/parser.js

/**
 * Builds the canonical LinkedIn Content Search URL with query and filter parameters.
 */
export function buildLinkedInSearchUrl({ query, timeFilter = '24h', sortBy = 'latest' }) {
  const base = 'https://www.linkedin.com/search/results/content/'
  const params = new URLSearchParams()

  params.set('keywords', query)
  params.set('origin', 'FACETED_SEARCH')

  // Map datePosted parameter to LinkedIn's faceted JSON array format
  let dateParam = 'past-24h'
  if (timeFilter === 'week' || timeFilter === '1w') {
    dateParam = 'past-week'
  } else if (timeFilter === 'month' || timeFilter === '1m') {
    dateParam = 'past-month'
  }
  params.set('datePosted', JSON.stringify([dateParam]))

  // Map sortBy parameter to LinkedIn's faceted JSON array format
  if (sortBy === 'latest' || sortBy === 'date_posted') {
    params.set('sortBy', JSON.stringify(['date_posted']))
  }

  return `${base}?${params.toString()}`
}

/**
 * In-page extractor executed inside the Playwright browser page context.
 * Expands "see more" buttons and extracts post elements into clean JSON objects.
 * Supports both modern LinkedIn SDUI (LazyColumn) and legacy feed containers.
 */
export async function extractPostsFromPage(page, existingUrls = new Set()) {
  // 1. Expand all visible "see more" / show more buttons instantly in the DOM
  try {
    await page.evaluate(() => {
      const candidates = Array.from(document.querySelectorAll(
        'button.feed-shared-inline-show-more-text__see-more-less-toggle, button.feed-shared-see-more, [data-testid="inline-show-more-text"] button, button'
      ))
      for (const btn of candidates) {
        const txt = (btn.innerText || '').toLowerCase().trim()
        if (
          txt.includes('see more') ||
          txt.includes('…see more') ||
          btn.classList.contains('feed-shared-inline-show-more-text__see-more-less-toggle')
        ) {
          try {
            btn.click()
          } catch {}
        }
      }
    })
    await new Promise((r) => setTimeout(r, 300))
  } catch {
    // Non-fatal
  }

  // 2. Extract DOM content of feed cards
  const rawPosts = await page.evaluate(() => {
    // Collect potential card nodes from modern SDUI, classic containers, and search containers
    let cards = Array.from(document.querySelectorAll(
      'li.reusable-search__result-container, div.feed-shared-update-v2, div[data-view-name="feed-full-update"], [data-testid="lazy-column"] > div, div[data-urn*="activity"]'
    ))

    if (cards.length === 0) {
      cards = Array.from(document.querySelectorAll('.artdeco-card, div[data-display-contents="true"]'))
    }

    const results = []

    for (let index = 0; index < cards.length; index++) {
      const card = cards[index]
      const rawText = (card.innerText || '').trim()

      // Skip non-card spacers, tiny badges, feedback widgets, or noise
      if (rawText.length < 50) continue
      if (
        rawText.includes('Are these results helpful?') ||
        rawText.includes('Your feedback helps us improve') ||
        rawText.includes('Try searching for') ||
        rawText.startsWith('People also search for')
      ) continue

      // Filter out sponsored / promoted ads
      const isPromoted = Boolean(
        card.querySelector('.feed-shared-actor__sub-description--promoted, [data-testid="promoted"]') ||
        rawText.startsWith('Promoted') ||
        rawText.includes('\nPromoted\n') ||
        rawText.includes('\nSponsored\n')
      )
      if (isPromoted) continue

      const allAnchors = Array.from(card.querySelectorAll('a'))

      // Also extract mailto links from anchors if present
      let mailtoEmail = ''
      for (const a of allAnchors) {
        const h = a.href || ''
        if (h.toLowerCase().startsWith('mailto:')) {
          mailtoEmail = h.replace(/^mailto:/i, '').split('?')[0].trim()
          break
        }
      }

      // Find post URN or permalink
      let postUrl = ''
      const urn = card.getAttribute('data-urn') || card.getAttribute('data-id') || card.getAttribute('data-chameleon-result-urn') || ''
      if (urn.includes('urn:li:activity:')) {
        const activityId = urn.split('urn:li:activity:')[1].split(':')[0]
        postUrl = `https://www.linkedin.com/feed/update/urn:li:activity:${activityId}/`
      } else if (urn.includes('urn:li:share:')) {
        const shareId = urn.split('urn:li:share:')[1].split(':')[0]
        postUrl = `https://www.linkedin.com/feed/update/urn:li:share:${shareId}/`
      }

      if (!postUrl) {
        for (const a of allAnchors) {
          const h = a.href || ''
          if (h.includes('/feed/update/urn:li:activity:') || h.includes('/feed/update/urn:li:share:')) {
            postUrl = h.split('?')[0]
            break
          }
          if (h.includes('/posts/') && !h.includes('/company/') && !h.includes('/in/')) {
            postUrl = h.split('?')[0]
            break
          }
        }
      }

      // Check timestamp link
      if (!postUrl) {
        const timeEl = card.querySelector('time')
        if (timeEl) {
          const timeAnchor = timeEl.closest('a')
          if (timeAnchor && timeAnchor.href && !timeAnchor.href.includes('/in/') && !timeAnchor.href.includes('/company/')) {
            postUrl = timeAnchor.href.split('?')[0]
          }
        }
      }

      // Look for author profile or company anchor
      let authorName = ''
      let authorUrl = ''
      const authorAnchor = allAnchors.find(a =>
        a.href && (a.href.includes('/in/') || a.href.includes('/company/'))
      )
      if (authorAnchor) {
        authorUrl = authorAnchor.href.split('?')[0]
        const textParts = (authorAnchor.innerText || '').split('\n').map(s => s.trim()).filter(Boolean)
        if (textParts.length > 0 && textParts[0].length > 1) {
          authorName = textParts[0].replace(/•.*/, '').trim()
        }
      }

      // Fallback author name from classes or text
      if (!authorName) {
        const authorEl = card.querySelector(
          '.update-components-actor__name, .feed-shared-actor__name, span[dir="ltr"]'
        )
        if (authorEl) {
          authorName = authorEl.textContent.trim().split('\n')[0].replace(/•.*/, '').trim()
        }
      }

      if (!authorName) {
        const lines = rawText.split('\n').map(l => l.trim()).filter(Boolean)
        const feedPostIdx = lines.findIndex(l => l.toLowerCase() === 'feed post')
        if (feedPostIdx !== -1 && lines[feedPostIdx + 1]) {
          authorName = lines[feedPostIdx + 1].replace(/•.*/, '').trim()
        } else {
          authorName = 'Hiring Manager'
        }
      }

      // Clean post text
      let postText = rawText
      if (postText.startsWith('Feed post')) {
        postText = postText.replace(/^Feed post\s*/i, '')
      }
      if (mailtoEmail && !postText.includes(mailtoEmail)) {
        postText += `\nEmail: ${mailtoEmail}`
      }

      // Generate clean unique URL if no direct permalink found
      if (!postUrl) {
        let hash = 0
        const str = `${authorName}-${postText.slice(0, 140)}`
        for (let i = 0; i < str.length; i++) {
          hash = ((hash << 5) - hash) + str.charCodeAt(i)
          hash |= 0
        }
        postUrl = `https://www.linkedin.com/feed/post/${Math.abs(hash).toString(36)}-${index}/`
      }

      results.push({
        url: postUrl,
        author: authorName,
        text: postText.trim(),
      })
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
