// worker/src/manual.js

/**
 * Manual post import handler as a reliable no-scrape fallback.
 * Allows users to paste post text, URL, and optional company/role details.
 */
export function processManualImport(payload = {}) {
  const { url, text, author, company, role, work_mode, paid_status, location, stipend, experience_years } = payload

  if (!url || !url.trim()) {
    throw new Error("Missing required field: 'url'")
  }

  if (!text || !text.trim()) {
    throw new Error("Missing required field: 'text'")
  }

  const cleanUrl = url.trim()
  const cleanText = text.trim()

  return {
    id: `manual_${Date.now()}`,
    url: cleanUrl,
    author: (author || '').trim(),
    text: cleanText,
    company: (company || '').trim(),
    role: (role || '').trim(),
    work_mode: (work_mode || 'unknown').trim().toLowerCase(),
    paid_status: (paid_status || 'unknown').trim().toLowerCase(),
    location: (location || '').trim(),
    extracted: {
      stipend: stipend ? Number(stipend) : null,
      experience_years: experience_years ? Number(experience_years) : null,
      source: 'manual_import',
    },
    status: 'new',
    found_at: new Date().toISOString(),
    created_at: new Date().toISOString(),
  }
}
