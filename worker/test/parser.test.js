// worker/test/parser.test.js
import test from 'node:test'
import assert from 'node:assert/strict'
import { buildLinkedInSearchUrl } from '../src/parser.js'

test('Parser - Builds proper LinkedIn search URLs', () => {
  const url1 = buildLinkedInSearchUrl({
    query: 'golang remote',
    timeFilter: '24h',
    sortBy: 'latest',
  })

  assert.match(url1, /keywords=golang\+remote/)
  assert.match(url1, /datePosted=%5B%22past-24h%22%5D/)
  assert.match(url1, /sortBy=%5B%22date_posted%22%5D/)

  const url2 = buildLinkedInSearchUrl({
    query: 'react engineer',
    timeFilter: 'month',
    sortBy: 'relevance',
  })

  assert.match(url2, /keywords=react\+engineer/)
  assert.match(url2, /datePosted=%5B%22past-month%22%5D/)
})
