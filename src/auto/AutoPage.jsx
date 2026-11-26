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

function parsePostedAgeMinutes(postedAt) {
  if (!postedAt) return Number.MAX_SAFE_INTEGER
  const m = String(postedAt).trim().toLowerCase().match(/^(\d+)\s*(m|min|mins|h|hr|hrs|d|w|mo|yr)/)
  if (!m) return Number.MAX_SAFE_INTEGER
  const val = parseInt(m[1], 10)
  const unit = m[2]
  if (unit.startsWith('m') && !unit.startsWith('mo')) return val
  if (unit.startsWith('h')) return val * 60
  if (unit.startsWith('d')) return val * 1440
  if (unit.startsWith('w')) return val * 10080
  if (unit.startsWith('mo')) return val * 43200
  if (unit.startsWith('y')) return val * 525600
  return Number.MAX_SAFE_INTEGER
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
    daily_cap: 100,
    last_run_at: null,
  })
  const [countdown, setCountdown] = useState(0)

  // Run panel state
  const [query, setQuery] = useState('')
  const [timeFilter, setTimeFilter] = useState('24h')
  const [sortBy, setSortBy] = useState('latest')
  const [isRunning, setIsRunning] = useState(false)

  // Data states
  const [posts, setPosts] = useState([])
  const [contacts, setContacts] = useState([])
  const [outreachList, setOutreachList] = useState([])
  const [replies, setReplies] = useState([])
  const [runs, setRuns] = useState([])
  const [settings, setSettings] = useState(null)
  const [profile, setProfile] = useState(null)
  const [authStatus, setAuthStatus] = useState({ connected: false, email: '' })
  const [toast, setToast] = useState('')

  // Multiselect state for Drafts, Queue, and Found Posts
  const [selectedDraftIds, setSelectedDraftIds] = useState([])
  const [selectedQueueIds, setSelectedQueueIds] = useState([])
  const [selectedPostIds, setSelectedPostIds] = useState([])
  const [dismissedAlreadyContactedIds, setDismissedAlreadyContactedIds] = useState([])
  const [postFilter, setPostFilter] = useState('all') // all | email | phone | dm
  const [onlyReadyToDraft, setOnlyReadyToDraft] = useState(false)
  const [editingDraftId, setEditingDraftId] = useState(null)
  const [draftEditValues, setDraftEditValues] = useState({ subject: '', body: '' })
  const [renderTimestamp, setRenderTimestamp] = useState(() => Date.now())
  const [selectedRun, setSelectedRun] = useState(null)
  const [actionLoadingId, setActionLoadingId] = useState(null)
  const [scrapeProgress, setScrapeProgress] = useState(null)

  const showToast = (msg) => {
    setToast(msg)
    setTimeout(() => setToast(''), 4500)
  }

  const loadAllData = useCallback(async () => {
    try {
      const [sumData, postData, contactData, outData, repData, runData, setts, prof, authData] = await Promise.all([
        api.getDashboardSummary(),
        api.getPosts(),
        api.getContacts(),
        api.getOutreach(),
        api.getReplies(),
        api.getRuns(),
        api.getSettings(),
        api.getProfile(),
        api.getAuthStatus(),
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
      setContacts(contactData || [])
      setOutreachList(outData || [])
      setReplies(repData || [])
      setRuns(runData || [])
      setSettings(setts)
      setProfile(prof)
      if (authData) setAuthStatus(authData)
    } catch (err) {
      console.warn('Error loading dashboard data:', err)
    }
  }, [])

  // Check URL params for OAuth redirect callback
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    if (params.get('auth') === 'success') {
      const email = params.get('email')
      showToast(`✓ Google Account connected successfully${email ? ' as ' + email : ''}!`)
      window.history.replaceState({}, document.title, window.location.pathname)
    } else if (params.get('error')) {
      showToast(`Google sign-in error: ${params.get('error')}`)
      window.history.replaceState({}, document.title, window.location.pathname)
    }
  }, [])

  // Initial load + periodic poller for orchestrator status (every 10s)
  useEffect(() => {
    // Clear legacy mock state from localStorage
    try {
      localStorage.removeItem('emailauto.mock_state.v1')
    } catch {
      // Ignore storage access errors
    }

    let ignore = false

    async function fetchInitialData() {
      try {
        const [sumData, postData, contactData, outData, repData, runData, setts, prof, authData] = await Promise.all([
          api.getDashboardSummary(),
          api.getPosts(),
          api.getContacts(),
          api.getOutreach(),
          api.getReplies(),
          api.getRuns(),
          api.getSettings(),
          api.getProfile(),
          api.getAuthStatus(),
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
          setContacts(contactData || [])
          setOutreachList(outData || [])
          setReplies(repData || [])
          setRuns(runData || [])
          setSettings(setts)
          setProfile(prof)
          if (authData) setAuthStatus(authData)
        }
      } catch (err) {
        console.warn('Error loading dashboard data:', err)
      }
    }

    fetchInitialData()

    const pollerInterval = setInterval(() => {
      api.getDashboardSummary().then((sumData) => {
        if (!ignore && sumData) {
          const nextState = sumData.state || sumData.status || 'IDLE'
          setSummary((prev) => {
            if ((prev.state === 'RUNNING' || prev.state === 'SEARCHING') && nextState === 'IDLE') {
              // Background run just finished — stop spinner, reload posts/drafts/runs, and notify user
              setIsRunning(false)
              setTimeout(() => {
                loadAllData()
                showToast('✓ Scrape & pipeline run finished! Found Posts & Drafts updated.')
              }, 50)
            }
            return {
              ...prev,
              auto_enabled: sumData.auto_enabled ?? sumData.autoEnabled ?? false,
              state: nextState,
              armed: Boolean(sumData.armed),
              daily_sent: sumData.daily_sent ?? sumData.dailySent ?? prev.daily_sent,
              daily_cap: sumData.daily_cap ?? sumData.dailyCap ?? prev.daily_cap,
              last_run_at: sumData.last_run_at || sumData.lastRunAt || prev.last_run_at,
            }
          })
        }
      }).catch(() => {})
    }, 4000)

    return () => {
      ignore = true
      clearInterval(pollerInterval)
    }
  }, [loadAllData])

  // Live 1-second countdown timer
  useEffect(() => {
    if (!summary.auto_enabled || !summary.armed) return

    const timer = setInterval(() => {
      setCountdown((prev) => (prev > 0 ? prev - 1 : 0))
    }, 1000)

    return () => clearInterval(timer)
  }, [summary.auto_enabled, summary.armed])

  // Live worker progress + live queue/posts refresh while scraping/processing is active
  useEffect(() => {
    if (!isRunning && summary.state !== 'RUNNING' && summary.state !== 'SEARCHING') {
      setScrapeProgress(null)
      return
    }

    let cancelled = false
    const pollProgress = async () => {
      const status = await api.getWorkerProgress()
      if (!cancelled && status?.progress) {
        setScrapeProgress(status.progress)
      }
    }
    pollProgress()
    const iv = setInterval(pollProgress, 1500)
    const dataRefreshIv = setInterval(() => {
      if (!cancelled) loadAllData()
    }, 6000)
    return () => {
      cancelled = true
      clearInterval(iv)
      clearInterval(dataRefreshIv)
    }
  }, [isRunning, summary.state, loadAllData])

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

  // Cancel Running Scrape
  const handleCancelRun = async () => {
    try {
      showToast('⏹ Cancelling active scrape and halting engine...')
      await api.cancelRun()
      setIsRunning(false)
      setSummary((prev) => ({
        ...prev,
        state: 'IDLE',
      }))
      await loadAllData()
      showToast('🛑 Active scrape cancelled.')
    } catch (err) {
      setIsRunning(false)
      showToast(`Cancel error: ${err.message}`)
    }
  }

  // Trigger Manual Run (Arms the engine for today)
  const handleTriggerRun = async () => {
    const trimmedQuery = query.trim()
    if (!trimmedQuery) {
      showToast('⚠️ Please enter a search query (e.g. hiring flutter, golang developer, react full stack)')
      return
    }
    setIsRunning(true)
    try {
      setSummary((prev) => ({
        ...prev,
        state: 'RUNNING',
      }))
      const res = await api.triggerRun({ query: trimmedQuery, timeFilter, sortBy, maxPosts: 500 })
      setRenderTimestamp(Date.now())

      if (res?.started) {
        setSummary((prev) => ({ ...prev, state: 'RUNNING', armed: true }))
        showToast(`⚡ Scraping LinkedIn for "${trimmedQuery}" in background — watch live progress below!`)
        return
      }

      if (res?.error || (res?.run && res.run.status === 'failed')) {
        const issue = res.run?.stats?.issue || res.run?.stats?.error || res.error || res.message || 'Scrape execution failed'
        showToast(`⚠️ Run Notice: ${issue}`)
        if (res.run) setSelectedRun(res.run)
        setIsRunning(false)
      } else {
        const stats = res?.run?.stats || {}
        showToast(`✓ Processed ${stats.found ?? 0} posts for "${trimmedQuery}" • Verified: ${stats.verified ?? 0} • Drafted: ${stats.drafted ?? 0} • Skipped: ${stats.skipped ?? 0}`)
        if (res.run) setSelectedRun(res.run)
        setIsRunning(false)
      }

      await loadAllData()
    } catch (err) {
      showToast(`Run error: ${err.message}`)
      setIsRunning(false)
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

  // Launch persistent Chrome for LinkedIn login
  const handleOpenLoginBrowser = async () => {
    showToast('Launching Chrome browser for LinkedIn login...')
    const res = await api.openLinkedInLogin()
    if (res?.message) {
      showToast(res.message)
    }
  }

  // On-demand draft generation from a found post
  const handleCreateDraftForPost = async (post, force = false) => {
    setActionLoadingId('draft_' + post.id)
    try {
      showToast(`${force ? 'Creating resend draft' : 'Generating cold email draft'} for ${post.company || post.role}...`)
      const matchingContact = contacts.find(
        (c) => c.post_id === post.id || (post.extracted?.emails && post.extracted.emails.includes(c.email))
      )
      const contactId = matchingContact?.id || ''
      const res = await api.generateDraft({ postId: post.id, contactId, force })
      await loadAllData()
      setActiveTab('drafts')
      if (res?.subject) {
        showToast(`✓ Cold email drafted: "${res.subject}"! Switched to Drafts & Approvals.`)
      } else {
        showToast(`✓ Cold email draft generated! Switched to Drafts & Approvals.`)
      }
    } catch (err) {
      showToast(`Draft generation failed: ${err.message}`)
    } finally {
      setActionLoadingId(null)
    }
  }

  // Clear all discovered opportunities
  const handleClearAllPosts = async () => {
    if (!window.confirm(`Are you sure you want to clear all ${posts.length} discovered opportunities?`)) {
      return
    }
    setActionLoadingId('clear_posts')
    try {
      showToast('Clearing all discovered opportunities...')
      await api.clearAllPosts()
      setPosts([])
      setSelectedPostIds([])
      await loadAllData()
      showToast('✓ All discovered opportunities cleared!')
    } catch (err) {
      showToast(`Error clearing posts: ${err.message}`)
    } finally {
      setActionLoadingId(null)
    }
  }

  // Found Posts Multiselect & Batch Draft Handlers
  const handleSelectPost = (id) => {
    setSelectedPostIds((prev) =>
      prev.includes(id) ? prev.filter((pId) => pId !== id) : [...prev, id]
    )
  }

  const handleSelectAllPosts = (visiblePosts) => {
    const draftableIds = visiblePosts
      .filter((p) => p.extracted?.emails?.length > 0)
      .map((p) => p.id)
    if (draftableIds.length > 0 && draftableIds.every((id) => selectedPostIds.includes(id))) {
      setSelectedPostIds([])
    } else {
      setSelectedPostIds(draftableIds)
    }
  }

  const handleBulkDraftPosts = async () => {
    if (selectedPostIds.length === 0) return
    const count = selectedPostIds.length
    setActionLoadingId('bulk_draft_posts')
    showToast(`Batch drafting ${count} selected posts using Gemini...`)
    try {
      const res = await api.generateDraftBatch({ postIds: selectedPostIds })
      setSelectedPostIds([])
      await loadAllData()
      setActiveTab('drafts')
      showToast(`✓ Drafted ${res?.drafted ?? 0} cold emails${res?.skipped_duplicates ? ` (${res.skipped_duplicates} duplicate/existing skipped)` : ''}! Switched to Drafts & Approvals.`)
    } catch (err) {
      showToast(`Batch draft error: ${err.message}`)
    } finally {
      setActionLoadingId(null)
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
    setActionLoadingId('bulk_approve')
    try {
      await api.bulkApproveOutreach(selectedDraftIds)
      setSelectedDraftIds([])
      await loadAllData()
      showToast(`Approved ${count} outreach drafts into queue`)
    } catch (err) {
      showToast(`Bulk approve error: ${err.message}`)
    } finally {
      setActionLoadingId(null)
    }
  }

  const handleBulkSendNow = async () => {
    if (selectedDraftIds.length === 0) return
    const count = selectedDraftIds.length
    setActionLoadingId('bulk_send')
    showToast(`Sending ${count} selected emails now via Gmail...`)
    try {
      const res = await api.bulkSendOutreach(selectedDraftIds)
      setSelectedDraftIds([])
      await loadAllData()
      if (res?.sentCount > 0 && res?.failedCount === 0) {
        showToast(`✓ Successfully sent ${res.sentCount} emails via Gmail!`)
      } else if (res?.sentCount > 0 && res?.failedCount > 0) {
        showToast(`⚠️ Sent ${res.sentCount}, failed ${res.failedCount}: ${res.reason}`)
      } else if (res?.reason) {
        showToast(`❌ Send failed (${res.failedCount || count}): ${res.reason}`)
      } else {
        showToast(`Dispatched ${count} selected emails`)
      }
    } catch (err) {
      showToast(`Bulk send error: ${err.message}`)
    } finally {
      setActionLoadingId(null)
    }
  }

  // Queue Multiselect Handlers
  const handleSelectQueue = (id) => {
    setSelectedQueueIds((prev) =>
      prev.includes(id) ? prev.filter((qId) => qId !== id) : [...prev, id]
    )
  }

  const handleSelectAllQueue = () => {
    if (selectedQueueIds.length === queue.length) {
      setSelectedQueueIds([])
    } else {
      setSelectedQueueIds(queue.map((q) => q.id))
    }
  }

  const handleBulkSendQueue = async () => {
    if (selectedQueueIds.length === 0) return
    const count = selectedQueueIds.length
    setActionLoadingId('bulk_send_queue')
    showToast(`Sending ${count} queued emails now via Gmail...`)
    try {
      const res = await api.bulkSendOutreach(selectedQueueIds)
      setSelectedQueueIds([])
      await loadAllData()
      if (res?.sentCount > 0 && res?.failedCount === 0) {
        showToast(`✓ Successfully sent ${res.sentCount} queued emails via Gmail!`)
      } else if (res?.sentCount > 0 && res?.failedCount > 0) {
        showToast(`⚠️ Sent ${res.sentCount}, failed ${res.failedCount}: ${res.reason}`)
      } else if (res?.reason) {
        showToast(`❌ Queue send failed (${res.failedCount || count}): ${res.reason}`)
      } else {
        showToast(`Dispatched ${count} queued emails`)
      }
    } catch (err) {
      showToast(`Queue bulk send error: ${err.message}`)
    } finally {
      setActionLoadingId(null)
    }
  }

  const handleCancelQueuedSingle = async (id) => {
    setActionLoadingId('cancel_queue_' + id)
    try {
      await api.skipOutreach(id)
      setSelectedQueueIds((prev) => prev.filter((qId) => qId !== id))
      await loadAllData()
      showToast('✕ Removed email from queue')
    } catch (err) {
      showToast(`Cancel error: ${err.message}`)
    } finally {
      setActionLoadingId(null)
    }
  }

  const handleBulkCancelQueue = async () => {
    if (selectedQueueIds.length === 0) return
    const count = selectedQueueIds.length
    setActionLoadingId('bulk_cancel_queue')
    try {
      await api.bulkSkipOutreach(selectedQueueIds)
      setSelectedQueueIds([])
      await loadAllData()
      showToast(`✕ Cancelled ${count} queued email${count > 1 ? 's' : ''}`)
    } catch (err) {
      showToast(`Bulk cancel error: ${err.message}`)
    } finally {
      setActionLoadingId(null)
    }
  }

  const handleEmptyQueue = async () => {
    if (queue.length === 0) return
    if (!window.confirm(`Are you sure you want to cancel and empty all ${queue.length} queued emails?`)) {
      return
    }
    setActionLoadingId('empty_queue')
    try {
      await api.clearQueue()
      setSelectedQueueIds([])
      await loadAllData()
      showToast('✓ Outreach queue emptied!')
    } catch (err) {
      showToast(`Empty queue error: ${err.message}`)
    } finally {
      setActionLoadingId(null)
    }
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
    setActionLoadingId('approve_' + id)
    try {
      await api.approveOutreach(id)
      await loadAllData()
      showToast('Outreach approved and placed in queue')
    } catch (err) {
      showToast(`Approve error: ${err.message}`)
    } finally {
      setActionLoadingId(null)
    }
  }

  const handleSendSingleNow = async (id) => {
    setActionLoadingId('send_' + id)
    showToast('Dispatching email via Gmail...')
    try {
      const res = await api.sendOutreachNow(id)
      await loadAllData()
      if (res?.sent) {
        showToast('✓ Email sent successfully via Gmail!')
      } else if (res?.reason) {
        showToast(`⚠️ Send notice: ${res.reason}`)
      } else {
        showToast('Dispatch request completed')
      }
    } catch (err) {
      showToast(`Send error: ${err.message}`)
    } finally {
      setActionLoadingId(null)
    }
  }

  const handleRemoveResume = async () => {
    try {
      const updated = { ...profile, resume_path: '', resume_text: '' }
      const res = await api.saveProfile(updated)
      setProfile(res || updated)
      showToast('✓ Resume removed. Future emails will not attach a PDF (live link dub.sh/pratyush is used).')
    } catch (err) {
      showToast(`Failed to remove resume: ${err.message}`)
    }
  }

  const handleDisconnectGoogle = async () => {
    try {
      await api.disconnectGoogle()
      setAuthStatus({ connected: false, email: '' })
      showToast('Google account disconnected')
    } catch (err) {
      showToast(`Disconnect failed: ${err.message}`)
    }
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
  const failedOutreach = outreachList.filter((o) => o.status === 'failed')
  const unrepliedSent = sent.filter((s) => !replies.some((r) => r.outreach_id === s.id))

  const handleMoveFailedToDrafts = async (ids) => {
    if (!ids || ids.length === 0) return
    setActionLoadingId('retry_failed')
    try {
      for (const id of ids) {
        await api.updateOutreach(id, { status: 'draft' })
      }
      await loadAllData()
      showToast(`✓ Moved ${ids.length} failed email${ids.length > 1 ? 's' : ''} back to Drafts for review & resend`)
    } catch (err) {
      showToast(`Error moving failed emails: ${err.message}`)
    } finally {
      setActionLoadingId(null)
    }
  }

  // Map of emails that have already received an email (sent or replied)
  const sentEmailsMap = {}
  for (const item of outreachList) {
    if ((item.status === 'sent' || item.status === 'replied') && item.contact_email) {
      const key = item.contact_email.toLowerCase().trim()
      if (!sentEmailsMap[key]) {
        sentEmailsMap[key] = item
      }
    }
  }

  // Sort discoveries latest on top (group scrape runs by 5-min window, then sort by posted_at age ascending)
  const sortedPosts = [...posts].sort((a, b) => {
    const bucketA = Math.floor(new Date(a.found_at).getTime() / (5 * 60 * 1000))
    const bucketB = Math.floor(new Date(b.found_at).getTime() / (5 * 60 * 1000))
    if (bucketB !== bucketA) return bucketB - bucketA
    const ageA = parsePostedAgeMinutes(a.extracted?.posted_at)
    const ageB = parsePostedAgeMinutes(b.extracted?.posted_at)
    if (ageA !== ageB) return ageA - ageB
    return new Date(b.found_at).getTime() - new Date(a.found_at).getTime()
  })

  // Posts whose email already received a mail previously (and don't currently have a new draft)
  const alreadyContactedPosts = sortedPosts.filter((p) => {
    if (dismissedAlreadyContactedIds.includes(p.id)) return false
    const email = p.extracted?.emails?.[0]?.toLowerCase()?.trim()
    if (!email || !sentEmailsMap[email]) return false
    const hasActiveDraft = drafts.some(
      (d) => d.post_id === p.id || d.contact_email?.toLowerCase()?.trim() === email
    )
    return !hasActiveDraft
  })

  // Sub-filter counts & filtered list for Found Posts tab
  const isPostReadyToDraft = (p) => {
    const email = p.extracted?.emails?.[0]?.toLowerCase()?.trim()
    if (!email) return false
    return !outreachList.some(
      (o) => o.post_id === p.id || o.contact_email?.toLowerCase()?.trim() === email
    )
  }

  const emailPostsCount = sortedPosts.filter((p) => p.extracted?.emails?.length > 0).length
  const phonePostsCount = sortedPosts.filter((p) => p.extracted?.phones?.length > 0).length
  const dmPostsCount = sortedPosts.filter((p) => p.extracted?.apply_via_dm).length
  const readyToDraftCount = sortedPosts.filter(isPostReadyToDraft).length

  const filteredPosts = sortedPosts.filter((p) => {
    // Hide non-hiring / #OpenToWork noise unless it has an email or phone
    if (p.status === 'skipped' && p.skip_reason === 'post is not a hiring or job announcement' && !(p.extracted?.emails?.length > 0)) {
      return false
    }
    if (postFilter === 'email' && !(p.extracted?.emails?.length > 0)) return false
    if (postFilter === 'phone' && !(p.extracted?.phones?.length > 0)) return false
    if (postFilter === 'dm' && !p.extracted?.apply_via_dm) return false
    if (onlyReadyToDraft && !isPostReadyToDraft(p)) return false
    return true
  })

  const isAllDraftsSelected = drafts.length > 0 && selectedDraftIds.length === drafts.length

  return (
    <main className="auto-shell">
      {/* ASCII Banner & Header */}
      <header className="app-header">
        <pre className="ascii-banner">{ASCII_LOGO}</pre>
        <div className="app-header-row">
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
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '0.5rem', flexWrap: 'wrap', gap: '0.5rem' }}>
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
              placeholder="e.g. hiring flutter, golang developer, full stack"
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
          <div className="run-panel-buttons">
            <button
              type="button"
              className="btn btn-ghost"
              onClick={handleOpenLoginBrowser}
              title="Open persistent Chrome window to log into LinkedIn"
              style={{ fontSize: '0.82rem', padding: '0 0.7rem', whiteSpace: 'nowrap' }}
            >
              🔑 LinkedIn Login
            </button>
            {isRunning ? (
              <button
                type="button"
                className="btn btn-kill"
                onClick={handleCancelRun}
                title="Immediately stop the active scrape"
                style={{ padding: '0 1rem', whiteSpace: 'nowrap' }}
              >
                ⏹ Cancel Scrape
              </button>
            ) : (
              <button
                type="button"
                className="btn"
                onClick={handleTriggerRun}
                disabled={!query.trim()}
                title={!query.trim() ? "Please enter a search query (e.g. hiring flutter)" : "Arm engine and start search pipeline"}
                style={{
                  padding: '0 1rem',
                  whiteSpace: 'nowrap',
                  opacity: !query.trim() ? 0.6 : 1,
                  cursor: !query.trim() ? 'not-allowed' : 'pointer'
                }}
              >
                Run Now & Arm
              </button>
            )}
          </div>
        </div>
      </section>

      {/* Active Scraping & Pipeline Indicator Banner */}
      {(isRunning || summary.state === 'RUNNING' || summary.state === 'SEARCHING') && (
        <section className="active-run-banner">
          <div className="active-run-spinner" />
          <div style={{ flex: 1 }}>
            <h3 style={{ margin: 0, color: '#93c5fd', fontSize: '1.02rem', display: 'flex', alignItems: 'center', gap: '0.5rem', flexWrap: 'wrap' }}>
              ⚡ Scraper & Pipeline Active: Querying "{query || scrapeProgress?.query}"
              {scrapeProgress?.active ? (
                <span className="pill pill-armed" style={{ fontSize: '0.78rem', padding: '0.15rem 0.55rem' }}>
                  {scrapeProgress.percent ?? 10}% • Scroll #{scrapeProgress.scrolls} • {scrapeProgress.extractedCount} posts found
                </span>
              ) : (
                <span className="pill pill-armed" style={{ fontSize: '0.78rem', padding: '0.15rem 0.55rem' }}>
                  90% • Processing & Drafting Emails...
                </span>
              )}
            </h3>
            <p className="muted" style={{ margin: '0.25rem 0 0.45rem 0', fontSize: '0.84rem' }}>
              {scrapeProgress?.active && scrapeProgress?.message
                ? scrapeProgress.message
                : 'Extracting contacts, verifying domains, and drafting tailored cold emails in the background...'}
            </p>
            <div style={{ width: '100%', height: '6px', background: '#1f2937', borderRadius: '999px', overflow: 'hidden' }}>
              <div
                style={{
                  width: `${scrapeProgress?.active ? (scrapeProgress.percent ?? 15) : 90}%`,
                  height: '100%',
                  background: 'linear-gradient(90deg, #3b82f6, #22c55e)',
                  transition: 'width 0.4s ease',
                }}
              />
            </div>
          </div>
          <button
            type="button"
            className="btn btn-kill"
            onClick={handleCancelRun}
            title="Immediately stop the active LinkedIn scrape and close browser"
            style={{ padding: '0.45rem 0.9rem', fontSize: '0.85rem' }}
          >
            ⏹ Cancel & Stop Scrape
          </button>
        </section>
      )}

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
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '0.8rem', flexWrap: 'wrap', gap: '0.5rem' }}>
            <div>
              <h2 style={{ margin: 0 }}>Discovered Opportunities ({sortedPosts.length})</h2>
              {sortedPosts.length > 0 && (
                <p className="muted" style={{ margin: '0.25rem 0 0 0', fontSize: '0.84rem' }}>
                  {emailPostsCount} with Email • {phonePostsCount} with Phone • {dmPostsCount} DM/Message Directly • {readyToDraftCount} Ready to Draft
                </p>
              )}
            </div>
            {sortedPosts.length > 0 && (
              <button
                type="button"
                className="btn btn-kill"
                onClick={handleClearAllPosts}
                disabled={actionLoadingId === 'clear_posts'}
                title="Delete all discovered opportunities from the database"
                style={{ padding: '0.35rem 0.85rem', fontSize: '0.82rem', whiteSpace: 'nowrap' }}
              >
                {actionLoadingId === 'clear_posts' ? '⏳ Clearing...' : `🗑 Clear All (${sortedPosts.length})`}
              </button>
            )}
          </div>

          {/* Interactive Filter Sections + Ready-to-Draft Toggle + Select All */}
          {sortedPosts.length > 0 && (
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '0.6rem', marginBottom: '1rem', paddingBottom: '0.8rem', borderBottom: '1px solid #27272a' }}>
              <div style={{ display: 'flex', gap: '0.45rem', flexWrap: 'wrap' }}>
                {[
                  { key: 'all', label: `All (${sortedPosts.length})` },
                  { key: 'email', label: `✉ With Email (${emailPostsCount})` },
                  { key: 'phone', label: `📞 With Phone (${phonePostsCount})` },
                  { key: 'dm', label: `💬 DM / Message (${dmPostsCount})` },
                ].map((f) => (
                  <button
                    key={f.key}
                    type="button"
                    className={postFilter === f.key ? 'btn' : 'btn btn-ghost'}
                    style={{ fontSize: '0.78rem', padding: '0.3rem 0.7rem' }}
                    onClick={() => setPostFilter(f.key)}
                  >
                    {f.label}
                  </button>
                ))}
                <button
                  type="button"
                  className={onlyReadyToDraft ? 'btn' : 'btn btn-ghost'}
                  style={{
                    fontSize: '0.78rem',
                    padding: '0.3rem 0.75rem',
                    borderColor: onlyReadyToDraft ? '#22c55e' : '#3f3f46',
                    background: onlyReadyToDraft ? '#16a34a' : 'transparent',
                    color: onlyReadyToDraft ? '#fff' : '#86efac',
                  }}
                  onClick={() => setOnlyReadyToDraft((prev) => !prev)}
                  title="Show only posts that have an email and don't have a draft yet"
                >
                  {onlyReadyToDraft ? `✓ Ready to Draft Only (${readyToDraftCount})` : `✉ Ready to Draft Only (${readyToDraftCount})`}
                </button>
              </div>

              {filteredPosts.some((p) => p.extracted?.emails?.length > 0) && (
                <label style={{ display: 'flex', alignItems: 'center', gap: '0.45rem', cursor: 'pointer', fontSize: '0.84rem', fontWeight: 600 }}>
                  <input
                    type="checkbox"
                    style={{ width: 'auto' }}
                    checked={
                      filteredPosts.some((p) => p.extracted?.emails?.length > 0) &&
                      filteredPosts
                        .filter((p) => p.extracted?.emails?.length > 0)
                        .every((p) => selectedPostIds.includes(p.id))
                    }
                    onChange={() => handleSelectAllPosts(filteredPosts)}
                  />
                  Select All ({filteredPosts.filter((p) => p.extracted?.emails?.length > 0).length})
                </label>
              )}
            </div>
          )}

          {/* Bulk Draft Toolbar for Selected Found Posts */}
          {selectedPostIds.length > 0 && (
            <div className="bulk-toolbar">
              <span style={{ fontWeight: 700, color: '#60a5fa' }}>
                ✓ {selectedPostIds.length} post{selectedPostIds.length > 1 ? 's' : ''} selected
              </span>
              <div style={{ display: 'flex', gap: '0.6rem', flexWrap: 'wrap' }}>
                <button
                  type="button"
                  className="btn"
                  onClick={handleBulkDraftPosts}
                  disabled={Boolean(actionLoadingId)}
                >
                  {actionLoadingId === 'bulk_draft_posts' ? '⏳ Batch Drafting...' : `✉ Draft Selected (${selectedPostIds.length})`}
                </button>
                <button
                  type="button"
                  className="btn btn-ghost"
                  onClick={() => setSelectedPostIds([])}
                  style={{ fontSize: '0.8rem' }}
                >
                  Deselect All
                </button>
              </div>
            </div>
          )}

          {filteredPosts.length === 0 ? (
            <p className="muted">
              {sortedPosts.length === 0
                ? 'No posts discovered yet. Trigger a run above or enable Autopilot.'
                : 'No posts match this filter. Try switching filter tabs or turning off "Ready to Draft Only".'}
            </p>
          ) : (
            filteredPosts.map((post, postIdx) => {
              const postEmail = post.extracted?.emails?.[0]?.toLowerCase()?.trim()
              const prevSent = postEmail ? sentEmailsMap[postEmail] : null
              const hasEmail = Boolean(postEmail)
              const isSelected = selectedPostIds.includes(post.id)
              return (
              <div
                key={post.id}
                className="item-card"
                style={{
                  borderColor: isSelected ? '#3b82f6' : 'var(--line-1)',
                  background: isSelected ? 'rgba(59, 130, 246, 0.04)' : '#0c0c0c',
                }}
              >
                <div className="item-card-header">
                  <div style={{ display: 'flex', alignItems: 'flex-start', gap: '0.75rem' }}>
                    {hasEmail && (
                      <input
                        type="checkbox"
                        checked={isSelected}
                        onChange={() => handleSelectPost(post.id)}
                        style={{ marginTop: '0.25rem', width: 'auto', cursor: 'pointer' }}
                        title="Select post for batch drafting"
                      />
                    )}
                    <div>
                      <h3 style={{ margin: 0, fontSize: '1.05rem' }}>
                        <span className="muted" style={{ fontSize: '0.82rem', marginRight: '0.45rem' }}>#{postIdx + 1}</span>
                        {post.role || 'Software Developer'}
                      </h3>
                      <p className="muted" style={{ fontSize: '0.88rem' }} title={`Scraped at ${new Date(post.found_at).toLocaleString()}`}>
                        <strong>{post.company || 'Company'}</strong> • posted by {post.author}
                        {post.extracted?.posted_at ? ` • 🕒 Posted ${post.extracted.posted_at} ago` : ''}
                      </p>
                    </div>
                  </div>
                  <div style={{ display: 'flex', gap: '0.4rem', flexWrap: 'wrap' }}>
                    {prevSent && (
                      <span
                        className="tag tag-remote"
                        style={{ background: 'rgba(245, 158, 11, 0.15)', color: '#fbbf24', borderColor: '#f59e0b' }}
                        title={`Already sent email to ${postEmail} on ${new Date(prevSent.sent_at || prevSent.updated_at).toLocaleString()}`}
                      >
                        ✓ Mail Already Sent ({new Date(prevSent.sent_at || prevSent.updated_at).toLocaleString([], { dateStyle: 'short', timeStyle: 'short' })})
                      </span>
                    )}
                    <span className={`tag tag-${post.paid_status || 'unknown'}`}>
                      {(post.paid_status || 'UNKNOWN').toUpperCase()}
                    </span>
                    <span className="tag tag-remote">{(post.work_mode || 'UNKNOWN').toUpperCase()}</span>
                    {post.extracted?.is_suspicious || (post.skip_reason && post.skip_reason.includes('SPAM')) ? (
                      <span className="tag tag-bounce">⚠️ FAKE / SPAM FLAGGED</span>
                    ) : prevSent ? null : post.status === 'skipped' ? (
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
                {post.text && (
                  <p
                    style={{
                      margin: '0.45rem 0 0.55rem 0',
                      fontSize: '0.84rem',
                      color: '#d4d4d8',
                      lineHeight: 1.4,
                      display: '-webkit-box',
                      WebkitLineClamp: 2,
                      WebkitBoxOrient: 'vertical',
                      overflow: 'hidden',
                      borderLeft: '2px solid #3f3f46',
                      paddingLeft: '0.6rem',
                    }}
                    title={post.text}
                  >
                    {post.text}
                  </p>
                )}

                <div style={{ display: 'flex', gap: '1rem', fontSize: '0.85rem', flexWrap: 'wrap', alignItems: 'center' }} className="muted">
                  <span>Location: <strong>{post.location || 'Not specified'}</strong></span>
                  {post.extracted?.stipend ? (
                    <span>Stipend: <strong>₹{post.extracted.stipend.toLocaleString()}/mo</strong></span>
                  ) : null}
                  {post.extracted?.experience_years != null ? (
                    <span>Exp: <strong>{post.extracted.experience_years} yrs</strong></span>
                  ) : null}
                  <a href={post.url} target="_blank" rel="noopener noreferrer" style={{ color: '#60a5fa', fontWeight: 600 }}>
                    View LinkedIn Post ↗
                  </a>
                  {post.extracted?.apply_via_dm && (
                    <a
                      href={post.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="tag tag-remote"
                      style={{ textDecoration: 'none', color: '#c4b5fd', borderColor: '#8b5cf6' }}
                      title="Post asks to message / DM directly on LinkedIn"
                    >
                      💬 DM / Message Directly ↗
                    </a>
                  )}
                </div>

                {post.extracted?.emails && post.extracted.emails.length > 0 && (
                  <div style={{ marginTop: '0.6rem', display: 'flex', gap: '0.8rem', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', borderTop: '1px solid #1f1f23', paddingTop: '0.6rem' }}>
                    <span style={{ fontSize: '0.82rem', color: '#93c5fd' }}>
                      ✉ Recipient Email: <strong>{post.extracted.emails[0]}</strong>
                    </span>
                    {(() => {
                      const existingDraft = outreachList.find(
                        (o) => o.post_id === post.id && o.status !== 'failed' && o.status !== 'skipped'
                      )
                      const existingByEmail = !existingDraft && postEmail
                        ? outreachList.find((o) => o.contact_email?.toLowerCase()?.trim() === postEmail && o.status !== 'failed' && o.status !== 'skipped')
                        : null
                      if (existingDraft) {
                        return (
                          <button
                            type="button"
                            className="btn btn-ghost"
                            style={{
                              fontSize: '0.78rem',
                              padding: '0.25rem 0.7rem',
                              color: '#86efac',
                              borderColor: '#22c55e',
                              background: 'rgba(34, 197, 94, 0.12)',
                              cursor: 'pointer',
                              fontWeight: 600,
                            }}
                            onClick={() => setActiveTab('drafts')}
                            title="Draft created! Click to view in Drafts & Approvals"
                          >
                            ✓ Draft Ready (in Approvals) ↗
                          </button>
                        )
                      }
                      return (
                        <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', flexWrap: 'wrap' }}>
                          {existingByEmail && (
                            <span className="muted" style={{ fontSize: '0.76rem', color: '#fbbf24' }}>
                              ℹ️ Not auto-drafted: duplicate email ({existingByEmail.status})
                            </span>
                          )}
                          <button
                            type="button"
                            className="btn"
                            style={{ fontSize: '0.78rem', padding: '0.25rem 0.65rem' }}
                            onClick={() => handleCreateDraftForPost(post)}
                            disabled={Boolean(actionLoadingId)}
                            title="Generate a personalized cold email draft tailored to this company"
                          >
                            {actionLoadingId === 'draft_' + post.id ? '⏳ Drafting with AI...' : '✉ Draft Cold Email'}
                          </button>
                        </div>
                      )
                    })()}
                  </div>
                )}

                {post.extracted?.phones && post.extracted.phones.length > 0 && (
                  <div style={{ marginTop: '0.5rem', display: 'flex', gap: '0.6rem', alignItems: 'center', flexWrap: 'wrap', borderTop: !post.extracted?.emails || post.extracted.emails.length === 0 ? '1px solid #1f1f23' : 'none', paddingTop: !post.extracted?.emails || post.extracted.emails.length === 0 ? '0.6rem' : '0' }}>
                    <span style={{ fontSize: '0.82rem', color: '#86efac' }}>📞 Contact Phone:</span>
                    {post.extracted.phones.map((phone, pIdx) => {
                      const cleanDigits = phone.replace(/\D/g, '')
                      const waNum = cleanDigits.length === 10 ? `91${cleanDigits}` : cleanDigits
                      return (
                        <div key={pIdx} style={{ display: 'inline-flex', gap: '0.4rem', alignItems: 'center' }}>
                          <a
                            href={`tel:${phone}`}
                            className="tag tag-remote"
                            style={{ textDecoration: 'none', color: '#93c5fd' }}
                            title="Call phone number"
                          >
                            📞 {phone}
                          </a>
                          <a
                            href={`https://wa.me/${waNum}?text=${encodeURIComponent(`Hi, I saw your post on LinkedIn regarding ${post.role || 'the open position'} at ${post.company || 'your company'}.`)}`}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="tag tag-paid"
                            style={{ textDecoration: 'none', background: '#059669', color: '#fff', fontSize: '0.75rem', padding: '0.15rem 0.5rem' }}
                            title="Chat on WhatsApp"
                          >
                            💬 WhatsApp
                          </a>
                        </div>
                      )
                    })}
                  </div>
                )}
              </div>
              )
            })
          )}
        </section>
      )}

      {/* Tab Content 2: Drafts & Approvals (With Multiselect) */}
      {activeTab === 'drafts' && (
        <section className="card">
          {!authStatus.connected && (
            <div style={{
              background: 'rgba(239, 68, 68, 0.12)',
              border: '1px solid rgba(239, 68, 68, 0.45)',
              borderRadius: '8px',
              padding: '0.85rem 1.1rem',
              marginBottom: '1.2rem',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              gap: '1rem',
              flexWrap: 'wrap'
            }}>
              <div>
                <span style={{ fontWeight: 700, color: '#f87171' }}>⚠️ Gmail Not Connected:</span>
                <span style={{ color: '#fca5a5', marginLeft: '0.5rem', fontSize: '0.88rem' }}>
                  To dispatch cold emails directly to recruiter inboxes, connect your Google account.
                </span>
              </div>
              <a
                href="http://localhost:8080/auth/google"
                className="btn"
                style={{
                  background: '#ef4444',
                  color: '#fff',
                  textDecoration: 'none',
                  padding: '0.35rem 0.85rem',
                  fontSize: '0.82rem',
                  fontWeight: 600,
                  borderRadius: '4px'
                }}
              >
                Connect Gmail Account ↗
              </a>
            </div>
          )}

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
                <button
                  type="button"
                  className="btn"
                  onClick={handleBulkApprove}
                  disabled={Boolean(actionLoadingId)}
                >
                  {actionLoadingId === 'bulk_approve' ? '⏳ Scheduling...' : `✓ Approve & Schedule (${selectedDraftIds.length})`}
                </button>
                <button
                  type="button"
                  className="btn-send-now"
                  onClick={handleBulkSendNow}
                  disabled={Boolean(actionLoadingId)}
                >
                  {actionLoadingId === 'bulk_send' ? '⏳ Sending...' : `✈ Send Now (${selectedDraftIds.length})`}
                </button>
                <button
                  type="button"
                  className="btn btn-ghost"
                  onClick={handleBulkSkip}
                  disabled={Boolean(actionLoadingId)}
                >
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
                        <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', flexWrap: 'wrap' }}>
                          <h3 style={{ margin: 0 }}>{d.subject}</h3>
                          <span className={`pill ${d.kind === 'initial' ? 'pill-idle' : 'pill-armed'}`} style={{ fontSize: '0.68rem', padding: '0.1rem 0.4rem' }}>
                            {d.kind ? d.kind.toUpperCase() : 'INITIAL'}
                          </span>
                          {(() => {
                            const prev = d.contact_email ? sentEmailsMap[d.contact_email.toLowerCase().trim()] : null
                            if (!prev) return null
                            return (
                              <span
                                className="tag tag-remote"
                                style={{ background: 'rgba(245, 158, 11, 0.16)', color: '#fbbf24', borderColor: '#f59e0b', fontSize: '0.72rem' }}
                                title={`Previously sent "${prev.subject}" on ${new Date(prev.sent_at || prev.updated_at).toLocaleString()}`}
                              >
                                ⚠️ Already received mail on {new Date(prev.sent_at || prev.updated_at).toLocaleString([], { dateStyle: 'short', timeStyle: 'short' })} — Resend?
                              </span>
                            )
                          })()}
                        </div>
                        <p className="muted" style={{ fontSize: '0.85rem', marginTop: '0.2rem' }}>
                          To: <strong>{d.contact_email}</strong> • {d.company} ({d.role})
                        </p>
                      </div>
                    </div>

                    <div className="row" style={{ gap: '0.4rem' }}>
                      <button
                        type="button"
                        className="btn"
                        onClick={() => handleApproveSingle(d.id)}
                        disabled={Boolean(actionLoadingId)}
                        title="Add to automated schedule queue"
                      >
                        {actionLoadingId === 'approve_' + d.id ? '⏳ Approving...' : '✓ Approve'}
                      </button>
                      <button
                        type="button"
                        className="btn-send-now"
                        onClick={() => handleSendSingleNow(d.id)}
                        disabled={Boolean(actionLoadingId)}
                        title="Send email immediately via Gmail"
                      >
                        {actionLoadingId === 'send_' + d.id ? '⏳ Sending...' : '✈ Send Now'}
                      </button>
                      <button
                        type="button"
                        className="btn btn-ghost"
                        onClick={() => (isEditing ? handleSaveEditDraft(d.id) : handleStartEditDraft(d))}
                        disabled={Boolean(actionLoadingId)}
                      >
                        {isEditing ? 'Save' : '✎ Edit'}
                      </button>
                      <button
                        type="button"
                        className="btn btn-ghost"
                        onClick={() => handleSkipSingle(d.id)}
                        disabled={Boolean(actionLoadingId)}
                      >
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

          {/* Dedicated Section: Failed Dispatch / Needs Retry */}
          {failedOutreach.length > 0 && (
            <div style={{ marginTop: '2rem', borderTop: '1px solid #27272a', paddingTop: '1.25rem' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '0.6rem', marginBottom: '0.8rem' }}>
                <div>
                  <h3 style={{ margin: 0, color: '#f87171', fontSize: '1.02rem' }}>
                    ⚠️ Failed Dispatch / Needs Retry ({failedOutreach.length})
                  </h3>
                  <p className="muted" style={{ margin: '0.2rem 0 0 0', fontSize: '0.84rem' }}>
                    These emails failed to send via Gmail. Move them back to Drafts or retry sending directly.
                  </p>
                </div>
                <button
                  type="button"
                  className="btn"
                  style={{ fontSize: '0.8rem', padding: '0.35rem 0.85rem' }}
                  onClick={() => handleMoveFailedToDrafts(failedOutreach.map((f) => f.id))}
                  disabled={Boolean(actionLoadingId)}
                >
                  {actionLoadingId === 'retry_failed' ? '⏳ Moving...' : `↻ Move All to Drafts (${failedOutreach.length})`}
                </button>
              </div>
              {failedOutreach.map((f) => (
                <div
                  key={f.id}
                  className="item-card"
                  style={{ borderColor: 'rgba(239, 68, 68, 0.45)', background: 'rgba(239, 68, 68, 0.04)' }}
                >
                  <div className="item-card-header">
                    <div>
                      <h4 style={{ margin: 0, fontSize: '0.96rem' }}>{f.subject}</h4>
                      <p className="muted" style={{ fontSize: '0.82rem', margin: '0.2rem 0 0 0' }}>
                        To: <strong>{f.contact_email}</strong> • {f.company || f.role}
                      </p>
                      {f.error && (
                        <p style={{ color: '#fca5a5', fontSize: '0.78rem', margin: '0.3rem 0 0 0' }}>
                          Error: {f.error}
                        </p>
                      )}
                    </div>
                    <div className="row" style={{ gap: '0.4rem' }}>
                      <button
                        type="button"
                        className="btn btn-ghost"
                        style={{ fontSize: '0.78rem', padding: '0.3rem 0.7rem' }}
                        onClick={() => handleMoveFailedToDrafts([f.id])}
                        disabled={Boolean(actionLoadingId)}
                      >
                        ↻ Move to Draft
                      </button>
                      <button
                        type="button"
                        className="btn-send-now"
                        style={{ fontSize: '0.78rem', padding: '0.3rem 0.7rem' }}
                        onClick={() => handleSendSingleNow(f.id)}
                        disabled={Boolean(actionLoadingId)}
                      >
                        {actionLoadingId === 'send_' + f.id ? '⏳ Sending...' : '✈ Retry Send Now'}
                      </button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}

          {/* Dedicated Section: Already Received Mail Previously */}
          {alreadyContactedPosts.length > 0 && (
            <div style={{ marginTop: '2rem', borderTop: '1px solid #27272a', paddingTop: '1.25rem' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '0.6rem', marginBottom: '0.4rem' }}>
                <h3 style={{ margin: 0, color: '#fbbf24', fontSize: '1.02rem' }}>
                  📬 Already Received Mail Previously ({alreadyContactedPosts.length})
                </h3>
                <button
                  type="button"
                  className="btn btn-ghost"
                  style={{ fontSize: '0.78rem', padding: '0.3rem 0.75rem', borderColor: '#f59e0b', color: '#fbbf24' }}
                  onClick={() => {
                    setDismissedAlreadyContactedIds((prev) => [
                      ...prev,
                      ...alreadyContactedPosts.map((p) => p.id),
                    ])
                    showToast('✓ Cleared "Already Received Mail Previously" list')
                  }}
                  title="Clear this list from Drafts & Approvals (keeps your Sent history intact)"
                >
                  🗑 Clear List ({alreadyContactedPosts.length})
                </button>
              </div>
              <p className="muted" style={{ margin: '0.25rem 0 0.9rem 0', fontSize: '0.84rem' }}>
                These contacts from your current discoveries have already received an email from you. They were skipped from auto-drafting to avoid spamming — click "Draft Resend Anyway" if you still want to send again.
              </p>
              {alreadyContactedPosts.map((p) => {
                const email = p.extracted?.emails?.[0]?.toLowerCase()?.trim()
                const prev = sentEmailsMap[email]
                return (
                  <div
                    key={p.id}
                    className="item-card"
                    style={{ borderColor: 'rgba(245, 158, 11, 0.35)', background: 'rgba(245, 158, 11, 0.04)' }}
                  >
                    <div className="item-card-header">
                      <div>
                        <h4 style={{ margin: 0, fontSize: '0.96rem' }}>
                          {p.role || 'Software Developer'} • {p.company || p.author}
                        </h4>
                        <p className="muted" style={{ fontSize: '0.82rem', margin: '0.2rem 0 0 0' }}>
                          ✉ <strong>{email}</strong> • Previously sent: <strong>{new Date(prev.sent_at || prev.updated_at).toLocaleString([], { dateStyle: 'short', timeStyle: 'short' })}</strong>
                          {prev.subject ? ` ("${prev.subject}")` : ''}
                        </p>
                      </div>
                      <button
                        type="button"
                        className="btn btn-ghost"
                        style={{ borderColor: '#f59e0b', color: '#fbbf24', fontSize: '0.78rem', padding: '0.3rem 0.7rem', whiteSpace: 'nowrap' }}
                        onClick={() => handleCreateDraftForPost(p, true)}
                        disabled={Boolean(actionLoadingId)}
                      >
                        {actionLoadingId === 'draft_' + p.id ? '⏳ Creating...' : '↻ Draft Resend Anyway'}
                      </button>
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </section>
      )}

      {/* Tab Content 3: Queue */}
      {activeTab === 'queue' && (
        <section className="card">
          {!authStatus.connected && (
            <div style={{
              background: 'rgba(239, 68, 68, 0.12)',
              border: '1px solid rgba(239, 68, 68, 0.45)',
              borderRadius: '8px',
              padding: '0.85rem 1.1rem',
              marginBottom: '1.2rem',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              gap: '1rem',
              flexWrap: 'wrap'
            }}>
              <div>
                <span style={{ fontWeight: 700, color: '#f87171' }}>⚠️ Gmail Not Connected:</span>
                <span style={{ color: '#fca5a5', marginLeft: '0.5rem', fontSize: '0.88rem' }}>
                  Emails waiting in queue will not send until your Google account is connected.
                </span>
              </div>
              <a
                href="http://localhost:8080/auth/google"
                className="btn"
                style={{
                  background: '#ef4444',
                  color: '#fff',
                  textDecoration: 'none',
                  padding: '0.35rem 0.85rem',
                  fontSize: '0.82rem',
                  fontWeight: 600,
                  borderRadius: '4px'
                }}
              >
                Connect Gmail Account ↗
              </a>
            </div>
          )}

          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '0.8rem' }}>
            <div>
              <h2 style={{ margin: 0 }}>Outreach Queue ({queue.length})</h2>
              <p className="muted" style={{ margin: '0.2rem 0 0 0', fontSize: '0.88rem' }}>
                Approved emails scheduled to send within your daily window respecting time gaps.
              </p>
            </div>
            {queue.length > 0 && (
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.85rem', flexWrap: 'wrap' }}>
                <label style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', cursor: 'pointer', fontWeight: 600 }}>
                  <input
                    type="checkbox"
                    style={{ width: 'auto' }}
                    checked={queue.length > 0 && selectedQueueIds.length === queue.length}
                    onChange={handleSelectAllQueue}
                  />
                  Select All ({queue.length})
                </label>
                <button
                  type="button"
                  className="btn btn-kill"
                  onClick={handleEmptyQueue}
                  disabled={Boolean(actionLoadingId)}
                  style={{ padding: '0.35rem 0.85rem', fontSize: '0.82rem', whiteSpace: 'nowrap' }}
                  title="Cancel and remove all emails from the queue"
                >
                  {actionLoadingId === 'empty_queue' ? '⏳ Emptying...' : `🗑 Empty Queue (${queue.length})`}
                </button>
              </div>
            )}
          </div>

          {selectedQueueIds.length > 0 && (
            <div className="bulk-toolbar">
              <span style={{ fontWeight: 700, color: '#60a5fa' }}>
                ✓ {selectedQueueIds.length} queued email{selectedQueueIds.length > 1 ? 's' : ''} selected
              </span>
              <div style={{ display: 'flex', gap: '0.6rem', flexWrap: 'wrap' }}>
                <button
                  type="button"
                  className="btn-send-now"
                  onClick={handleBulkSendQueue}
                  disabled={Boolean(actionLoadingId)}
                >
                  {actionLoadingId === 'bulk_send_queue' ? '⏳ Sending...' : `✈ Send Selected Now (${selectedQueueIds.length})`}
                </button>
                <button
                  type="button"
                  className="btn btn-ghost"
                  onClick={handleBulkCancelQueue}
                  disabled={Boolean(actionLoadingId)}
                >
                  {actionLoadingId === 'bulk_cancel_queue' ? '⏳ Cancelling...' : `✕ Cancel Selected (${selectedQueueIds.length})`}
                </button>
                <button
                  type="button"
                  className="btn btn-ghost"
                  onClick={() => setSelectedQueueIds([])}
                  style={{ fontSize: '0.8rem' }}
                >
                  Deselect All
                </button>
              </div>
            </div>
          )}

          {queue.length === 0 ? (
            <p className="muted" style={{ marginTop: '1rem' }}>Queue is currently empty.</p>
          ) : (
            queue.map((q) => {
              const isSelected = selectedQueueIds.includes(q.id)
              return (
              <div
                key={q.id}
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
                      onChange={() => handleSelectQueue(q.id)}
                      style={{ marginTop: '0.3rem', width: 'auto', cursor: 'pointer' }}
                    />
                    <div>
                      <h3 style={{ margin: 0 }}>{q.subject}</h3>
                      <p className="muted" style={{ fontSize: '0.85rem', marginTop: '0.2rem' }}>
                        To: <strong>{q.contact_email}</strong> • {q.company}
                      </p>
                    </div>
                  </div>
                  <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center' }}>
                    {q.scheduled_at ? (
                      <span className="tag tag-remote" title={`Scheduled for ${new Date(q.scheduled_at).toLocaleString()}`}>
                        ⏰ Scheduled: {new Date(q.scheduled_at).toLocaleString([], { dateStyle: 'short', timeStyle: 'short' })}
                        {new Date(q.scheduled_at).getTime() <= Date.now() ? ' (Due Now)' : ` (in ${Math.max(1, Math.round((new Date(q.scheduled_at).getTime() - Date.now()) / 60000))}m)`}
                      </span>
                    ) : (
                      <span className="tag tag-remote">⏳ Pending dispatch gap</span>
                    )}
                    <button
                      type="button"
                      className="btn-send-now"
                      onClick={() => handleSendSingleNow(q.id)}
                      disabled={Boolean(actionLoadingId)}
                      title="Send email immediately via Gmail"
                    >
                      {actionLoadingId === 'send_' + q.id ? '⏳ Sending...' : '✈ Send Now'}
                    </button>
                    <button
                      type="button"
                      className="btn btn-ghost"
                      onClick={() => handleCancelQueuedSingle(q.id)}
                      disabled={Boolean(actionLoadingId)}
                      title="Cancel and remove from queue"
                      style={{ fontSize: '0.8rem', padding: '0.35rem 0.65rem' }}
                    >
                      {actionLoadingId === 'cancel_queue_' + q.id ? '⏳...' : '✕ Cancel'}
                    </button>
                  </div>
                </div>
                <div className="email-preview-box">{q.body}</div>
              </div>
              )
            })
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
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '0.6rem', flexWrap: 'wrap', gap: '0.6rem' }}>
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
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '0.8rem' }}>
            <div>
              <h2 style={{ margin: 0 }}>Pipeline Run Logs ({runs.length})</h2>
              <p className="muted" style={{ margin: '0.2rem 0 0 0', fontSize: '0.88rem' }}>
                Click on any run row below to view detailed execution logs, error diagnostics, and resolution steps.
              </p>
            </div>
          </div>

          {runs.length === 0 ? (
            <p className="muted" style={{ marginTop: '1rem' }}>No execution runs recorded yet.</p>
          ) : (
            <div className="runs-table-wrap">
              <table className="runs-table">
                <thead>
                  <tr>
                    <th>Date / Time</th>
                    <th>Trigger</th>
                    <th>Query</th>
                    <th>Window</th>
                    <th>Status</th>
                    <th>Pipeline Metrics</th>
                    <th>Action</th>
                  </tr>
                </thead>
                <tbody>
                  {runs.map((r) => {
                    const stats = r.stats || {}
                    const runDate = r.created_at || r.started_at
                    const isSelected = selectedRun?.id === r.id
                    const isFailed = r.status === 'failed'
                    const issueText = stats.issue || stats.error

                    return (
                      <tr
                        key={r.id}
                        className={`run-row-clickable ${isSelected ? 'run-row-selected' : ''}`}
                        onClick={() => setSelectedRun(isSelected ? null : r)}
                        title="Click to view run logs & diagnostics"
                      >
                        <td>{runDate ? new Date(runDate).toLocaleString() : 'N/A'}</td>
                        <td>
                          <span className={`pill ${r.trigger === 'auto' ? 'pill-armed' : 'pill-idle'}`} style={{ fontSize: '0.7rem' }}>
                            {(r.trigger || 'manual').toUpperCase()}
                          </span>
                        </td>
                        <td><code>{r.query || 'Default'}</code></td>
                        <td>{r.time_filter || '24h'} / {r.sort_by || 'latest'}</td>
                        <td>
                          <span className={`tag ${isFailed ? 'tag-unpaid' : 'tag-paid'}`}>
                            {(r.status || 'completed').toUpperCase()}
                          </span>
                          {isFailed && issueText ? (
                            <div style={{ color: '#fca5a5', fontSize: '0.72rem', marginTop: '0.2rem', maxWidth: '200px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                              {issueText}
                            </div>
                          ) : null}
                        </td>
                        <td>
                          <span className="stat-pill stat-found">Found: {stats.found ?? 0}</span>
                          <span className="stat-pill stat-passed">Filtered: {stats.filtered ?? 0}</span>
                          <span className="stat-pill stat-passed">Verified: {stats.verified ?? 0}</span>
                          <span className="stat-pill stat-skipped">Skipped: {stats.skipped ?? 0}</span>
                          <span className="stat-pill stat-drafted">Drafted: {stats.drafted ?? 0}</span>
                        </td>
                        <td>
                          <button
                            type="button"
                            className="btn btn-ghost"
                            style={{ fontSize: '0.75rem', padding: '0.2rem 0.5rem' }}
                            onClick={(e) => {
                              e.stopPropagation()
                              setSelectedRun(isSelected ? null : r)
                            }}
                          >
                            {isSelected ? '▲ Hide Logs' : '▼ View Logs'}
                          </button>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}

          {/* Detailed Selected Run Inspector */}
          {selectedRun && (
            <div className="run-details-card">
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '0.8rem', borderBottom: '1px solid #27272a', paddingBottom: '0.6rem', flexWrap: 'wrap', gap: '0.8rem' }}>
                <div>
                  <h3 style={{ margin: 0, fontSize: '1rem', wordBreak: 'break-all' }}>
                    Run Diagnostics: <code>{selectedRun.id}</code>
                  </h3>
                  <p className="muted" style={{ margin: '0.2rem 0 0 0', fontSize: '0.8rem', wordBreak: 'break-word' }}>
                    Trigger: <strong>{(selectedRun.trigger || 'manual').toUpperCase()}</strong> • Search Query: <code>"{selectedRun.query}"</code> • Filter: <strong>{selectedRun.time_filter || '24h'}</strong> / <strong>{selectedRun.sort_by || 'latest'}</strong>
                  </p>
                </div>
                <button
                  type="button"
                  className="btn btn-ghost"
                  style={{ fontSize: '0.8rem' }}
                  onClick={() => setSelectedRun(null)}
                >
                  ✕ Close Inspector
                </button>
              </div>

              {/* Error & Resolution Alert if Failed */}
              {selectedRun.status === 'failed' && (
                <div style={{ background: '#1c1917', borderLeft: '4px solid #ef4444', padding: '0.8rem 1rem', borderRadius: '6px', marginBottom: '1rem' }}>
                  <h4 style={{ margin: '0 0 0.3rem 0', color: '#f87171', fontSize: '0.92rem' }}>
                    🚨 Run Execution Issue
                  </h4>
                  <p style={{ margin: '0 0 0.5rem 0', fontSize: '0.88rem', color: '#fecaca', fontWeight: 600 }}>
                    {selectedRun.stats?.issue || selectedRun.stats?.error || 'Scrape execution failed without error text'}
                  </p>

                  {/* Specific Actionable Guidance for Playwright Worker Down */}
                  {(selectedRun.stats?.issue?.includes('playwright worker') ||
                    selectedRun.stats?.error?.includes('playwright worker') ||
                    selectedRun.stats?.error?.includes('connectex') ||
                    selectedRun.stats?.error?.includes('refused it')) && (
                    <div style={{ background: '#27272a', padding: '0.8rem 1rem', borderRadius: '6px', marginTop: '0.6rem', border: '1px solid #3f3f46' }}>
                      <p style={{ margin: '0 0 0.4rem 0', fontWeight: 700, color: '#fbbf24', fontSize: '0.85rem' }}>
                        ⚙️ How to fix: Start the Playwright Worker
                      </p>
                      <p style={{ margin: '0 0 0.4rem 0', fontSize: '0.83rem', color: '#e4e4e7' }}>
                        The backend Go server is running, but the Node.js Playwright worker service on port 3001 is not running.
                      </p>
                      <ol style={{ margin: 0, paddingLeft: '1.2rem', fontSize: '0.83rem', color: '#e4e4e7', lineHeight: '1.6' }}>
                        <li>Open a terminal in the project directory.</li>
                        <li>Run: <code style={{ color: '#38bdf8' }}>cd worker; npm start</code></li>
                        <li>Once you see <code style={{ color: '#86efac' }}>[EmailAuto Worker] Running on http://localhost:3001</code>, click <strong>"Run Now & Arm"</strong>.</li>
                      </ol>
                    </div>
                  )}

                  {/* Specific Actionable Guidance for LinkedIn Login Wall */}
                  {(selectedRun.stats?.issue?.includes('Login Required') ||
                    selectedRun.stats?.error?.includes('login') ||
                    selectedRun.stats?.error?.includes('CAPTCHA') ||
                    selectedRun.stats?.error?.includes('uas/login')) && (
                    <div style={{ background: '#27272a', padding: '0.8rem 1rem', borderRadius: '6px', marginTop: '0.6rem', border: '1px solid #3f3f46' }}>
                      <p style={{ margin: '0 0 0.4rem 0', fontWeight: 700, color: '#fbbf24', fontSize: '0.85rem' }}>
                        🔑 How to fix this in 10 seconds:
                      </p>
                      <ol style={{ margin: 0, paddingLeft: '1.2rem', fontSize: '0.83rem', color: '#e4e4e7', lineHeight: '1.5' }}>
                        <li>The Playwright worker launched a headed Chrome browser window on your desktop.</li>
                        <li>In that Chrome window, open <strong style={{ color: '#60a5fa' }}>linkedin.com</strong> and log into your LinkedIn account.</li>
                        <li>Complete 2FA verification if LinkedIn prompts for it.</li>
                        <li>Once you see your LinkedIn feed in that Chrome window, return to this dashboard and click <strong style={{ color: '#34d399' }}>"Run Now & Arm"</strong>.</li>
                        <li>Your session cookies will be permanently preserved in <code>worker/user_data/</code> so you will not have to log in again.</li>
                      </ol>
                      <button
                        type="button"
                        className="btn"
                        onClick={handleOpenLoginBrowser}
                        style={{ marginTop: '0.8rem', fontSize: '0.85rem' }}
                      >
                        🔑 Open LinkedIn in Chrome to Log In
                      </button>
                    </div>
                  )}
                </div>
              )}

              {/* Stats Breakdown Grid */}
              <div className="grid-2" style={{ gap: '0.6rem', marginBottom: '0.8rem' }}>
                <div style={{ background: '#141417', padding: '0.6rem 0.8rem', borderRadius: '6px', border: '1px solid #1f1f23' }}>
                  <span className="muted" style={{ fontSize: '0.78rem' }}>Posts Discovered:</span>
                  <div style={{ fontSize: '1.1rem', fontWeight: 700, color: '#93c5fd' }}>{selectedRun.stats?.found ?? 0}</div>
                </div>
                <div style={{ background: '#141417', padding: '0.6rem 0.8rem', borderRadius: '6px', border: '1px solid #1f1f23' }}>
                  <span className="muted" style={{ fontSize: '0.78rem' }}>Passed Filters:</span>
                  <div style={{ fontSize: '1.1rem', fontWeight: 700, color: '#86efac' }}>{selectedRun.stats?.filtered ?? 0}</div>
                </div>
                <div style={{ background: '#141417', padding: '0.6rem 0.8rem', borderRadius: '6px', border: '1px solid #1f1f23' }}>
                  <span className="muted" style={{ fontSize: '0.78rem' }}>Domain Verified:</span>
                  <div style={{ fontSize: '1.1rem', fontWeight: 700, color: '#a7f3d0' }}>{selectedRun.stats?.verified ?? 0}</div>
                </div>
                <div style={{ background: '#141417', padding: '0.6rem 0.8rem', borderRadius: '6px', border: '1px solid #1f1f23' }}>
                  <span className="muted" style={{ fontSize: '0.78rem' }}>Drafts Generated:</span>
                  <div style={{ fontSize: '1.1rem', fontWeight: 700, color: '#c7d2fe' }}>{selectedRun.stats?.drafted ?? 0}</div>
                </div>
              </div>

              {/* Raw JSON Debugging */}
              <details style={{ marginTop: '0.5rem' }}>
                <summary style={{ cursor: 'pointer', color: '#60a5fa', fontSize: '0.8rem' }}>
                  🔍 Show Raw Run Metadata & Logs (JSON)
                </summary>
                <pre className="json-debug-pre">
                  {JSON.stringify(selectedRun, null, 2)}
                </pre>
              </details>
            </div>
          )}
        </section>
      )}

      {/* Tab Content 7: Settings & Profile */}
      {activeTab === 'settings' && settings && profile && (
        <form onSubmit={handleSaveSettings} style={{ display: 'grid', gap: '1.2rem' }}>
          {/* Google Account Connection Card */}
          <section className="card">
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '1rem' }}>
              <div>
                <h2 style={{ margin: 0 }}>Google Account & Gmail Dispatch</h2>
                <p className="muted" style={{ margin: '0.3rem 0 0 0', fontSize: '0.88rem' }}>
                  Required to send emails from your personal/work Gmail account and monitor replies.
                </p>
              </div>
              <div>
                {authStatus.connected ? (
                  <div style={{ display: 'flex', gap: '0.8rem', alignItems: 'center' }}>
                    <span className="tag tag-paid" style={{ padding: '0.4rem 0.8rem', fontSize: '0.85rem' }}>
                      ✓ Connected: {authStatus.email || 'Authorized'}
                    </span>
                    <button
                      type="button"
                      className="btn btn-ghost"
                      onClick={handleDisconnectGoogle}
                      style={{ fontSize: '0.82rem', padding: '0.35rem 0.75rem', borderColor: '#ef4444', color: '#f87171' }}
                    >
                      Disconnect
                    </button>
                  </div>
                ) : (
                  <a
                    href="http://localhost:8080/auth/google"
                    className="btn"
                    style={{
                      background: '#3b82f6',
                      color: '#fff',
                      textDecoration: 'none',
                      padding: '0.45rem 1rem',
                      fontSize: '0.88rem',
                      fontWeight: 600,
                      borderRadius: '4px',
                      display: 'inline-block'
                    }}
                  >
                    Connect Gmail Account ↗
                  </a>
                )}
              </div>
            </div>
          </section>

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
                <div style={{ marginTop: '0.8rem', display: 'flex', alignItems: 'center', justifyContent: 'space-between', background: '#18181b', border: '1px solid #27272a', padding: '0.65rem 0.9rem', borderRadius: '6px', flexWrap: 'wrap', gap: '0.6rem' }}>
                  <div>
                    <span style={{ color: '#10b981', fontWeight: 600, fontSize: '0.86rem' }}>📎 PDF Attached: </span>
                    <code style={{ fontSize: '0.83rem', color: '#93c5fd' }}>{profile.resume_path}</code>
                    <span className="muted" style={{ fontSize: '0.8rem', marginLeft: '0.5rem' }}>({profile.resume_text?.length || 0} chars extracted)</span>
                  </div>
                  <button
                    type="button"
                    className="btn btn-ghost"
                    onClick={handleRemoveResume}
                    style={{ color: '#f87171', borderColor: '#ef4444', fontSize: '0.8rem', padding: '0.25rem 0.65rem' }}
                    title="Remove PDF resume attachment from outgoing emails"
                  >
                    ✕ Remove Resume Attachment
                  </button>
                </div>
              ) : (
                <p className="muted" style={{ fontSize: '0.82rem', marginTop: '0.5rem', color: '#a1a1aa' }}>
                  ℹ️ No PDF file currently attached. Your emails will include your live portfolio/resume link (<code>dub.sh/pratyush</code>) cleanly in the email signature without attaching a physical file.
                </p>
              )}
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
