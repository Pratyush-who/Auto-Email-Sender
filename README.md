# Email Auto Sender

A browser-based Gmail sender that lets you:

- connect your Google account
- send the same email to multiple recipients
- save and reuse templates in local storage
- attach files

## 1) Environment Setup

Create or edit [.env](.env):

```env
VITE_GOOGLE_CLIENT_ID=YOUR_GOOGLE_OAUTH_CLIENT_ID.apps.googleusercontent.com
```

The app reads only this variable.

## 2) How To Get Google Credentials

1. Open Google Cloud Console: https://console.cloud.google.com/
2. Create a project (or select an existing project).
3. Go to APIs & Services > Library and enable:
	- Gmail API
4. Go to APIs & Services > OAuth consent screen:
	- Choose External (or Internal for Workspace)
	- Fill app name, support email, developer email
    For Application type:
5. Go to APIs & Services > Credentials > Create Credentials > OAuth client ID.
6. Application type: Web application.
7. Add Authorized JavaScript origins:
	- http://localhost:5173
8. (Optional) Add Authorized redirect URIs:
	- http://localhost:5173
9. Copy the Client ID and paste it in [.env](.env) as VITE_GOOGLE_CLIENT_ID.

## 3) Run

```bash
npm install
npm run dev
```

Then open http://localhost:5173 and click Connect Google.

## 4) Common Issues

- Connect button appears but stays Not connected:
  - VITE_GOOGLE_CLIENT_ID missing or invalid
  - localhost:5173 not added as Authorized JavaScript origin
  - OAuth consent screen not configured fully
  - logged-in account is not in Test users

- Gmail send fails:
  - Gmail API not enabled in your project
  - consent not granted for gmail.send scope
