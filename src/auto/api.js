// src/auto/api.js
// Client API layer communicating with Go server or falling back to rich mock data.

const API_BASE = import.meta.env.VITE_API_URL || 'http://localhost:8080'

// Default mock state for offline development & immediate demo
const MOCK_STORAGE_KEY = 'emailauto.mock_state.v1'

const initialMockState = {
  autoEnabled: false,
  status: 'IDLE', // IDLE | SEARCHING | FILTERING | GENERATING | SENDING | STOPPED
  nextRunIn: '14m 20s',
  dailySent: 12,
  dailyCap: 30,
  lastRunAt: new Date(Date.now() - 3600000).toISOString(),
  posts: [
    {
      id: 'p1',
      url: 'https://news.ycombinator.com/item?id=4001',
      author: 'alex_tech',
      company: 'SupraBase Labs',
      role: 'Full Stack Go / React Developer',
      work_mode: 'remote',
      paid_status: 'paid',
      location: 'Remote (Anywhere)',
      status: 'extracted',
      found_at: new Date(Date.now() - 1800000).toISOString(),
      extracted: {
        stipend: 45000,
        experience_years: 2.5,
        keywords: ['golang', 'react', 'postgres'],
      },
    },
    {
      id: 'p2',
      url: 'https://news.ycombinator.com/item?id=4002',
      author: 'dev_hire',
      company: 'FinCrypto Solutions',
      role: 'Blockchain Smart Contract Lead',
      work_mode: 'remote',
      paid_status: 'paid',
      location: 'London / Remote',
      status: 'skipped',
      skip_reason: 'contains excluded keyword: crypto',
      found_at: new Date(Date.now() - 3600000).toISOString(),
      extracted: {
        stipend: 60000,
        experience_years: 4.0,
        keywords: ['crypto', 'solidity'],
      },
    },
    {
      id: 'p3',
      url: 'https://news.ycombinator.com/item?id=4003',
      author: 'sarah_hr',
      company: 'Zenith Health AI',
      role: 'Junior Frontend Intern',
      work_mode: 'hybrid',
      paid_status: 'unpaid',
      location: 'Bangalore, India',
      status: 'skipped',
      skip_reason: 'post is unpaid',
      found_at: new Date(Date.now() - 7200000).toISOString(),
      extracted: {
        stipend: 0,
        experience_years: 0.5,
        keywords: ['react', 'tailwind'],
      },
    },
    {
      id: 'p4',
      url: 'https://news.ycombinator.com/item?id=4004',
      author: 'fast_hire_crypto',
      company: 'Global Task Wealth',
      role: 'Remote Data Reviewer (Earn $400/day)',
      work_mode: 'remote',
      paid_status: 'paid',
      location: 'Remote',
      status: 'skipped',
      skip_reason: 'SPAM / FAKE POST FLAGGED: contains fraudulent scam indicator: pay application fee & unverified telegram recruiter',
      found_at: new Date(Date.now() - 10800000).toISOString(),
      extracted: {
        stipend: 150000,
        is_suspicious: true,
        suspicion_reason: 'Detected pay-to-apply fee requirement and unverified free email domain.',
        red_flags: ['pay-to-apply', 'vague business model'],
      },
    },
  ],
  outreach: [
    {
      id: 'o1',
      contact_id: 'c1',
      contact_email: 'alex@suprabase.io',
      company: 'SupraBase Labs',
      role: 'Full Stack Go / React Developer',
      kind: 'initial',
      subject: 'Exploring Full Stack Go / React role at SupraBase Labs',
      body: 'Hi Alex,\n\nI noticed your opening for a Full Stack Go / React Developer. With production experience building high-throughput APIs in Go and reactive web UIs, I believe my background aligns directly with your stack.\n\nAttached is my resume for your review. Would you be open to a brief 10-minute chat this week?\n\nBest regards,\nPratyush',
      status: 'draft',
      scheduled_at: null,
      sent_at: null,
    },
    {
      id: 'o2',
      contact_id: 'c2',
      contact_email: 'marcus@cloudscale.net',
      company: 'CloudScale Systems',
      role: 'Backend Systems Engineer',
      kind: 'initial',
      subject: 'Backend Systems Engineer opening - Cold outreach',
      body: 'Hi Marcus,\n\nLoved your recent post regarding microservices scaling. I specialize in concurrent Go systems and would love to contribute to CloudScale.',
      status: 'queued',
      scheduled_at: new Date(Date.now() + 180000).toISOString(),
      sent_at: null,
    },
    {
      id: 'o3',
      contact_id: 'c3',
      contact_email: 'priya@datastride.com',
      company: 'DataStride Analytics',
      role: 'Go Platform Engineer',
      kind: 'initial',
      subject: 'Platform Engineering Application - Pratyush',
      body: 'Hi Priya,\n\nReaching out regarding your DataStride posting. Here is my profile and experience.',
      status: 'sent',
      gmail_message_id: 'msg_18f88a91b2c',
      scheduled_at: null,
      sent_at: new Date(Date.now() - 14400000).toISOString(),
    },
  ],
  replies: [
    {
      id: 'r1',
      outreach_id: 'o3',
      from: 'Priya Sharma <priya@datastride.com>',
      subject: 'Re: Platform Engineering Application - Pratyush',
      snippet: 'Thanks for reaching out Pratyush! Your background looks very relevant. Can you do a call tomorrow at 4 PM IST?',
      received_at: new Date(Date.now() - 7200000).toISOString(),
      is_bounce: false,
    },
    {
      id: 'r2',
      outreach_id: 'o4',
      from: 'Mail Delivery Subsystem <mailer-daemon@googlemail.com>',
      subject: 'Delivery Status Notification (Failure)',
      snippet: '550 5.1.1 The email account that you tried to reach does not exist.',
      received_at: new Date(Date.now() - 28800000).toISOString(),
      is_bounce: true,
      bounce_reason: '550 5.1.1 Mailbox does not exist',
    },
  ],
  profile: {
    summary: 'Full Stack & Backend Developer experienced in Go, React, PostgreSQL, and distributed automation pipelines.',
    skills: 'Go, React 19, JavaScript, Python, PostgreSQL, Docker, Playwright, REST APIs',
    example_text: 'Hi {name},\n\nI came across your post about {role} at {company} and would love to discuss how my skill set can support your roadmap.',
    resume_path: 'uploads/resume_sample.pdf',
    resume_text: 'Senior Software Developer with 3+ years experience building cloud applications in Go and React...',
  },
  settings: {
    min_stipend: 15000,
    allowed_locations: ['Bangalore', 'Remote', 'Delhi'],
    work_modes: ['remote', 'hybrid'],
    min_experience_years: 0.5,
    max_experience_years: 5.0,
    include_keywords: ['golang', 'go', 'react', 'full stack', 'backend'],
    exclude_keywords: ['crypto', 'web3', 'blockchain', 'solidity'],
    skip_unpaid: true,
    skip_unknown_paid: false,
    trust_threshold: 40,
    daily_cap: 30,
    send_window: '09:00-19:00 IST',
    gap_min_minutes: 2,
    gap_max_minutes: 6,
    approval_mode: true,
    followup_after_hours: 24,
    max_followups: 2,
  },
}

function getStoredMockState() {
  try {
    const raw = localStorage.getItem(MOCK_STORAGE_KEY)
    if (!raw) return initialMockState
    return JSON.parse(raw)
  } catch {
    return initialMockState
  }
}

function saveStoredMockState(state) {
  try {
    localStorage.setItem(MOCK_STORAGE_KEY, JSON.stringify(state))
  } catch (err) {
    console.warn('Could not persist mock state', err)
  }
}

// Request helper with graceful mock fallback
async function safeFetch(endpoint, options = {}, mockFallbackFn) {
  try {
    const url = `${API_BASE}${endpoint}`
    const res = await fetch(url, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        ...(options.headers || {}),
      },
    })
    if (!res.ok) {
      throw new Error(`Server returned ${res.status}: ${res.statusText}`)
    }
    return await res.json()
  } catch (err) {
    // Fall back to local mock data
    if (mockFallbackFn) {
      return mockFallbackFn()
    }
    throw err
  }
}

export const api = {
  async getStatus() {
    return safeFetch('/health', {}, () => {
      const state = getStoredMockState()
      return {
        status: state.status,
        database: 'mock_active',
        uptime: '2h 15m',
      }
    })
  },

  async getDashboardSummary() {
    return safeFetch('/api/orchestrator/status', {}, () => {
      const state = getStoredMockState()
      return {
        auto_enabled: state.autoEnabled,
        state: state.status,
        armed: false,
        next_run_in_seconds: 3600,
        daily_sent: state.dailySent,
        daily_cap: state.dailyCap,
        last_run_at: state.lastRunAt,
      }
    })
  },

  async toggleAuto(enabled) {
    return safeFetch(
      '/api/orchestrator/toggle-auto',
      {
        method: 'POST',
        body: JSON.stringify({ enabled }),
      },
      () => {
        const state = getStoredMockState()
        state.autoEnabled = enabled
        state.status = enabled ? 'IDLE' : 'PAUSED'
        saveStoredMockState(state)
        return { auto_enabled: state.autoEnabled, status: { state: state.status } }
      },
    )
  },

  async killSwitch() {
    return safeFetch(
      '/api/orchestrator/kill-switch',
      { method: 'POST' },
      () => {
        const state = getStoredMockState()
        state.autoEnabled = false
        state.status = 'PAUSED'
        saveStoredMockState(state)
        return { message: 'KILL SWITCH ACTIVATED', status: { state: 'PAUSED', armed: false, auto_enabled: false } }
      },
    )
  },

  async triggerRun({ query, timeFilter, sortBy }) {
    return safeFetch(
      '/api/orchestrator/run-now',
      {
        method: 'POST',
        body: JSON.stringify({
          query,
          time_filter: timeFilter,
          sort_by: sortBy,
        }),
      },
      () => {
        const state = getStoredMockState()
        state.status = 'IDLE'
        const newRun = {
          id: 'run_' + Date.now(),
          query: query || 'golang remote developer',
          time_filter: timeFilter || '24h',
          sort_by: sortBy || 'latest',
          trigger: 'manual',
          status: 'completed',
          stats: { found: 3, filtered: 3, verified: 1, skipped: 2, drafted: 1 },
          created_at: new Date().toISOString(),
        }
        if (!state.runs) state.runs = []
        state.runs.unshift(newRun)
        saveStoredMockState(state)
        return {
          message: 'pipeline run initiated and engine armed for today',
          run: newRun,
          status: { state: 'IDLE', armed: true, auto_enabled: state.autoEnabled },
        }
      },
    )
  },

  async getRuns() {
    return safeFetch('/api/runs', {}, () => {
      const state = getStoredMockState()
      if (!state.runs) {
        state.runs = [
          {
            id: 'r_mock_1',
            query: 'golang react remote',
            time_filter: '24h',
            sort_by: 'latest',
            trigger: 'manual',
            status: 'completed',
            stats: { found: 15, filtered: 15, verified: 4, skipped: 11, drafted: 4 },
            created_at: new Date(Date.now() - 3600000).toISOString(),
          },
        ]
        saveStoredMockState(state)
      }
      return state.runs
    })
  },

  async getPosts() {
    return safeFetch('/api/posts', {}, () => {
      const state = getStoredMockState()
      return state.posts
    })
  },

  async getOutreach(status) {
    const query = status ? `?status=${status}` : ''
    return safeFetch(`/api/outreach${query}`, {}, () => {
      const state = getStoredMockState()
      if (status) return state.outreach.filter((o) => o.status === status)
      return state.outreach
    })
  },

  async approveOutreach(id) {
    return safeFetch(
      `/api/outreach/${id}/approve`,
      { method: 'POST' },
      () => {
        const state = getStoredMockState()
        state.outreach = state.outreach.map((item) => {
          if (item.id === id) {
            return {
              ...item,
              status: 'queued',
              scheduled_at: new Date(Date.now() + 60000).toISOString(),
            }
          }
          return item
        })
        saveStoredMockState(state)
        return { success: true }
      },
    )
  },

  async skipOutreach(id) {
    return safeFetch(
      `/api/outreach/${id}/skip`,
      { method: 'POST' },
      () => {
        const state = getStoredMockState()
        state.outreach = state.outreach.map((item) => {
          if (item.id === id) {
            return { ...item, status: 'skipped' }
          }
          return item
        })
        saveStoredMockState(state)
        return { success: true }
      },
    )
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
    for (const id of ids) {
      await this.approveOutreach(id)
      await this.sendNext()
    }
    return { success: true, count: ids.length }
  },

  async sendOutreachNow(id) {
    await this.approveOutreach(id)
    return await this.sendNext()
  },

  async updateOutreach(id, { subject, body, status }) {
    return safeFetch(
      `/api/outreach/${id}`,
      {
        method: 'PUT',
        body: JSON.stringify({ subject, body, status }),
      },
      () => {
        const state = getStoredMockState()
        state.outreach = state.outreach.map((item) => {
          if (item.id === id) {
            return {
              ...item,
              subject: subject ?? item.subject,
              body: body ?? item.body,
              status: status ?? item.status,
            }
          }
          return item
        })
        saveStoredMockState(state)
        return { success: true }
      },
    )
  },

  async getSenderStatus() {
    return safeFetch('/api/sender/status', {}, () => {
      const state = getStoredMockState()
      return {
        state: state.status.toLowerCase(),
        daily_sent: state.dailySent,
        daily_cap: state.dailyCap,
      }
    })
  },

  async pauseSender() {
    return safeFetch('/api/sender/pause', { method: 'POST' }, () => {
      const state = getStoredMockState()
      state.status = 'PAUSED'
      saveStoredMockState(state)
      return { message: 'sender paused' }
    })
  },

  async resumeSender() {
    return safeFetch('/api/sender/resume', { method: 'POST' }, () => {
      const state = getStoredMockState()
      state.status = 'IDLE'
      saveStoredMockState(state)
      return { message: 'sender resumed' }
    })
  },

  async stopAllSender() {
    return safeFetch('/api/sender/stop-all', { method: 'POST' }, () => {
      const state = getStoredMockState()
      state.status = 'STOPPED'
      saveStoredMockState(state)
      return { message: 'sender stopped' }
    })
  },

  async getReplies() {
    return safeFetch('/api/replies', {}, () => {
      const state = getStoredMockState()
      return state.replies
    })
  },

  async pollNow() {
    return safeFetch('/api/poller/poll', { method: 'POST' }, () => {
      return {
        threads_checked: 2,
        replies_recorded: 1,
        bounces_detected: 0,
        followup1_drafted: 1,
        followup2_drafted: 0,
      }
    })
  },

  async sendNext() {
    return safeFetch('/api/sender/send-next', { method: 'POST' }, () => {
      return { sent: true, reason: 'dispatched via mock mode' }
    })
  },

  async getSettings() {
    return safeFetch('/api/settings', {}, () => {
      const state = getStoredMockState()
      return state.settings
    })
  },

  async saveSettings(settings) {
    return safeFetch(
      '/api/settings',
      {
        method: 'PUT',
        body: JSON.stringify(settings),
      },
      () => {
        const state = getStoredMockState()
        state.settings = { ...state.settings, ...settings }
        saveStoredMockState(state)
        return state.settings
      },
    )
  },

  async getProfile() {
    return safeFetch('/api/profile', {}, () => {
      const state = getStoredMockState()
      return state.profile
    })
  },

  async saveProfile(profile) {
    return safeFetch(
      '/api/profile',
      {
        method: 'PUT',
        body: JSON.stringify(profile),
      },
      () => {
        const state = getStoredMockState()
        state.profile = { ...state.profile, ...profile }
        saveStoredMockState(state)
        return state.profile
      },
    )
  },

  async uploadResume(file) {
    try {
      const formData = new FormData()
      formData.append('resume', file)
      const res = await fetch(`${API_BASE}/api/profile/resume`, {
        method: 'POST',
        body: formData,
      })
      if (!res.ok) throw new Error(`Upload returned ${res.status}`)
      return await res.json()
    } catch {
      // Mock fallback
      const state = getStoredMockState()
      state.profile.resume_path = `uploads/${file.name}`
      state.profile.resume_text = `Extracted text from ${file.name}: Experienced software engineer proficient in modern web applications and automated systems.`
      saveStoredMockState(state)
      return {
        message: 'Resume uploaded and processed (mock mode)',
        resume_path: state.profile.resume_path,
        extracted_chars: state.profile.resume_text.length,
        profile: state.profile,
      }
    }
  },

  async getAuthStatus() {
    return safeFetch('/auth/status', {}, () => {
      return {
        connected: true,
        email: 'pratyush@example.com',
        scopes: ['gmail.send', 'gmail.readonly'],
      }
    })
  },
}
