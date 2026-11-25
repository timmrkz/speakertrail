import { describe, expect, it } from 'vitest'
import { clock } from './clock'

describe('clock', () => {
  it('writes a length the way a player shows it', () => {
    expect(clock(7.9)).toBe('0:07')
    expect(clock(65)).toBe('1:05')
    expect(clock(3723)).toBe('1:02:03')
  })
  it('writes nothing it cannot read as no time', () => {
    expect(clock(Number.NaN)).toBe('0:00')
    expect(clock(-3)).toBe('0:00')
  })
})
