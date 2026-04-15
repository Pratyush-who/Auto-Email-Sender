const GOOGLE_SCRIPT_ID = 'google-identity-services'

export const GOOGLE_SCOPES = [
  'https://www.googleapis.com/auth/gmail.send',
  'https://www.googleapis.com/auth/userinfo.email',
].join(' ')

const loadGoogleIdentityScript = () =>
  new Promise((resolve, reject) => {
    if (window.google?.accounts?.oauth2) {
      resolve(window.google)
      return
    }

    const existing = document.getElementById(GOOGLE_SCRIPT_ID)

    if (existing) {
      existing.addEventListener('load', () => resolve(window.google))
      existing.addEventListener('error', () =>
        reject(new Error('Could not load Google Identity Services script.')),
      )
      return
    }

    const script = document.createElement('script')
    script.id = GOOGLE_SCRIPT_ID
    script.src = 'https://accounts.google.com/gsi/client'
    script.async = true
    script.defer = true
    script.onload = () => resolve(window.google)
    script.onerror = () =>
      reject(new Error('Could not load Google Identity Services script.'))

    document.head.appendChild(script)
  })

export const createGoogleTokenClient = async ({ clientId, scope }) => {
  const google = await loadGoogleIdentityScript()

  if (!google?.accounts?.oauth2?.initTokenClient) {
    throw new Error('Google OAuth SDK did not initialize correctly.')
  }

  let resolver
  let rejecter

  const tokenPromise = new Promise((resolve, reject) => {
    resolver = resolve
    rejecter = reject
  })

  const tokenClient = google.accounts.oauth2.initTokenClient({
    client_id: clientId,
    scope,
    callback: (response) => {
      if (response.error) {
        rejecter(new Error(response.error_description || response.error))
        return
      }

      resolver(response)
    },
  })

  return {
    requestAccessToken: (options) => tokenClient.requestAccessToken(options),
    waitForToken: () => tokenPromise,
  }
}

export const fetchGoogleAccountEmail = async (accessToken) => {
  const response = await fetch('https://www.googleapis.com/oauth2/v3/userinfo', {
    headers: {
      Authorization: `Bearer ${accessToken}`,
    },
  })

  if (!response.ok) {
    throw new Error('Could not fetch Google account email.')
  }

  const payload = await response.json()
  return payload.email || ''
}

export const revokeGoogleToken = (accessToken) => {
  if (!window.google?.accounts?.oauth2 || !accessToken) {
    return
  }

  window.google.accounts.oauth2.revoke(accessToken)
}
