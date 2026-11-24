// worker/test/manual.test.js
import test from 'node:test'
import assert from 'node:assert/strict'
import { processManualImport } from '../src/manual.js'

test('Manual Import - Normalizes valid pasted post', () => {
  const result = processManualImport({
    url: 'https://www.linkedin.com/feed/update/urn:li:activity:99887766/',
    text: 'We are hiring a founding Go engineer for our team.',
    author: 'Sam Altman',
    company: 'OpenCore Labs',
    role: 'Founding Engineer',
    work_mode: 'remote',
    paid_status: 'paid',
    location: 'Remote',
    stipend: 50000,
  })

  assert.equal(result.url, 'https://www.linkedin.com/feed/update/urn:li:activity:99887766/')
  assert.equal(result.company, 'OpenCore Labs')
  assert.equal(result.role, 'Founding Engineer')
  assert.equal(result.work_mode, 'remote')
  assert.equal(result.extracted.stipend, 50000)
  assert.equal(result.status, 'new')
})

test('Manual Import - Throws on missing required fields', () => {
  assert.throws(() => {
    processManualImport({ text: 'Some text' })
  }, /Missing required field: 'url'/)

  assert.throws(() => {
    processManualImport({ url: 'https://example.com' })
  }, /Missing required field: 'text'/)
})
