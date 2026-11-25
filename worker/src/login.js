// worker/src/login.js
import path from 'node:path'
import process from 'node:process'
import { chromium } from 'playwright'

const userDataDir = path.resolve(process.env.USER_DATA_DIR || './user_data')

console.log('====================================================================')
console.log('[EmailAuto] Launching Persistent Chrome for LinkedIn Login...')
console.log(`[EmailAuto] User Data Directory: ${userDataDir}`)
console.log('====================================================================')
console.log('👉 INSTRUCTIONS:')
console.log('1. A Google Chrome window is opening on your desktop right now.')
console.log('2. Log into your LinkedIn account in that browser window.')
console.log('3. Complete any two-factor or security verification.')
console.log('4. Once you can see your LinkedIn feed, you are all set!')
console.log('5. You can close the Chrome window or press Ctrl+C here when done.')
console.log('====================================================================')

try {
  let context
  try {
    context = await chromium.launchPersistentContext(userDataDir, {
      headless: false,
      channel: 'chrome',
      viewport: { width: 1280, height: 850 },
      args: ['--disable-blink-features=AutomationControlled', '--start-maximized'],
    })
  } catch {
    context = await chromium.launchPersistentContext(userDataDir, {
      headless: false,
      viewport: { width: 1280, height: 850 },
      args: ['--disable-blink-features=AutomationControlled'],
    })
  }

  const page = context.pages().length > 0 ? context.pages()[0] : await context.newPage()
  await page.goto('https://www.linkedin.com/login', { waitUntil: 'domcontentloaded' })

  console.log('[EmailAuto] Chrome is running. Waiting for your login...')

  context.on('close', () => {
    console.log('[EmailAuto] Browser closed. Session cookies saved permanently in worker/user_data!')
    process.exit(0)
  })
} catch (err) {
  console.error('[EmailAuto] Failed to launch Chrome:', err.message)
  process.exit(1)
}
