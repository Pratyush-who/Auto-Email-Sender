// worker/test/guardrails.test.js
import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { Guardrails } from '../src/guardrails.js'

test('Guardrails - Single run concurrency lock', () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'guardrails_test_'))
  const g = new Guardrails(tmpDir)

  const first = g.acquireRunLock('run_1')
  assert.equal(first.allowed, true)
  assert.equal(g.isLocked, true)

  const second = g.acquireRunLock('run_2')
  assert.equal(second.allowed, false)
  assert.equal(second.code, 'CONCURRENT_RUN_REJECTED')

  g.releaseRunLock()
  assert.equal(g.isLocked, false)

  fs.rmSync(tmpDir, { recursive: true, force: true })
})

test('Guardrails - Daily limit of 3 runs enforced', () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'guardrails_test_'))
  const g = new Guardrails(tmpDir)

  // 1st run
  const r1 = g.acquireRunLock('r1')
  assert.equal(r1.allowed, true)
  g.releaseRunLock()

  // 2nd run
  const r2 = g.acquireRunLock('r2')
  assert.equal(r2.allowed, true)
  g.releaseRunLock()

  // 3rd run
  const r3 = g.acquireRunLock('r3')
  assert.equal(r3.allowed, true)
  g.releaseRunLock()

  // 4th run should be rejected by daily quota
  const r4 = g.acquireRunLock('r4')
  assert.equal(r4.allowed, false)
  assert.equal(r4.code, 'DAILY_LIMIT_EXCEEDED')

  fs.rmSync(tmpDir, { recursive: true, force: true })
})

test('Guardrails - Clamps max posts to 60', () => {
  const g = new Guardrails('./tmp')

  assert.equal(g.clampMaxPosts(100), 60)
  assert.equal(g.clampMaxPosts(15), 15)
  assert.equal(g.clampMaxPosts(-5), 60)
  assert.equal(g.clampMaxPosts('invalid'), 60)
})

test('Guardrails - Detects login walls and checkpoints', () => {
  const g = new Guardrails('./tmp')

  // URL detection
  const check1 = g.detectCheckpointOrAuthWall('https://www.linkedin.com/checkpoint/challenge/123')
  assert.equal(check1.detected, true)

  const check2 = g.detectCheckpointOrAuthWall('https://www.linkedin.com/authwall?trk=abc')
  assert.equal(check2.detected, true)

  // Content detection
  const check3 = g.detectCheckpointOrAuthWall(
    'https://www.linkedin.com/search',
    '<div>Please complete this Security Verification to continue</div>'
  )
  assert.equal(check3.detected, true)

  // Clean page
  const clean = g.detectCheckpointOrAuthWall(
    'https://www.linkedin.com/search/results/content/',
    '<div>Showing 1,234 posts for golang</div>'
  )
  assert.equal(clean.detected, false)
})
