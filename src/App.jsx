import { useMemo, useState } from 'react'
import './App.css'
import {
  GOOGLE_SCOPES,
  createGoogleTokenClient,
  fetchGoogleAccountEmail,
  revokeGoogleToken,
} from './lib/googleAuth'
import { sendGmailMessage } from './lib/gmailService'
import {
  createTemplate,
  deleteTemplate,
  loadTemplates,
  updateTemplate,
} from './lib/templateStorage'

function App() {
  const [token, setToken] = useState('')
  const [accountEmail, setAccountEmail] = useState('')
  const [authLoading, setAuthLoading] = useState(false)
  const [authError, setAuthError] = useState('')

  const [recipientsText, setRecipientsText] = useState('')
  const [subject, setSubject] = useState('')
  const [body, setBody] = useState('')
  const [attachments, setAttachments] = useState([])

  const [templates, setTemplates] = useState(loadTemplates)
  const [templateName, setTemplateName] = useState('')
  const [selectedTemplateId, setSelectedTemplateId] = useState('')

  const [isSending, setIsSending] = useState(false)
  const [sendLogs, setSendLogs] = useState([])

  const clientId = import.meta.env.VITE_GOOGLE_CLIENT_ID

  const recipients = useMemo(() => {
    const parsed = recipientsText
      .split(/[\n,;]+/)
      .map((entry) => entry.trim())
      .filter(Boolean)

    return [...new Set(parsed)]
  }, [recipientsText])

  const connectGoogle = async () => {
    setAuthError('')

    if (!clientId) {
      setAuthError('Missing VITE_GOOGLE_CLIENT_ID in .env file.')
      return
    }

    setAuthLoading(true)

    try {
      const tokenClient = await createGoogleTokenClient({
        clientId,
        scope: GOOGLE_SCOPES,
      })

      tokenClient.requestAccessToken({ prompt: 'consent' })

      const tokenResponse = await tokenClient.waitForToken()

      if (!tokenResponse.access_token) {
        throw new Error('Google did not return an access token.')
      }

      setToken(tokenResponse.access_token)

      const email = await fetchGoogleAccountEmail(tokenResponse.access_token)
      setAccountEmail(email)
    } catch (error) {
      setAuthError(error.message || 'Google sign-in failed.')
    } finally {
      setAuthLoading(false)
    }
  }

  const disconnectGoogle = () => {
    if (token) {
      revokeGoogleToken(token)
    }

    setToken('')
    setAccountEmail('')
  }

  const onAttachmentsChange = (event) => {
    const files = Array.from(event.target.files || [])
    setAttachments(files)
  }

  const saveCurrentTemplate = () => {
    if (!templateName.trim()) {
      return
    }

    const nextTemplate = {
      name: templateName,
      subject,
      body,
    }

    if (selectedTemplateId) {
      const updated = updateTemplate(templates, selectedTemplateId, nextTemplate)
      setTemplates(updated)
      return
    }

    const created = createTemplate(templates, nextTemplate)
    setTemplates(created)
  }

  const applyTemplate = (templateId) => {
    setSelectedTemplateId(templateId)

    const template = templates.find((entry) => entry.id === templateId)
    if (!template) {
      return
    }

    setTemplateName(template.name)
    setSubject(template.subject)
    setBody(template.body)
  }

  const removeSelectedTemplate = () => {
    if (!selectedTemplateId) {
      return
    }

    const nextTemplates = deleteTemplate(templates, selectedTemplateId)
    setTemplates(nextTemplates)
    setSelectedTemplateId('')
  }

  const sendToAllRecipients = async () => {
    if (!token) {
      setAuthError('Please connect your Google account first.')
      return
    }

    if (!recipients.length) {
      setAuthError('Add at least one recipient email address.')
      return
    }

    if (!subject.trim() || !body.trim()) {
      setAuthError('Subject and body are required.')
      return
    }

    setAuthError('')
    setIsSending(true)
    setSendLogs([])

    let successCount = 0

    for (const recipient of recipients) {
      try {
        await sendGmailMessage({
          accessToken: token,
          to: recipient,
          subject,
          body,
          attachments,
        })

        successCount += 1
        setSendLogs((previous) => [
          ...previous,
          { recipient, status: 'Sent' },
        ])
      } catch (error) {
        setSendLogs((previous) => [
          ...previous,
          { recipient, status: `Failed: ${error.message}` },
        ])
      }
    }

    setIsSending(false)

    setSendLogs((previous) => [
      ...previous,
      {
        recipient: 'Summary',
        status: `${successCount}/${recipients.length} sent successfully`,
      },
    ])
  }

  return (
    <main className="app-shell">
      <header className="app-header">
        <h1>Email Auto Sender</h1>
        <p>Send one message to many recipients using your Google account.</p>
      </header>

      <section className="card">
        <h2>Google Connection</h2>
        <p className="muted">
          Scope used: gmail.send + userinfo.email. Your token stays in this browser
          session.
        </p>
        <div className="row">
          <button
            type="button"
            className="btn"
            onClick={connectGoogle}
            disabled={authLoading || isSending}
          >
            {authLoading ? 'Connecting...' : 'Connect Google'}
          </button>
          <button
            type="button"
            className="btn btn-ghost"
            onClick={disconnectGoogle}
            disabled={!token || isSending}
          >
            Disconnect
          </button>
        </div>
        <p className="status">
          {token
            ? `Connected as ${accountEmail || 'Google account'}`
            : 'Not connected'}
        </p>
        {authError ? <p className="error">{authError}</p> : null}
      </section>

      <section className="card two-col">
        <div>
          <h2>Templates</h2>
          <label>
            Template name
            <input
              value={templateName}
              onChange={(event) => setTemplateName(event.target.value)}
              placeholder="Monthly follow-up"
            />
          </label>
          <label>
            Saved templates
            <select
              value={selectedTemplateId}
              onChange={(event) => applyTemplate(event.target.value)}
            >
              <option value="">Select a template</option>
              {templates.map((entry) => (
                <option key={entry.id} value={entry.id}>
                  {entry.name}
                </option>
              ))}
            </select>
          </label>
          <div className="row">
            <button
              type="button"
              className="btn"
              onClick={saveCurrentTemplate}
              disabled={isSending}
            >
              Save Template
            </button>
            <button
              type="button"
              className="btn btn-ghost"
              onClick={removeSelectedTemplate}
              disabled={!selectedTemplateId || isSending}
            >
              Delete Template
            </button>
          </div>
        </div>

        <div>
          <h2>Recipients</h2>
          <label>
            Emails (comma, semicolon, or new line separated)
            <textarea
              rows="10"
              value={recipientsText}
              onChange={(event) => setRecipientsText(event.target.value)}
              placeholder="user1@example.com&#10;user2@example.com"
            />
          </label>
          <p className="muted">Detected unique recipients: {recipients.length}</p>
        </div>
      </section>

      <section className="card">
        <h2>Compose</h2>
        <label>
          Subject
          <input
            value={subject}
            onChange={(event) => setSubject(event.target.value)}
            placeholder="Your subject line"
          />
        </label>
        <label>
          Body
          <textarea
            rows="8"
            value={body}
            onChange={(event) => setBody(event.target.value)}
            placeholder="Write your message"
          />
        </label>
        <label>
          Attachments
          <input type="file" multiple onChange={onAttachmentsChange} />
        </label>
        <ul className="attachment-list">
          {attachments.map((file) => (
            <li key={`${file.name}-${file.lastModified}`}>
              {file.name} ({Math.ceil(file.size / 1024)} KB)
            </li>
          ))}
        </ul>
      </section>

      <section className="card">
        <h2>Send</h2>
        <div className="row">
          <button
            type="button"
            className="btn"
            onClick={sendToAllRecipients}
            disabled={isSending}
          >
            {isSending ? 'Sending...' : 'Send To All'}
          </button>
        </div>

        <div className="log-box">
          {sendLogs.length ? (
            sendLogs.map((log, index) => (
              <p key={`${log.recipient}-${index}`}>
                {log.recipient}: {log.status}
              </p>
            ))
          ) : (
            <p className="muted">No send logs yet.</p>
          )}
        </div>
      </section>

      <footer className="app-footer">
        <p>Powered by Gmail API in your browser.</p>
      </footer>
    </main>
  )
}

export default App
