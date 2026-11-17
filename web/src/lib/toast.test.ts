import { afterEach, describe, expect, it, vi } from 'vitest'

// said posts to the app's log, which is not there in a test.
vi.mock('./said', () => ({ said: () => {} }))

const { fault, toast } = await import('./toast.svelte')

describe('toast', () => {
  afterEach(() => toast.hide())

  // Report goes only beside a failure the person cannot put right, as
  // FrameFairy's report mark, its issue #311.
  it('offers Report only for a fault', () => {
    toast.show('Paste the Exa key in Settings first', 'bad')
    expect(toast.report).toBe(false)
    fault(new Error('The file could not be written'))
    expect(toast.message).toBe('The file could not be written')
    expect(toast.tone).toBe('bad')
    expect(toast.report).toBe(true)
  })

  it('never offers Report for what was done, and forgets it when it goes', () => {
    toast.show('Run stopped', 'plain', null, true)
    expect(toast.report).toBe(false)
    fault('not an error')
    expect(toast.message).toBe('Something went wrong')
    toast.hide()
    expect(toast.report).toBe(false)
  })
})
