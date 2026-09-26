<script lang="ts">
  // How far a going run is: what it does now, a fill, the counts, the time
  // so far and about how long is left. The one way a run shows its work,
  // in the list and in the run's sheet alike.
  import type { Run } from '../api'
  import { fmtElapsed, fmtLeft, plural } from '../format'

  let { run }: { run: Run } = $props()

  // The clock ticks here, between the answers from the server, and the
  // time left counts down from the last answer.
  let now = $state(Date.now())
  let answeredAt = $state(Date.now())
  $effect(() => {
    void run.progress
    answeredAt = Date.now()
  })
  $effect(() => {
    const t = setInterval(() => (now = Date.now()), 1000)
    return () => clearInterval(t)
  })

  let p = $derived(run.progress)
  // The total counts the reads and lookups the run expects, so the fill
  // does not jump back each time a check queues new ones.
  let total = $derived(p ? p.checks + Math.max(p.reads, p.reads_expected) + Math.max(p.lookups, p.lookups_expected) : 0)
  let done = $derived(p ? p.checks_done + p.reads_done + p.lookups_done : 0)
  let share = $derived(total ? Math.min(1, done / total) : 0)
  let step = $derived.by(() => {
    const first = p?.now[0]
    if (!first) return done === 0 ? 'Starting' : done >= total ? 'Finishing' : 'Waiting for the next website to allow a request'
    const what =
      first.kind === 'check' ? `Checking ${first.label}` : first.kind === 'lookup' ? `Looking up ${first.label}` : `Reading the page of ${first.label}`
    const more = (p?.now.length ?? 1) - 1
    return more > 0 ? `${what} and ${more} more` : what
  })
  let left = $derived(p?.seconds_left == null ? 'measuring the time left' : fmtLeft(p.seconds_left - (now - answeredAt) / 1000))
  let counts = $derived(
    p
      ? [
          `${p.checks_done} of ${plural(p.checks, 'check')}`,
          p.reads ? `${p.reads_done} of ${plural(p.reads, 'read')}` : '',
          p.lookups ? `${p.lookups_done} of ${plural(p.lookups, 'lookup')}` : '',
        ].filter(Boolean).join(' · ')
      : '',
  )
</script>

<div class="run-progress" aria-live="polite">
  <p class="step ellipsis" title={step}>{step}</p>
  <div class="progress" class:unknown={!total} role="progressbar" aria-label="How far the run is"
    aria-valuemin={0} aria-valuemax={100} aria-valuenow={total ? Math.round(share * 100) : undefined}>
    <i style:width={`${share * 100}%`}></i>
  </div>
  <p class="meta num">
    <span>{counts}</span>
    <span>{fmtElapsed(now - new Date(run.started_at).getTime())} so far, {left}</span>
  </p>
</div>

<style>
  .run-progress { display: grid; gap: 6px; min-width: 0; }
  .step { font-size: 14px; font-weight: 600; color: var(--ink); min-height: 1.4em; }
  .meta { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 2px 12px; font-size: 12px; color: var(--ink-2); }
</style>
