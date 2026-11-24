import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import './AutoPage.css'
import { api } from './api'

const ASCII_LOGO = `
 ███████╗███╗   ███╗ █████╗ ██╗██╗      █████╗ ██╗   ██╗████████╗ ██████╗ 
 ██╔════╝████╗ ████║██╔══██╗██║██║     ██╔══██╗██║   ██║╚══██╔══╝██╔═══██╗
 █████╗  ██╔████╔██║███████║██║██║     ███████║██║   ██║   ██║   ██║   ██║
 ██╔══╝  ██║╚██╔╝██║██╔══██║██║██║     ██╔══██║██║   ██║   ██║   ██║   ██║
 ███████╗██║ ╚═╝ ██║██║  ██║██║███████╗██║  ██║╚██████╔╝   ██║   ╚██████╔╝
 ╚══════╝╚═╝     ╚═╝╚═╝  ╚═╝╚═╝╚══════╝╚═╝  ╚═╝ ╚═════╝    ╚═╝    ╚═════╝ 
                      [ AUTONOMOUS JOB OUTREACH ENGINE ]
`

function formatCountdown(totalSec) {
  if (totalSec <= 0) return 'Due now'
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  const s = totalSec % 60
  return `${h.toString().padStart(2, '0')}h ${m.toString().padStart(2, '0')}m ${s.toString().padStart(2, '0')}s`
}

export default function AutoPage() {
  const [activeTab, setActiveTab] = useState('posts') // posts | drafts | queue | sent | replies | history | settings

  // Orchestrator Telemetry State
  const [summary, setSummary] = useState({
    auto_enabled: false,
    state: 'IDLE',
    armed: false,
    next_run_in_seconds: 7200,
    daily_sent: 0,
    daily_cap: 30,
    last_run_at: null,
  })
  const [countdown, setCountdown] = useState(0)

  // Run panel state
  const [query, setQuery] = useState('golang remote developer')
  const [timeFilter, setTimeFilter] = useState('24h')
  const [sortBy, setSortBy] = useState('latest')
  const [isRunning, setIsRunning] = useState(false)

  // Data states
  const [posts, setPosts] = useState([])
  const [outreachList, setOutreachList] = useState([])
  const [replies, setReplies] = useState([])
  const [runs, setRuns] = useState([])
  const [settings, setSettings] = useState(null)
  const [profile, setProfile] = useState(null)
  const [toast, setToast] = useState('')

  // Multiselect state for Drafts
  const [selectedDraftIds, setSelectedDraftIds] = useState([])
  const [editingDraftId, setEditingDraftId] = useState(null)
  const [draftEditValues, setDraftEditValues] = useState({ subject: '', body: '' })
  const [renderTimestamp, setRenderTimestamp] = useState(() => Date.now())

  const showToast = (msg) => {
    setToast(msg)
    setTimeout(() => setToast(''), 4500)
  }

  const loadAllData = useCallback(async () => {
    try {
      const [sumData, postData, outData, repData, runData, setts, prof] = await Promise.all([
        api.getDashboardSummary(),
        api.getPosts(),
        api.getOutreach(),
        api.getReplies(),
        api.getRuns(),
        api.getSettings(),
        api.getProfile(),
      ])

      if (sumData) {
        setSummary({
          auto_enabled: sumData.auto_enabled ?? sumData.autoEnabled ?? false,
          state: sumData.state || sumData.status || 'IDLE',
          armed: Boolean(sumData.armed),
          next_run_in_seconds: sumData.next_run_in_seconds ?? 0,
          daily_sent: sumData.daily_sent ?? sumData.dailySent ?? 0,
          daily_cap: sumData.daily_cap ?? sumData.dailyCap ?? 30,
          last_run_at: sumData.last_run_at || sumData.lastRunAt || null,
        })
        setCountdown(sumData.next_run_in_seconds ?? 0)
      }

      setPosts(postData || [])
      setOutreachList(outData || [])
      setReplies(repData || [])
      setRuns(runData || [])
      setSettings(setts)
      setProfile(prof)
    } catch (err) {
      console.warn('Error loading dashboard data:', err)
    }
  }, [])

  // Initial load + periodic poller for orchestrator status (every 10s)
  useEffect(() => {
    let ignore = false

    async function fetchInitialData() {
      try {
        const [sumData, postData, outData, repData, runData, setts, prof] = await Promise.all([
          api.getDashboardSummary(),
          api.getPosts(),
          api.getOutreach(),
          api.getReplies(),
          api.getRuns(),
          api.getSettings(),
          api.getProfile(),
        ])

        if (!ignore) {
          if (sumData) {
            setSummary({
              auto_enabled: sumData.auto_enabled ?? sumData.autoEnabled ?? false,
              state: sumData.state || sumData.status || 'IDLE',
              armed: Boolean(sumData.armed),
              next_run_in_seconds: sumData.next_run_in_seconds ?? 0,
              daily_sent: sumData.daily_sent ?? sumData.dailySent ?? 0,
              daily_cap: sumData.daily_cap ?? sumData.dailyCap ?? 30,
              last_run_at: sumData.last_run_at || sumData.lastRunAt || null,
            })
            setCountdown(sumData.next_run_in_seconds ?? 0)
          }

          setPosts(postData || [])
          setOutreachList(outData || [])
          setReplies(repData || [])
          setRuns(runData || [])
          setSettings(setts)
          setProfile(prof)
        }
      } catch (err) {
        console.warn('Error loading dashboard data:', err)
      }
    }

    fetchInitialData()

    const pollerInterval = setInterval(() => {
      api.getDashboardSummary().then((sumData) => {
        if (!ignore && sumData) {
          setSummary((prev) => ({
            ...prev,
            auto_enabled: sumData.auto_enabled ?? sumData.autoEnabled ?? false,
            state: sumData.state || sumData.status || 'IDLE',
            armed: Boolean(sumData.armed),
            daily_sent: sumData.daily_sent ?? sumData.dailySent ?? prev.daily_sent,
            daily_cap: sumData.daily_cap ?? sumData.dailyCap ?? prev.daily_cap,
            last_run_at: sumData.last_run_at || sumData.lastRunAt || prev.last_run_at,
          }))
        }
      }).catch(() => {})
    }, 10000)

    return () => {
      ignore = true
      clearInterval(pollerInterval)
    }
  }, [])

  // Live 1-second countdown timer
  useEffect(() => {
    if (!summary.auto_enabled || !summary.armed) return

    const timer = setInterval(() => {
      setCountdown((prev) => (prev > 0 ? prev - 1 : 0))
    }, 1000)

    return () => clearInterval(timer)
  }, [summary.auto_enabled, summary.armed])

  // Toggle Auto Scheduler
  const handleToggleAuto = async () => {
    const next = !summary.auto_enabled
    const res = await api.toggleAuto(next)
    const isNowEnabled = res.auto_enabled ?? next
    setSummary((prev) => ({
      ...prev,
      auto_enabled: isNowEnabled,
      state: isNowEnabled ? 'IDLE' : 'PAUSED',
    }))
    showToast(isNowEnabled ? 'Autopilot armed & scheduled (runs every 2h if armed)' : 'Autopilot paused')
  }

  // Kill Switch
  const handleKillSwitch = async () => {
    await api.killSwitch()
    setSummary((prev) => ({
      ...prev,
      auto_enabled: false,
      state: 'PAUSED',
      armed: false,
    }))
    showToast('🚨 EMERGENCY KILL SWITCH ACTIVATED: Engine paused and disarmed.')
  }

  // Trigger Manual Run (Arms the engine for today)
  const handleTriggerRun = async () => {
    setIsRunning(true)
    try {
      await api.triggerRun({ query, timeFilter, sortBy })
      setRenderTimestamp(Date.now())
      setSummary((prev) => ({
        ...prev,
        state: 'RUNNING',
        armed: true,
      }))
      showToast(`Manual run started for "${query}" — Engine is now ARMED for today!`)
      setTimeout(async () => {
        setIsRunning(false)
        await loadAllData()
      }, 2000)
    } catch (err) {
      setIsRunning(false)
      showToast(`Run error: ${err.message}`)
    }
  }

  // On-demand thread check / poll
  const handlePollThreads = async () => {
    try {
      showToast('Checking monitored Gmail threads for replies & bounces...')
      const report = await api.pollNow()
      await loadAllData()
      showToast(
        `Poll complete: ${report.threads_checked || 0} threads checked, ${report.replies_recorded || 0} replies recorded, ${report.followup1_drafted || 0} follow-ups drafted.`
      )
    } catch (err) {
      showToast(`Poll error: ${err.message}`)
    }
  }

  // Drafts Multiselect Handlers
  const handleSelectDraft = (id) => {
    setSelectedDraftIds((prev) =>
      prev.includes(id) ? prev.filter((dId) => dId !== id) : [...prev, id]
    )
  }

  const handleSelectAllDrafts = () => {
    if (selectedDraftIds.length === drafts.length) {
      setSelectedDraftIds([])
    } else {
      setSelectedDraftIds(drafts.map((d) => d.id))
    }
  }

  const handleBulkApprove = async () => {
    if (selectedDraftIds.length === 0) return
    const count = selectedDraftIds.length
    await api.bulkApproveOutreach(selectedDraftIds)
    setSelectedDraftIds([])
    await loadAllData()
    showToast(`Approved ${count} outreach drafts into queue`)
  }

  const handleBulkSendNow = async () => {
    if (selectedDraftIds.length === 0) return
    const count = selectedDraftIds.length
    showToast(`Sending ${count} selected emails now via Gmail...`)
    await api.bulkSendOutreach(selectedDraftIds)
    setSelectedDraftIds([])
    await loadAllData()
    showToast(`Successfully dispatched ${count} selected emails`)
  }

  const handleBulkSkip = async () => {
    if (selectedDraftIds.length === 0) return
    const count = selectedDraftIds.length
    await api.bulkSkipOutreach(selectedDraftIds)
    setSelectedDraftIds([])
    await loadAllData()
    showToast(`Marked ${count} drafts as skipped`)
  }

  const handleApproveSingle = async (id) => {
    await api.approveOutreach(id)
    await loadAllData()
    showToast('Outreach approved and placed in queue')
  }

  const handleSendSingleNow = async (id) => {
    showToast('Dispatching email via Gmail...')
    await api.sendOutreachNow(id)
    await loadAllData()
    showToast('Email sent successfully!')
  }

  const handleSkipSingle = async (id) => {
    await api.skipOutreach(id)
    await loadAllData()
    showToast('Outreach skipped')
  }

  const handleStartEditDraft = (d) => {
    setEditingDraftId(d.id)
    setDraftEditValues({ subject: d.subject, body: d.body })
  }

  const handleSaveEditDraft = async (id) => {
    await api.updateOutreach(id, draftEditValues)
    setEditingDraftId(null)
    await loadAllData()
    showToast('Draft updated successfully')
  }

  // Settings & Profile Save
  const handleSaveSettings = async (e) => {
    e.preventDefault()
    try {
      await api.saveSettings(settings)
      await api.saveProfile(profile)
      showToast('Settings & candidate profile saved successfully!')
    } catch (err) {
      showToast(`Save error: ${err.message}`)
    }
  }

  const handleResumeUpload = async (e) => {
    const file = e.target.files?.[0]
    if (!file) return
    showToast(`Uploading and extracting text from ${file.name}...`)
    try {
      const res = await api.uploadResume(file)
      if (res?.profile) {
        setProfile(res.profile)
      }
      showToast(`Resume uploaded! Extracted ${res.extracted_chars || 0} characters for LLM context.`)
    } catch (err) {
      showToast(`Resume upload failed: ${err.message}`)
    }
  }

  // Buckets
  const drafts = outreachList.filter((o) => o.status === 'draft')
  const queue = outreachList.filter((o) => o.status === 'queued')
  const sent = outreachList.filter((o) => o.status === 'sent')
  const unrepliedSent = sent.filter((s) => !replies.some((r) => r.outreach_id === s.id))

  const isAllDraftsSelected = drafts.length > 0 && selectedDraftIds.length === drafts.length

  return (
    <main className="auto-shell">
      {/* ASCII Banner & Header */}
      <header className="app-header">
        <pre className="ascii-banner">{ASCII_LOGO}</pre>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: '0.8rem' }}>
          <div>
            <h1>Autonomous Job Hunter</h1>
            <p className="muted">
              Playwright headless scraping • Domain trust analysis • LLM drafting • Human review & Gmail dispatch
            </p>
          </div>
          <div>
            <Link to="/" className="nav-link">← Switch to Manual Mailer</Link>
          </div>
        </div>
      </header>

      {/* Toast Alert */}
      {toast ? (
        <div className="card" style={{ background: '#1c1917', borderLeft: '4px solid #f59e0b', padding: '0.7rem 1rem' }}>
          <p style={{ margin: 0, fontWeight: 600 }}>{toast}</p>
        </div>
      ) : null}

      {/* Telemetry Bar */}
      <section className="telemetry-bar">
        <div className="telemetry-group">
          {/* Autopilot Switch */}
          <label className="switch" title="Toggle autonomous search & email scheduler">
            <input type="checkbox" checked={summary.auto_enabled} onChange={handleToggleAuto} />
            <span className="slider"></span>
          </label>
          <span style={{ fontWeight: 600 }}>Autopilot: {summary.auto_enabled ? 'ON' : 'OFF'}</span>

          {/* Engine State Pill */}
          <span className={`pill pill-${summary.state.toLowerCase()}`}>
            {summary.state}
          </span>

          {/* Armed Safety Indicator */}
          {summary.armed ? (
            <span className="pill pill-armed" title="Engine was armed by a manual run today. Auto runs will execute.">
              ● ARMED (Today)
            </span>
          ) : (
            <span className="pill pill-unarmed" title="Engine needs 1 manual run today to arm auto-runs.">
              ○ UNARMED (Run manually first)
            </span>
          )}

          {/* Next Run Countdown */}
          <span className="muted" style={{ fontSize: '0.85rem' }}>
            Next scan:{' '}
            <strong>
              {!summary.auto_enabled
                ? 'Paused'
                : !summary.armed
                ? 'Awaiting 1st manual run'
                : formatCountdown(countdown)}
            </strong>
          </span>
        </div>

        <div className="telemetry-group">
          {/* Daily Cap Progress */}
          <div className="cap-meter">
            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.8rem' }}>
              <span className="muted">Daily Cap:</span>
              <span><strong>{summary.daily_sent} / {summary.daily_cap}</strong></span>
            </div>
            <div className="cap-bar-outer">
              <div
                className="cap-bar-inner"
                style={{ width: `${Math.min(100, (summary.daily_sent / (summary.daily_cap || 30)) * 100)}%` }}
              ></div>
            </div>
          </div>

          {/* Quick Check Threads Button */}
          <button type="button" className="btn-poll" onClick={handlePollThreads} title="Poll Gmail for replies & bounces">
            ⟳ Check Threads
          </button>

          {/* Emergency Kill Switch */}
          <button type="button" className="btn-kill" onClick={handleKillSwitch} title="Immediately halt engine and disarm">
            KILL SWITCH
          </button>
        </div>
      </section>

      {/* Run Panel */}
      <section className="card">
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '0.5rem' }}>
          <h2 style={{ margin: 0 }}>Manual Scrape & Pipeline Trigger</h2>
          <span className="muted" style={{ fontSize: '0.82rem' }}>
            {summary.armed ? '✓ Engine is currently ARMED for today' : '⚠️ Clicking "Run Now" arms the engine for today'}
          </span>
        </div>
        <div className="run-panel">
          <label>
            Search query
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="e.g. golang remote, founding react engineer"
            />
          </label>
          <label>
            Date posted
            <select value={timeFilter} onChange={(e) => setTimeFilter(e.target.value)}>
              <option value="24h">Past 24 hours</option>
              <option value="1w">Past 1 week</option>
              <option value="1m">Past 1 month</option>
            </select>
          </label>
          <label>
            Sort by
            <select value={sortBy} onChange={(e) => setSortBy(e.target.value)}>
              <option value="latest">Latest</option>
              <option value="relevance">Relevance</option>
            </select>
          </label>
          <button
            type="button"
            className="btn"
            onClick={handleTriggerRun}
            disabled={isRunning}
            style={{ height: '42px' }}
          >
            {isRunning ? 'Running...' : 'Run Now & Arm'}
          </button>
        </div>
      </section>

      {/* Tab Navigation */}
      <nav className="tabs-nav">
        <button
          type="button"
          className={`tab-btn ${activeTab === 'posts' ? 'active' : ''}`}
          onClick={() => setActiveTab('posts')}
        >
          Found Posts <span className="badge">{posts.length}</span>
        </button>
        <button
          type="button"
          className={`tab-btn ${activeTab === 'drafts' ? 'active' : ''}`}
          onClick={() => setActiveTab('drafts')}
        >
          Drafts & Approvals <span className="badge">{drafts.length}</span>
        </button>
        <button
          type="button"
          className={`tab-btn ${activeTab === 'queue' ? 'active' : ''}`}
          onClick={() => setActiveTab('queue')}
        >
          Queue <span className="badge">{queue.length}</span>
        </button>
        <button
          type="button"
          className={`tab-btn ${activeTab === 'sent' ? 'active' : ''}`}
          onClick={() => setActiveTab('sent')}
        >
          Sent <span className="badge">{sent.length}</span>
        </button>
        <button
          type="button"
          className={`tab-btn ${activeTab === 'replies' ? 'active' : ''}`}
          onClick={() => setActiveTab('replies')}
        >
          Replies & Tracker <span className="badge">{replies.length}</span>
        </button>
        <button
          type="button"
          className={`tab-btn ${activeTab === 'history' ? 'active' : ''}`}
          onClick={() => setActiveTab('history')}
        >
          Run History <span className="badge">{runs.length}</span>
        </button>
        <button
          type="button"
          className={`tab-btn ${activeTab === 'settings' ? 'active' : ''}`}
          onClick={() => setActiveTab('settings')}
        >
          Settings & Profile
        </button>
      </nav>

      {/* Tab Content 1: Found Posts */}
      {activeTab === 'posts' && (
        <section className="card">
          <h2>Discovered Opportunities ({posts.length})</h2>
          {posts.length === 0 ? (
            <p className="muted">No posts discovered yet. Trigger a run above or enable Autopilot.</p>
          ) : (
            posts.map((post) => (
              <div key={post.id} className="item-card">
                <div className="item-card-header">
                  <div>
                    <h3 style={{ margin: 0, fontSize: '1.05rem' }}>{post.role}</h3>
                    <p className="muted" style={{ fontSize: '0.88rem' }}>
                      <strong>{post.company}</strong> • posted by {post.author} • {new Date(post.found_at).toLocaleDateString()}
                    </p>
                  </div>
                  <div style={{ display: 'flex', gap: '0.4rem', flexWrap: 'wrap' }}>
                    <span className={`tag tag-${post.paid_status || 'unknown'}`}>
                      {(post.paid_status || 'UNKNOWN').toUpperCase()}
                    </span>
                    <span className="tag tag-remote">{(post.work_mode || 'UNKNOWN').toUpperCase()}</span>
                    {post.extracted?.is_suspicious || (post.skip_reason && post.skip_reason.includes('SPAM')) ? (
                      <span className="tag tag-bounce">⚠️ FAKE / SPAM FLAGGED</span>
                    ) : post.status === 'skipped' ? (
                      <span className="tag tag-skipped">SKIPPED: {post.skip_reason}</span>
                    ) : (
                      <span className="tag tag-paid">EXTRACTED</span>
                    )}
                  </div>
                </div>

                {(post.extracted?.is_suspicious || (post.skip_reason && post.skip_reason.includes('SPAM'))) && (
                  <div className="spam-warning-box">
                    <strong>⚠️ Potential Spam or Fake Job Flagged:</strong> {post.extracted?.suspicion_reason || post.skip_reason}
                  </div>
                )}
                <div style={{ display: 'flex', gap: '1rem', fontSize: '0.85rem', flexWrap: 'wrap' }} className="muted">
                  <span>Location: <strong>{post.location || 'Not specified'}</strong></span>
                  {post.extracted?.stipend ? (
                    <span>Stipend: <strong>₹{post.extracted.stipend.toLocaleString()}/mo</strong></span>
                  ) : null}
                  {post.extracted?.experience_years != null ? (
                    <span>Exp: <strong>{post.extracted.experience_years} yrs</strong></span>
                  ) : null}
                  <a href={post.url} target="_blank" rel="noopener noreferrer" style={{ color: '#60a5fa' }}>
                    View LinkedIn Post ↗
                  </a>
                </div>
              </div>
            ))
          )}
        </section>
      )}

      {/* Tab Content 2: Drafts & Approvals (With Multiselect) */}
      {activeTab === 'drafts' && (
        <section className="card">
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '0.8rem' }}>
            <div>
              <h2 style={{ margin: 0 }}>Review & Dispatch Drafts ({drafts.length})</h2>
              <p className="muted" style={{ margin: '0.2rem 0 0 0', fontSize: '0.88rem' }}>
                Strict human-in-the-loop: select drafts to approve into queue or dispatch immediately.
              </p>
            </div>
            {drafts.length > 0 && (
              <label style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', cursor: 'pointer', fontWeight: 600 }}>
                <input
                  type="checkbox"
                  style={{ width: 'auto' }}
                  checked={isAllDraftsSelected}
                  onChange={handleSelectAllDrafts}
                />
                Select All ({drafts.length})
              </label>
            )}
          </div>

          {/* Sticky Floating Bulk Actions Bar */}
          {selectedDraftIds.length > 0 && (
            <div className="bulk-toolbar">
              <span style={{ fontWeight: 700, color: '#60a5fa' }}>
                ✓ {selectedDraftIds.length} draft{selectedDraftIds.length > 1 ? 's' : ''} selected
              </span>
              <div style={{ display: 'flex', gap: '0.6rem', flexWrap: 'wrap' }}>
                <button type="button" className="btn" onClick={handleBulkApprove}>
                  ✓ Approve Selected ({selectedDraftIds.length})
                </button>
                <button type="button" className="btn-send-now" onClick={handleBulkSendNow}>
                  ✈ Send Selected Now ({selectedDraftIds.length})
                </button>
                <button type="button" className="btn btn-ghost" onClick={handleBulkSkip}>
                  ✕ Skip Selected
                </button>
                <button
                  type="button"
                  className="btn btn-ghost"
                  onClick={() => setSelectedDraftIds([])}
                  style={{ fontSize: '0.8rem' }}
                >
                  Deselect All
                </button>
              </div>
            </div>
          )}

          {drafts.length === 0 ? (
            <p className="muted" style={{ marginTop: '1rem' }}>No drafts awaiting review. New drafts generated by the pipeline will appear here.</p>
          ) : (
            drafts.map((d) => {
              const isSelected = selectedDraftIds.includes(d.id)
              const isEditing = editingDraftId === d.id

              return (
                <div
                  key={d.id}
                  className="item-card"
                  style={{
                    borderColor: isSelected ? '#3b82f6' : 'var(--line-1)',
                    background: isSelected ? 'rgba(59, 130, 246, 0.04)' : '#0c0c0c',
                  }}
                >
                  <div className="item-card-header">
                    <div style={{ display: 'flex', alignItems: 'flex-start', gap: '0.8rem' }}>
                      <input
                        type="checkbox"
                        checked={isSelected}
                        onChange={() => handleSelectDraft(d.id)}
                        style={{ marginTop: '0.3rem', width: 'auto', cursor: 'pointer' }}
                      />
                      <div>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                          <h3 style={{ margin: 0 }}>{d.subject}</h3>
                          <span className={`pill ${d.kind === 'initial' ? 'pill-idle' : 'pill-armed'}`} style={{ fontSize: '0.68rem', padding: '0.1rem 0.4rem' }}>
                            {d.kind ? d.kind.toUpperCase() : 'INITIAL'}
                          </span>
                        </div>
                        <p className="muted" style={{ fontSize: '0.85rem', marginTop: '0.2rem' }}>
                          To: <strong>{d.contact_email}</strong> • {d.company} ({d.role})
                        </p>
                      </div>
                    </div>

                    <div className="row" style={{ gap: '0.4rem' }}>
                      <button type="button" className="btn" onClick={() => handleApproveSingle(d.id)} title="Add to automated schedule queue">
                        ✓ Approve
                      </button>
                      <button type="button" className="btn-send-now" onClick={() => handleSendSingleNow(d.id)} title="Send email immediately via Gmail">
                        ✈ Send Now
                      </button>
                      <button
                        type="button"
                        className="btn btn-ghost"
                        onClick={() => (isEditing ? handleSaveEditDraft(d.id) : handleStartEditDraft(d))}
                      >
                        {isEditing ? 'Save' : '✎ Edit'}
                      </button>
                      <button type="button" className="btn btn-ghost" onClick={() => handleSkipSingle(d.id)}>
                        ✕ Skip
                      </button>
                    </div>
                  </div>

                  {(d.is_suspicious || (d.trust_score != null && d.trust_score < 40)) && (
                    <div className="spam-warning-box">
                      <strong>⚠️ SPAM / LOW-TRUST WARNING:</strong> This recipient or job post was flagged with suspicious signals. Review carefully before sending.
                    </div>
                  )}

                  {isEditing ? (
                    <div style={{ display: 'grid', gap: '0.5rem', marginTop: '0.5rem' }}>
                      <input
                        value={draftEditValues.subject}
                        onChange={(e) => setDraftEditValues({ ...draftEditValues, subject: e.target.value })}
                        placeholder="Subject line"
                      />
                      <textarea
                        rows="6"
                        value={draftEditValues.body}
                        onChange={(e) => setDraftEditValues({ ...draftEditValues, body: e.target.value })}
                      />
                    </div>
                  ) : (
                    <div className="email-preview-box">{d.body}</div>
                  )}
                </div>
              )
            })
          )}
        </section>
      )}

      {/* Tab Content 3: Queue */}
      {activeTab === 'queue' && (
        <section className="card">
          <h2>Outreach Queue ({queue.length})</h2>
          <p className="muted">Approved emails scheduled to send within your daily window respecting time gaps.</p>
          {queue.length === 0 ? (
            <p className="muted">Queue is currently empty.</p>
          ) : (
            queue.map((q) => (
              <div key={q.id} className="item-card">
                <div className="item-card-header">
                  <div>
                    <h3 style={{ margin: 0 }}>{q.subject}</h3>
                    <p className="muted" style={{ fontSize: '0.85rem' }}>
                      To: <strong>{q.contact_email}</strong> • {q.company}
                    </p>
                  </div>
                  <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center' }}>
                    <span className="tag tag-remote">
                      Scheduled: {q.scheduled_at ? new Date(q.scheduled_at).toLocaleTimeString() : 'Pending gap'}
                    </span>
                    <button type="button" className="btn-send-now" onClick={() => handleSendSingleNow(q.id)}>
                      ✈ Send Now
                    </button>
                  </div>
                </div>
                <div className="email-preview-box">{q.body}</div>
              </div>
            ))
          )}
        </section>
      )}

      {/* Tab Content 4: Sent */}
      {activeTab === 'sent' && (
        <section className="card">
          <h2>Sent Emails ({sent.length})</h2>
          {sent.length === 0 ? (
            <p className="muted">No sent emails recorded yet.</p>
          ) : (
            sent.map((s) => (
              <div key={s.id} className="item-card">
                <div className="item-card-header">
                  <div>
                    <h3 style={{ margin: 0 }}>{s.subject}</h3>
                    <p className="muted" style={{ fontSize: '0.85rem' }}>
                      To: <strong>{s.contact_email}</strong> • {s.company} • Sent {new Date(s.sent_at).toLocaleString()}
                    </p>
                  </div>
                  <span className="tag tag-paid">DELIVERED</span>
                </div>
                {s.gmail_message_id ? (
                  <p className="muted" style={{ fontSize: '0.78rem' }}>
                    Gmail Message ID: <code>{s.gmail_message_id}</code>
                  </p>
                ) : null}
              </div>
            ))
          )}
        </section>
      )}

      {/* Tab Content 5: Replies & Unreplied Follow-up Tracker */}
      {activeTab === 'replies' && (
        <div style={{ display: 'grid', gap: '1.2rem' }}>
          {/* Section A: Inbound Replies */}
          <section className="card">
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '0.6rem' }}>
              <div>
                <h2 style={{ margin: 0 }}>Inbound Replies ({replies.length})</h2>
                <p className="muted" style={{ margin: '0.2rem 0 0 0', fontSize: '0.88rem' }}>
                  Replies detected by the 30-minute background thread poller.
                </p>
              </div>
              <button type="button" className="btn-poll" onClick={handlePollThreads}>
                ⟳ Poll Gmail Now
              </button>
            </div>

            {replies.length === 0 ? (
              <p className="muted">No inbound replies recorded yet. Poller runs every 30 minutes while server is active.</p>
            ) : (
              replies.map((r) => (
                <div key={r.id} className="item-card" style={r.is_bounce ? { borderLeft: '4px solid #ef4444' } : { borderLeft: '4px solid #10b981' }}>
                  <div className="item-card-header">
                    <div>
                      <h3 style={{ margin: 0 }}>{r.subject}</h3>
                      <p className="muted" style={{ fontSize: '0.85rem' }}>
                        From: <strong>{r.from}</strong> • {new Date(r.received_at).toLocaleString()}
                      </p>
                    </div>
                    {r.is_bounce ? (
                      <span className="tag tag-bounce">BOUNCE / FAILED</span>
                    ) : (
                      <span className="tag tag-paid">REPLIED</span>
                    )}
                  </div>
                  <p style={{ margin: '0.4rem 0', fontSize: '0.9rem', color: '#e4e4e7' }}>{r.snippet}</p>
                  {r.bounce_reason ? (
                    <p className="muted" style={{ fontSize: '0.8rem', color: '#fca5a5', margin: 0 }}>
                      Bounce Reason: {r.bounce_reason} (Recipient contact invalidated to prevent further attempts)
                    </p>
                  ) : null}
                </div>
              ))
            )}
          </section>

          {/* Section B: Sent Emails Awaiting Replies */}
          <section className="card">
            <h2>Outreach Awaiting Reply ({unrepliedSent.length})</h2>
            <p className="muted" style={{ fontSize: '0.88rem' }}>
              Tracks follow-up eligibility. If unreplied after 24h, the poller drafts a follow-up for your manual approval.
            </p>
            {unrepliedSent.length === 0 ? (
              <p className="muted">No pending sent emails awaiting responses.</p>
            ) : (
              unrepliedSent.map((item) => {
                const hoursAgo = item.sent_at
                  ? Math.floor((renderTimestamp - new Date(item.sent_at).getTime()) / (1000 * 3600))
                  : 0

                return (
                  <div key={item.id} className="item-card">
                    <div className="item-card-header">
                      <div>
                        <h3 style={{ margin: 0 }}>{item.subject}</h3>
                        <p className="muted" style={{ fontSize: '0.85rem' }}>
                          To: <strong>{item.contact_email}</strong> • Sent {hoursAgo}h ago
                        </p>
                      </div>
                      <span className="pill pill-idle">
                        {hoursAgo >= 24 ? 'FOLLOW-UP ELIGIBLE' : `WAITING (${Math.max(0, 24 - hoursAgo)}h left)`}
                      </span>
                    </div>
                  </div>
                )
              })
            )}
          </section>
        </div>
      )}

      {/* Tab Content 6: Run History */}
      {activeTab === 'history' && (
        <section className="card">
          <h2>Pipeline Run Logs ({runs.length})</h2>
          <p className="muted">History of manual triggers and autonomous scheduled scans.</p>
          {runs.length === 0 ? (
            <p className="muted">No execution runs recorded yet.</p>
          ) : (
            <div style={{ overflowX: 'auto' }}>
              <table className="runs-table">
                <thead>
                  <tr>
                    <th>Date / Time</th>
                    <th>Trigger</th>
                    <th>Query</th>
                    <th>Window</th>
                    <th>Status</th>
                    <th>Pipeline Metrics</th>
                  </tr>
                </thead>
                <tbody>
                  {runs.map((r) => {
                    const stats = r.stats || {}
                    const runDate = r.created_at || r.started_at
                    return (
                      <tr key={r.id}>
                        <td>{runDate ? new Date(runDate).toLocaleString() : 'N/A'}</td>
                        <td>
                          <span className={`pill ${r.trigger === 'auto' ? 'pill-armed' : 'pill-idle'}`} style={{ fontSize: '0.7rem' }}>
                            {(r.trigger || 'manual').toUpperCase()}
                          </span>
                        </td>
                        <td><code>{r.query || 'Default'}</code></td>
                        <td>{r.time_filter || '24h'} / {r.sort_by || 'latest'}</td>
                        <td>
                          <span className={`tag ${r.status === 'completed' ? 'tag-paid' : 'tag-unpaid'}`}>
                            {(r.status || 'completed').toUpperCase()}
                          </span>
                        </td>
                        <td>
                          <span className="stat-pill stat-found">Found: {stats.found ?? 0}</span>
                          <span className="stat-pill stat-passed">Filtered: {stats.filtered ?? 0}</span>
                          <span className="stat-pill stat-passed">Verified: {stats.verified ?? 0}</span>
                          <span className="stat-pill stat-skipped">Skipped: {stats.skipped ?? 0}</span>
                          <span className="stat-pill stat-drafted">Drafted: {stats.drafted ?? 0}</span>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </section>
      )}

      {/* Tab Content 7: Settings & Profile */}
      {activeTab === 'settings' && settings && profile && (
        <form onSubmit={handleSaveSettings} style={{ display: 'grid', gap: '1.2rem' }}>
          {/* Profile Section */}
          <section className="card">
            <h2>Candidate Profile & Resume (LLM Context)</h2>
            <label>
              Professional summary (for prompt personalization)
              <textarea
                rows="3"
                value={profile.summary || ''}
                onChange={(e) => setProfile({ ...profile, summary: e.target.value })}
                placeholder="Senior Full Stack Engineer with expertise in Go, React..."
              />
            </label>
            <label>
              Core skills (comma separated)
              <input
                value={profile.skills || ''}
                onChange={(e) => setProfile({ ...profile, skills: e.target.value })}
                placeholder="Go, React 19, TypeScript, PostgreSQL, Docker"
              />
            </label>
            <label>
              Example cold email style (optional template reference)
              <textarea
                rows="4"
                value={profile.example_text || ''}
                onChange={(e) => setProfile({ ...profile, example_text: e.target.value })}
                placeholder="Hi {name}, I noticed your post about {role}..."
              />
            </label>

            <div>
              <p style={{ margin: '0 0 0.5rem 0', fontWeight: 600 }}>Resume PDF Upload</p>
              <div className="dropzone">
                <input
                  type="file"
                  accept="application/pdf"
                  onChange={handleResumeUpload}
                  style={{ display: 'none' }}
                  id="resume-file-input"
                />
                <label htmlFor="resume-file-input" style={{ cursor: 'pointer' }}>
                  <p style={{ margin: 0, fontSize: '0.95rem' }}>📄 Click to select and upload your Resume PDF</p>
                  <p className="muted" style={{ fontSize: '0.8rem', marginTop: '0.2rem' }}>
                    PDF text will be extracted automatically on disk and injected into prompt context.
                  </p>
                </label>
              </div>
              {profile.resume_path ? (
                <p className="muted" style={{ fontSize: '0.85rem', marginTop: '0.4rem' }}>
                  Stored at: <code>{profile.resume_path}</code> ({profile.resume_text?.length || 0} chars extracted)
                </p>
              ) : null}
            </div>
          </section>

          {/* Filters & Configuration */}
          <section className="card">
            <h2>Outreach Filters & Limits (null = disabled)</h2>
            <div className="grid-2">
              <label>
                Min Stipend / Monthly Pay (₹)
                <input
                  type="number"
                  value={settings.min_stipend ?? ''}
                  onChange={(e) =>
                    setSettings({
                      ...settings,
                      min_stipend: e.target.value === '' ? null : Number(e.target.value),
                    })
                  }
                  placeholder="e.g. 15000"
                />
              </label>

              <label>
                Daily Email Cap
                <input
                  type="number"
                  value={settings.daily_cap ?? 30}
                  onChange={(e) => setSettings({ ...settings, daily_cap: Number(e.target.value) })}
                />
              </label>

              <label>
                Allowed locations (comma-separated, leave blank for all)
                <input
                  value={settings.allowed_locations ? settings.allowed_locations.join(', ') : ''}
                  onChange={(e) =>
                    setSettings({
                      ...settings,
                      allowed_locations: e.target.value ? e.target.value.split(',').map((s) => s.trim()) : null,
                    })
                  }
                  placeholder="Bangalore, Remote, Delhi"
                />
              </label>

              <label>
                Work modes (comma-separated: remote, hybrid, onsite)
                <input
                  value={settings.work_modes ? settings.work_modes.join(', ') : ''}
                  onChange={(e) =>
                    setSettings({
                      ...settings,
                      work_modes: e.target.value ? e.target.value.split(',').map((s) => s.trim()) : null,
                    })
                  }
                  placeholder="remote, hybrid"
                />
              </label>

              <label>
                Include keywords (must match at least one)
                <input
                  value={settings.include_keywords ? settings.include_keywords.join(', ') : ''}
                  onChange={(e) =>
                    setSettings({
                      ...settings,
                      include_keywords: e.target.value ? e.target.value.split(',').map((s) => s.trim()) : null,
                    })
                  }
                  placeholder="golang, react, full stack"
                />
              </label>

              <label>
                Exclude keywords (rejects if present)
                <input
                  value={settings.exclude_keywords ? settings.exclude_keywords.join(', ') : ''}
                  onChange={(e) =>
                    setSettings({
                      ...settings,
                      exclude_keywords: e.target.value ? e.target.value.split(',').map((s) => s.trim()) : null,
                    })
                  }
                  placeholder="crypto, web3, blockchain"
                />
              </label>

              <label>
                Min Experience Years
                <input
                  type="number"
                  step="0.5"
                  value={settings.min_experience_years ?? ''}
                  onChange={(e) =>
                    setSettings({
                      ...settings,
                      min_experience_years: e.target.value === '' ? null : Number(e.target.value),
                    })
                  }
                  placeholder="e.g. 1"
                />
              </label>

              <label>
                Max Experience Years
                <input
                  type="number"
                  step="0.5"
                  value={settings.max_experience_years ?? ''}
                  onChange={(e) =>
                    setSettings({
                      ...settings,
                      max_experience_years: e.target.value === '' ? null : Number(e.target.value),
                    })
                  }
                  placeholder="e.g. 5"
                />
              </label>

              <label>
                Send Window (Time Range & TZ)
                <input
                  value={settings.send_window || '09:00-19:00 IST'}
                  onChange={(e) => setSettings({ ...settings, send_window: e.target.value })}
                />
              </label>

              <label>
                Dispatch Gaps (Min / Max minutes)
                <div style={{ display: 'flex', gap: '0.5rem' }}>
                  <input
                    type="number"
                    value={settings.gap_min_minutes ?? 2}
                    onChange={(e) => setSettings({ ...settings, gap_min_minutes: Number(e.target.value) })}
                    placeholder="Min"
                  />
                  <input
                    type="number"
                    value={settings.gap_max_minutes ?? 6}
                    onChange={(e) => setSettings({ ...settings, gap_max_minutes: Number(e.target.value) })}
                    placeholder="Max"
                  />
                </div>
              </label>
            </div>

            <div style={{ display: 'flex', gap: '1.5rem', flexWrap: 'wrap', marginTop: '0.8rem' }}>
              <label style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  style={{ width: 'auto' }}
                  checked={settings.skip_unpaid ?? true}
                  onChange={(e) => setSettings({ ...settings, skip_unpaid: e.target.checked })}
                />
                Skip Unpaid Opportunities
              </label>

              <label style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  style={{ width: 'auto' }}
                  checked={settings.skip_unknown_paid ?? false}
                  onChange={(e) => setSettings({ ...settings, skip_unknown_paid: e.target.checked })}
                />
                Skip Unknown Paid Status
              </label>

              <label style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  style={{ width: 'auto' }}
                  checked={settings.approval_mode ?? true}
                  onChange={(e) => setSettings({ ...settings, approval_mode: e.target.checked })}
                />
                Require Manual Approval Before Dispatch
              </label>
            </div>

            <div className="row" style={{ marginTop: '1rem' }}>
              <button type="submit" className="btn">
                Save Settings & Profile
              </button>
            </div>
          </section>
        </form>
      )}

      {/* Footer */}
      <footer className="app-footer">
        <p>Autonomous Job Engine • Go, Chi, PostgreSQL, Playwright & Gemini 2.5 Flash</p>
      </footer>
    </main>
  )
}
