// src/auto/api.js
// Direct API layer communicating with the Go backend on :8080 and Playwright worker on :3001.

const API_BASE = import.meta.env.VITE_API_URL || 'http://localhost:8080'

// Generic HTTP request helper. Returns JSON or throws error.
async function request(endpoint, options = {}) {
  const url = `${API_BASE}${endpoint}`
  const res = await fetch(url, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(options.headers || {}),
    },
  })
  if (!res.ok) {
    const errorBody = await res.text().catch(() => '')
    throw new Error(`HTTP ${res.status}: ${errorBody || res.statusText}`)
  }
  return await res.json()
}

export const api = {
  async getStatus() {
    try {
      return await request('/health')
    } catch {
      return { status: 'OFFLINE', database: 'disconnected', uptime: '0m' }
    }
  },

  async getDashboardSummary() {
    try {
      return await request('/api/orchestrator/status')
    } catch (err) {
      console.warn('Could not fetch dashboard summary:', err.message)
      return {
        auto_enabled: false,
        state: 'IDLE',
        armed: false,
        next_run_in_seconds: 0,
        daily_sent: 0,
        daily_cap: 30,
        last_run_at: null,
      }
    }
  },

  async toggleAuto(enabled) {
    return request('/api/orchestrator/toggle-auto', {
      method: 'POST',
      body: JSON.stringify({ enabled }),
    })
  },

  async cancelRun() {
    try {
      await fetch('http://localhost:3001/cancel', { method: 'POST' })
    } catch {
      // Worker offline or unreachable
    }
    return request('/api/orchestrator/kill-switch', { method: 'POST' })
  },

  async killSwitch() {
    return request('/api/orchestrator/kill-switch', { method: 'POST' })
  },

  async triggerRun({ query, timeFilter, sortBy, maxPosts = 150 }) {
    return request('/api/orchestrator/run-now', {
      method: 'POST',
      body: JSON.stringify({
        query,
        time_filter: timeFilter,
        sort_by: sortBy,
        max_posts: maxPosts,
      }),
    })
  },

  async getRuns() {
    try {
      const data = await request('/api/runs')
      return Array.isArray(data) ? data : []
    } catch {
      return []
    }
  },

  async getPosts(limit = 500) {
    try {
      const data = await request(`/api/posts?limit=${limit}`)
      return Array.isArray(data) ? data : []
    } catch {
      return []
    }
  },

  async clearAllPosts() {
    return request('/api/posts', { method: 'DELETE' })
  },

  async getContacts() {
    try {
      const data = await request('/api/contacts')
      return Array.isArray(data) ? data : []
    } catch {
      return []
    }
  },

  async getOutreach(status) {
    try {
      const query = status ? `?status=${status}` : ''
      const data = await request(`/api/outreach${query}`)
      return Array.isArray(data) ? data : []
    } catch {
      return []
    }
  },

  async approveOutreach(id) {
    return request(`/api/outreach/${id}/approve`, { method: 'POST' })
  },

  async skipOutreach(id) {
    return request(`/api/outreach/${id}/skip`, { method: 'POST' })
  },

  async generateDraft({ postId, contactId }) {
    return request('/api/composer/generate', {
      method: 'POST',
      body: JSON.stringify({ post_id: postId, contact_id: contactId }),
    })
  },

  async bulkApproveOutreach(ids) {
    for (const id of ids) {
      await this.approveOutreach(id)
    }
    return { success: true, count: ids.length }
  },

  async bulkSkipOutreach(ids) {
    for (const id of ids) {
      await this.skipOutreach(id)
    }
    return { success: true, count: ids.length }
  },

  async bulkSendOutreach(ids) {
    let lastResult = null
    for (const id of ids) {
      await this.approveOutreach(id)
      lastResult = await this.sendNext(true)
    }
    return lastResult || { success: true, count: ids.length }
  },

  async sendOutreachNow(id) {
    await this.approveOutreach(id)
    return await this.sendNext(true)
  },

  async updateOutreach(id, { subject, body, status }) {
    return request(`/api/outreach/${id}`, {
      method: 'PUT',
      body: JSON.stringify({ subject, body, status }),
    })
  },

  async getSenderStatus() {
    try {
      return await request('/api/sender/status')
    } catch {
      return { state: 'idle', daily_sent: 0, daily_cap: 30 }
    }
  },

  async pauseSender() {
    return request('/api/sender/pause', { method: 'POST' })
  },

  async resumeSender() {
    return request('/api/sender/resume', { method: 'POST' })
  },

  async stopAllSender() {
    return request('/api/sender/stop-all', { method: 'POST' })
  },

  async getReplies() {
    try {
      const data = await request('/api/replies')
      return Array.isArray(data) ? data : []
    } catch {
      return []
    }
  },

  async pollNow() {
    return request('/api/poller/poll', { method: 'POST' })
  },

  async sendNext(force = false) {
    const url = '/api/sender/send-next' + (force ? '?force=true' : '')
    return request(url, { method: 'POST' })
  },

  async getSettings() {
    try {
      return await request('/api/settings')
    } catch {
      return null
    }
  },

  async saveSettings(settings) {
    return request('/api/settings', {
      method: 'PUT',
      body: JSON.stringify(settings),
    })
  },

  async getProfile() {
    try {
      return await request('/api/profile')
    } catch {
      return null
    }
  },

  async saveProfile(profile) {
    return request('/api/profile', {
      method: 'PUT',
      body: JSON.stringify(profile),
    })
  },

  async uploadResume(file) {
    const formData = new FormData()
    formData.append('resume', file)
    const res = await fetch(`${API_BASE}/api/profile/resume`, {
      method: 'POST',
      body: formData,
    })
    if (!res.ok) {
      const errorText = await res.text().catch(() => '')
      throw new Error(`Upload returned ${res.status}: ${errorText}`)
    }
    return await res.json()
  },

  async getAuthStatus() {
    try {
      return await request('/auth/status')
    } catch {
      return { connected: false, email: '' }
    }
  },

  async disconnectGoogle() {
    return request('/auth/disconnect', { method: 'POST' })
  },

  async openLinkedInLogin() {
    try {
      const res = await fetch('http://localhost:3001/open-login', { method: 'POST' })
      if (!res.ok) throw new Error(`Worker returned status ${res.status}`)
      return await res.json()
    } catch {
      return { message: 'Could not connect to worker on port 3001. Please run "npm start" in the worker directory.' }
    }
  },

  async resetWorkerQuota() {
    try {
      const res = await fetch('http://localhost:3001/reset-quota', { method: 'POST' })
      return await res.json()
    } catch {
      return { success: true }
    }
  },
}
