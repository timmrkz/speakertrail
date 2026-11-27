// The run that is going, one for the whole workspace, so every screen and
// the pulsing dot show the same state without a refresh. Start and Stop
// live here too and show at once, before the server answers.

import { api, type Added, type Run } from './api'
import { fmtElapsed, fmtLeft, plural, runBrought } from './format'
import { navigate, router } from './router.svelte'
import { errorText, Problem } from './problem.svelte'
import { toast } from './toast.svelte'

const GOING_MS = 2000
const QUIET_MS = 5000

class RunWatch {
  // The run that is going, or null.
  run = $state<Run | null>(null)
  // The run that ended while the workspace was open, with what it brought.
  ended = $state<Run | null>(null)
  // Counts every answer while a run goes, so lists can follow along.
  tick = $state(0)
  busy = $state<'' | 'starting' | 'stopping' | 'adding'>('')
  // What the last links added brought, until People has shown it under
  // New, see shown.
  added = $state<Added | null>(null)
  // What went wrong starting or stopping a run, or adding links, which
  // People says under Run, where it was asked for, see
  // lib/problem.svelte.ts.
  problem = new Problem()
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
        .then((r) => {
          this.ended = r.run
          ended(r.run)
        })
        .catch(() => (this.ended = { ...before, state: 'finished', progress: null }))
    }
  }

  async start(): Promise<void> {
    if (this.busy || this.run) return
    this.busy = 'starting'
    this.ended = null
    this.problem.clear()
    // A run shows at once, starting, until the server has queued its work.
    this.run = placeholder()
    try {
      const r = await api.startRun()
      if (!r.started) toast.show('A run is already going')
    } catch (e) {
      this.run = null
      this.problem.say(e)
    } finally {
      this.busy = ''
      this.seq++
      void this.poll()
    }
  }

  // Adds the people behind the links in a text, from the search on
  // People or dropped on the window. Their profiles are read by the run
  // going, or by a run of their own that shows at once. People then
  // shows them under New, or opens the one person the links lead to who
  // was here already. Links dropped while others are added wait their
  // turn rather than go lost. A toast says what was added, with Undo.
  add(text: string): Promise<Added | null> {
    const next = this.adding.then(() => this.addNow(text))
    this.adding = next.catch(() => null)
    return next
  }
  private adding: Promise<unknown> = Promise.resolve()
  // Counts every change adding made or took back, so People reads again.
  changed = $state(0)

  private async addNow(text: string): Promise<Added | null> {
    this.busy = 'adding'
    this.problem.clear()
    const was = this.run
    if (!was) {
      this.ended = null
      this.run = placeholder()
    }
    try {
      const r = await api.addPeople(text)
      if (r.open) navigate(`/app/people/${r.open}`)
      else {
        this.added = r
        navigate('/app/people')
      }
      this.changed++
      const said = r.new.length ? `Added ${plural(r.new.length, 'person', 'people')}` : r.connected.length ? `Marked ${plural(r.connected.length, 'person', 'people')} connected` : ''
      const undo = said ? { label: 'Undo', run: () => void this.undo(r) } : null
      if (r.wait) toast.show(`${said || 'Added'}. ${r.wait}`, 'bad', undo)
      else if (said) toast.show(said, 'plain', undo)
      return r
    } catch (e) {
      if (!was) this.run = null
      // Links dropped on another page say what went wrong on People,
      // where they would have shown.
      this.problem.say(e)
      if (!router.path.startsWith('/app/people')) navigate('/app/people')
      return null
    } finally {
      this.busy = ''
      this.seq++
      void this.poll()
    }
  }

  // Takes back what adding did: whom it brought in goes again, and whom
  // it marked connected is open again.
  private async undo(r: Added) {
    try {
      await Promise.all([...r.new.map((id) => api.removePerson(id)), ...r.connected.map((id) => api.patchPerson(id, { connected: false }))])
      const open = router.match('/app/people/:id')
      if (open && r.new.includes(Number(open.id))) navigate('/app/people', { replace: true })
    } catch (e) {
      // Undo on a toast that is gone has no place of its own, so a toast
      // says what went wrong (Tim, 27 November 2026, #72).
      toast.show(errorText(e), 'bad', null, true)
    } finally {
      this.changed++
      void this.poll()
    }
  }

  async stop(): Promise<void> {
    const run = this.run
    if (this.busy || !run || run.id <= 0) return
    this.busy = 'stopping'
    this.problem.clear()
    // The run shows as stopped at once. What is running finishes by itself.
    this.run = null
    this.ended = { ...run, state: 'stopped', progress: null, finished_at: new Date().toISOString() }
    try {
      await api.stopRun(run.id)
      toast.show('Run stopped. What it did not get to waits for the next run')
    } catch (e) {
      this.run = run
      this.ended = null
      this.problem.failed(e)
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

// A run that ended while Tim looked says what it brought, the way a
// toast says anything done. One stopped by hand said so already.
function ended(run: Run) {
  if (run.state === 'stopped') return
  const out = runBrought(run)
  toast.show(out.failed ? `Run done. ${out.brought}. ${out.failed}` : `Run done. ${out.brought}`, out.failed ? 'bad' : 'plain')
}

// What a going run does now and about how long is left, in the words the
// Stop button gives under the pointer. answeredAt is when the app last
// said how long is left, so the time counts down between answers.
export function runSays(run: Run, now: number, answeredAt: number): string {
  const p = run.progress
  if (!p) return 'Starting'
  const first = p.now[0]
  let step = 'Starting'
  if (first) {
    step = first.kind === 'check' ? `Checking ${first.label}` : first.kind === 'read' ? `Reading the profile of ${first.label}` : first.label
    if (p.now.length > 1) step += ` and ${p.now.length - 1} more`
  } else if (p.checks_done + p.reads_done > 0) {
    step = 'Finishing'
  }
  const left = p.seconds_left == null ? 'measuring the time left' : fmtLeft(p.seconds_left - (now - answeredAt) / 1000)
  const counts = [p.checks ? `${p.checks_done} of ${plural(p.checks, 'search', 'searches')}` : '', p.reads ? `${p.reads_done} of ${plural(p.reads, 'profile')}` : ''].filter(Boolean).join(', ')
  return [step, counts, `${fmtElapsed(now - new Date(run.started_at).getTime())} so far, ${left}`].filter(Boolean).join('. ')
}

// How many of a run's searches and profiles are done, and of how many,
// as New counts them while the run goes, see People.
export function runCount(run: Run | null): { done: number; of: number } | null {
  const p = run?.progress
  if (!p || run!.id <= 0) return null
  return { done: p.checks_done + p.reads_done, of: p.checks + Math.max(p.reads, p.reads_expected) }
}

// What a run looks like in the moment between Start and the server's answer.
function placeholder(): Run {
  const now = new Date().toISOString()
  return {
    id: 0, kind: 'manual', state: 'going', started_at: now, finished_at: null,
    sources_checked: 0, events_found: 0, events_new: 0, pages_read: 0, reads_failed: 0,
    startups_looked_up: 0, lookups_failed: 0, people_new: 0, errors: 0,
    progress: { checks: 0, checks_done: 0, reads: 0, reads_done: 0, reads_expected: 0, lookups: 0, lookups_done: 0, lookups_expected: 0, now: [], seconds_left: null },
  }
}

export const runWatch = new RunWatch()

// How far a run is, from 0 to 1, or -1 while it cannot say. The total
// counts the reads the run expects, so the share does not jump back each
// time links join it. The fill of the Stop button reads it, and New
// counts the same work, see runCount, so the two never disagree.
export function runShare(run: Run | null): number {
  const p = run?.progress
  if (!p) return -1
  const total = p.checks + Math.max(p.reads, p.reads_expected) + Math.max(p.lookups, p.lookups_expected)
  if (!total) return -1
  return Math.min(1, (p.checks_done + p.reads_done + p.lookups_done) / total)
}
