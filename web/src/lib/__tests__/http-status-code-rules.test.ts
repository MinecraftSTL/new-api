import { describe, expect, it } from 'vitest'

import { parseHttpStatusCodeRules } from '../http-status-code-rules'

describe('parseHttpStatusCodeRules', () => {
  it('accepts 000 as the bad response body retry rule', () => {
    const result = parseHttpStatusCodeRules('000,500-503', {
      allowBadResponseBody: true,
    })

    expect(result.ok).toBe(true)
    expect(result.normalized).toBe('000,500-503')
    expect(result.ranges).toEqual([
      { start: 0, end: 0 },
      { start: 500, end: 503 },
    ])
  })

  it('rejects ranges that mix 000 with HTTP status codes', () => {
    const result = parseHttpStatusCodeRules('000-500', {
      allowBadResponseBody: true,
    })

    expect(result.ok).toBe(false)
    expect(result.invalidTokens).toEqual(['000-500'])
  })

  it('rejects 000 for non-retry status settings', () => {
    const result = parseHttpStatusCodeRules('000')

    expect(result.ok).toBe(false)
    expect(result.invalidTokens).toEqual(['000'])
  })
})
