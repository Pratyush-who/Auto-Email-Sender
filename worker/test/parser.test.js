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
  assert.match(url1, /datePosted=%22past-24h%22/)
  assert.match(url1, /sortBy=%22date_posted%22/)

  const url2 = buildLinkedInSearchUrl({
    query: 'react engineer',
    timeFilter: 'month',
    sortBy: 'relevance',
  })

  assert.match(url2, /keywords=react\+engineer/)
  assert.match(url2, /datePosted=%22past-month%22/)
  assert.match(url2, /sortBy=%22relevance%22/)
})
