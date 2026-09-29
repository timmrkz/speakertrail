<script lang="ts">
  import { api, type Run } from '../lib/api'
  import { fmtAgo, fmtDateTime, fmtDuration } from '../lib/format'
  import { Load } from '../lib/load.svelte'
  import { navigate, router } from '../lib/router.svelte'
  import { runWatch } from '../lib/run.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import Skeleton from '../lib/components/Skeleton.svelte'
  import RunOutcome from '../lib/components/RunOutcome.svelte'
  import RunProgress from '../lib/components/RunProgress.svelte'
  import RunStatePill from '../lib/components/RunStatePill.svelte'
  import RunSheet from './RunSheet.svelte'

  const runs = new Load<Run[]>()

  $effect(() => {
    document.title = 'Runs · Speaker Trail'
  })

  $effect(() => runWatch.watch())

  let openId = $derived.by(() => {
    const m = router.match('/app/runs/:id')
    return m ? Number(m.id) : null
  })

  // The list follows the watch: it reads again with every answer while a
  // run goes, and once more when one ends. A Check now going refreshes it
  // too, every few seconds.
  $effect(() => {
    void runWatch.tick
    void runWatch.ended
    runs.run(api.runs, { quiet: true })
  })
  $effect(() => {
    if (!runs.data?.some((r) => r.state === 'going' && r.kind === 'check')) return
    const t = setInterval(() => runs.run(api.runs, { quiet: true }), 2000)
    return () => clearInterval(t)
  })

  // The going run and the one that just ended come from the watch, so the
  // list shows what the dot and every other screen show.
  let list = $derived.by(() => {
    const out = (runs.data ?? []).map((r) => {
      if (runWatch.run && r.id === runWatch.run.id) return runWatch.run
      if (runWatch.ended && r.id === runWatch.ended.id) return runWatch.ended
      return r
    })
    // A run just started shows at once, before the list has it.
    const w = runWatch.run
    if (w && !out.some((r) => r.id === w.id)) out.unshift(w)
    return out
  })

  let going = $derived(runWatch.run)
  let label = $derived(
    runWatch.busy === 'starting' ? 'Starting' : runWatch.busy === 'stopping' ? 'Stopping' : going ? 'Stop the run' : 'Start a run',
  )

  const KIND: Record<Run['kind'], string> = { nightly: 'nightly', manual: 'started by hand', check: 'Check now' }

  function took(r: Run): string {
    if (r.state === 'going') return 'still running'
    if (!r.finished_at) return ''
    return `took ${fmtDuration(new Date(r.finished_at).getTime() - new Date(r.started_at).getTime())}`
  }
</script>

<div class="view">
  <div class="view-head">
    <div>
      <h1>Runs</h1>
      <p>Each run, what it checked and what it found.</p>
    </div>
    <button class="btn start" class:primary={!going} type="button" onclick={() => (going ? runWatch.stop() : runWatch.start())}
      disabled={runWatch.busy !== '' || (going !== null && going.id <= 0)}
      title={going ? 'Stop this run. What is running finishes, the rest waits for the next run' : 'Check every due source now, like the nightly run'}>
      <Icon name={going ? 'close' : 'refresh'} />{label}
    </button>
  </div>

  {#if runs.error && !runs.data}
    <EmptyState icon="alert" tone="bad" title="Could not load the runs" text={runs.error}>
      <button class="btn" type="button" onclick={() => runs.run(api.runs)}><Icon name="refresh" />Try again</button>
    </EmptyState>
  {:else if !runs.data}
    <Skeleton count={6} />
  {:else if !list.length}
    <EmptyState icon="runs" title="No runs yet" text="Start one now, or the first one starts tonight." />
  {:else}
    <ul class="list">
      {#each list as r (r.id)}
        <li>
          <a class="row-btn run" href={r.id > 0 ? `/app/runs/${r.id}` : undefined} aria-current={openId === r.id ? 'true' : undefined}>
            <span class="when">
              <b>{fmtDateTime(r.started_at)}</b>
              <span class="faint small">{[fmtAgo(r.started_at), KIND[r.kind] ?? r.kind, took(r)].filter(Boolean).join(', ')}</span>
            </span>
            <span class="state"><RunStatePill state={r.state} /></span>
            <span class="brought"><RunOutcome run={r} /></span>
          </a>
          {#if r.state === 'going' && r.progress}<div class="going"><RunProgress run={r} /></div>{/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>

{#if openId !== null}
  {#key openId}
    <RunSheet id={openId} onclose={() => navigate('/app/runs')} />
  {/key}
{/if}

<style>
  .going { padding: 0 16px 14px; }
  /* Room for the longest label, so the button keeps its width. */
  .start { min-width: 11.2em; justify-content: center; }
  .run { grid-template-columns: minmax(0, 1fr) auto; row-gap: 4px; }
  .when { display: grid; gap: 1px; min-width: 0; }
  .state { grid-column: 2; grid-row: 1; align-self: start; }
  .brought { grid-column: 1 / span 2; min-width: 0; }
</style>
