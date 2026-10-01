// The run that is going, one for the whole workspace, so every screen and
// the pulsing dot show the same state without a refresh. Start and Stop
// live here too and show at once, before the server answers.

import { api, type Run } from './api'
import { errorText, toast } from './toast.svelte'

const GOING_MS = 2000
const QUIET_MS = 5000

class RunWatch {
  // The run that is going, or null.
  run = $state<Run | null>(null)
  // The run that ended while the workspace was open, with what it brought.
  ended = $state<Run | null>(null)
  // Counts every answer while a run goes, so lists can follow along.
  tick = $state(0)
  busy = $state<'' | 'starting' | 'stopping'>('')
  private timer: ReturnType<typeof setTimeout> | undefined
  private users = 0
  private seq = 0

  // Screens that show the run call this and the function it returns on
  // leaving. The watch polls while anyone looks.
  watch(): () => void {
    this.users++
    if (this.users === 1) {
      document.addEventListener('visibilitychange', this.onVisible)
      void this.poll()
    }
    return () => {
      this.users--
      if (this.users === 0) {
        clearTimeout(this.timer)
        document.removeEventListener('visibilitychange', this.onVisible)
      }
    }
  }

  private onVisible = () => {
    if (document.visibilityState === 'visible') void this.poll()
  }

  async poll(): Promise<void> {
    clearTimeout(this.timer)
    const mine = ++this.seq
    try {
      const run = await api.currentRun()
      if (mine !== this.seq || this.busy) return
      this.settle(run)
    } catch {
      // Keep what is shown. The next answer puts it right.
    } finally {
      if (mine === this.seq && this.users > 0 && document.visibilityState === 'visible') {
        this.timer = setTimeout(() => void this.poll(), this.run || this.busy ? GOING_MS : QUIET_MS)
      }
    }
  }

  // A run that was going and is no more ended. Its end says what it brought.
  private settle(run: Run | null) {
    const before = this.run
    this.run = run
    this.tick++
    if (before && !run && before.id > 0) {
      api
        .run(before.id)
        .then((r) => (this.ended = r.run))
        .catch(() => (this.ended = { ...before, state: 'finished', progress: null }))
    }
  }

  async start(): Promise<void> {
    if (this.busy || this.run) return
    this.busy = 'starting'
    this.ended = null
    // A run shows at once, starting, until the server has queued its work.
    this.run = placeholder()
    try {
      const r = await api.startRun()
      if (!r.started) toast.show('A run is already going')
    } catch (e) {
      this.run = null
      toast.show(errorText(e), 'bad')
    } finally {
      this.busy = ''
      this.seq++
      void this.poll()
    }
  }

  async stop(): Promise<void> {
    const run = this.run
    if (this.busy || !run || run.id <= 0) return
    this.busy = 'stopping'
    // The run shows as stopped at once. What is running finishes by itself.
    this.run = null
    this.ended = { ...run, state: 'stopped', progress: null, finished_at: new Date().toISOString() }
    try {
      await api.stopRun(run.id)
      toast.show('Run stopped. What it did not get to waits for the next run')
    } catch (e) {
      this.run = run
      this.ended = null
      toast.show(errorText(e), 'bad')
    } finally {
      this.busy = ''
      this.seq++
      api.run(run.id).then((r) => {
        if (!this.run && this.ended?.id === run.id) this.ended = r.run
      }).catch(() => {})
      void this.poll()
    }
  }
}

// What a run looks like in the moment between Start and the server's answer.
function placeholder(): Run {
  const now = new Date().toISOString()
  return {
    id: 0, kind: 'manual', state: 'going', started_at: now, finished_at: null,
    sources_checked: 0, events_found: 0, events_new: 0, pages_read: 0, reads_failed: 0,
    startups_looked_up: 0, lookups_failed: 0, people_new: 0, fits_new: 0, errors: 0,
    progress: { checks: 0, checks_done: 0, reads: 0, reads_done: 0, reads_expected: 0, lookups: 0, lookups_done: 0, lookups_expected: 0, now: [], seconds_left: null },
  }
}

export const runWatch = new RunWatch()
