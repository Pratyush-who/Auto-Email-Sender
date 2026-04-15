const CRLF = '\r\n'

const normalizeRecipient = (value) => value.trim().replace(/^<|>$/g, '')

const toBase64Url = (input) =>
  btoa(input).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')

const encodeUtf8ToBase64Url = (text) => {
  const binary = unescape(encodeURIComponent(text))
  return toBase64Url(binary)
}

const arrayBufferToBinaryString = (arrayBuffer) => {
  const bytes = new Uint8Array(arrayBuffer)
  const chunkSize = 0x8000
  let result = ''

  for (let index = 0; index < bytes.length; index += chunkSize) {
    const chunk = bytes.subarray(index, index + chunkSize)
    result += String.fromCharCode(...chunk)
  }

  return result
}

const buildAttachmentPart = async (file, boundary) => {
  const buffer = await file.arrayBuffer()
  const binary = arrayBufferToBinaryString(buffer)
  const content = btoa(binary)

  return [
    `--${boundary}`,
    `Content-Type: ${file.type || 'application/octet-stream'}; name="${file.name}"`,
    'Content-Transfer-Encoding: base64',
    `Content-Disposition: attachment; filename="${file.name}"`,
    '',
    content,
  ].join(CRLF)
}

const buildRawMessage = async ({ to, subject, body, attachments }) => {
  const boundary = `boundary_${crypto.randomUUID()}`
  const recipient = normalizeRecipient(to)

  const headers = [
    `To: ${recipient}`,
    `Subject: ${subject}`,
    'MIME-Version: 1.0',
    `Content-Type: multipart/mixed; boundary="${boundary}"`,
    '',
    `--${boundary}`,
    'Content-Type: text/plain; charset="UTF-8"',
    'Content-Transfer-Encoding: 7bit',
    '',
    body,
  ]

  const attachmentParts = []

  for (const file of attachments) {
    attachmentParts.push(await buildAttachmentPart(file, boundary))
  }

  const mime = [...headers, ...attachmentParts, `--${boundary}--`, ''].join(CRLF)

  return encodeUtf8ToBase64Url(mime)
}

export const sendGmailMessage = async ({
  accessToken,
  to,
  subject,
  body,
  attachments = [],
}) => {
  const raw = await buildRawMessage({ to, subject, body, attachments })

  const response = await fetch(
    'https://gmail.googleapis.com/gmail/v1/users/me/messages/send',
    {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${accessToken}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ raw }),
    },
  )

  if (!response.ok) {
    let detail = 'Gmail API request failed.'

    try {
      const payload = await response.json()
      if (payload?.error?.message) {
        detail = payload.error.message
      }
    } catch {
      // Keep fallback error message.
    }

    throw new Error(detail)
  }

  return response.json()
}
