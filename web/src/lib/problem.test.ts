import { afterEach, describe, expect, it, vi } from 'vitest'

// said posts to the app's log, which is not there in a test.
const lines: { text: string; how: unknown }[] = []
vi.mock('./said', () => ({ said: (text: string, how: unknown) => lines.push({ text, how }) }))

const { Problem } = await import('./problem.svelte')

describe('problem', () => {
  afterEach(() => (lines.length = 0))

  // Report goes only beside a failure the person cannot put right, as
  // FrameFairy's report mark, its issue #311.
  it('offers Report only for a failure the person cannot put right', () => {
    const p = new Problem()
    p.say(new Error('Paste the Exa key in Settings first'))
    expect(p.text).toBe('Paste the Exa key in Settings first')
    expect(p.report).toBe(false)
    p.failed(new Error('The file could not be written'))
    expect(p.text).toBe('The file could not be written')
    expect(p.report).toBe(true)
  })

  it('says words given as they are, and the rest as something that went wrong', () => {
    const p = new Problem()
    p.say('Could not copy the message.')
    expect(p.text).toBe('Could not copy the message.')
    p.failed('not an error')
    expect(p.text).toBe('Something went wrong')
  })

  it('goes into the log the way Settings, Logs lists it, and forgets it when cleared', () => {
    const p = new Problem()
    p.failed(new Error('The file could not be written'))
    expect(lines).toEqual([{ text: 'The file could not be written', how: { level: 'warn', toast: true } }])
    p.needs('Enter a word or phrase')
    expect(p.text).toBe('Enter a word or phrase')
    expect(lines[1].how).toEqual({ level: 'info' })
    p.clear()
    expect(p.text).toBe('')
    expect(p.report).toBe(false)
  })
})
