<script lang="ts">
  import { api, type Run } from '../lib/api'
  import { fmtAgo, fmtDateTime, fmtDuration, fmtNum } from '../lib/format'
  import { Load } from '../lib/load.svelte'
  import { navigate, router } from '../lib/router.svelte'
  import { toast } from '../lib/toast.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import Skeleton from '../lib/components/Skeleton.svelte'
  import RunSheet from './RunSheet.svelte'

  const runs = new Load<Run[]>()
  runs.run(api.runs)

  $effect(() => {
    document.title = 'Runs · Speaker Trail'
  })

  let openId = $derived.by(() => {
    const m = router.match('/app/runs/:id')
    return m ? Number(m.id) : null
  })

  // A run that is still going refreshes the list every few seconds, so its
  // numbers grow while you watch.
  let going = $derived(runs.data?.some((r) => !r.finished_at && r.kind !== 'check') ?? false)
  $effect(() => {
    if (!runs.data?.some((r) => !r.finished_at)) return
    const t = setInterval(() => runs.run(api.runs), 5000)
    return () => clearInterval(t)
  })

  let starting = $state(false)
  async function start() {
    starting = true
    try {
      const r = await api.startRun()
      if (!r.started) toast.show('A run is already going')
      await runs.run(api.runs)
    } catch (e) {
      toast.show(e instanceof Error ? e.message : 'Could not start the run')
    } finally {
      starting = false
    }
  }

  const KIND: Record<Run['kind'], string> = { nightly: 'nightly', manual: 'started by hand', check: 'Check now' }

  function took(r: Run): string {
    if (!r.finished_at) return 'still running'
    return `took ${fmtDuration(new Date(r.finished_at).getTime() - new Date(r.started_at).getTime())}`
  }
</script>

<div class="view">
  <div class="view-head">
    <div>
      <h1>Runs</h1>
      <p>Each run, what it checked and what it found.</p>
    </div>
    <button class="btn primary start" type="button" onclick={start} disabled={starting || going}
      title="Check every due source now, like the nightly run">
      <Icon name="refresh" />{starting ? 'Starting' : going ? 'Running' : 'Start a run'}
    </button>
  </div>

  {#if runs.error && !runs.data}
    <EmptyState icon="alert" tone="bad" title="Could not load the runs" text={runs.error}>
      <button class="btn" type="button" onclick={() => runs.run(api.runs)}><Icon name="refresh" />Try again</button>
    </EmptyState>
  {:else if !runs.data}
    <Skeleton count={6} />
  {:else if !runs.data.length}
    <EmptyState icon="runs" title="No runs yet" text="Start one now, or the first one starts tonight." />
  {:else}
    <ul class="list">
      {#each runs.data as r (r.id)}
        <li>
          <a class="row-btn run" href="/app/runs/{r.id}" aria-current={openId === r.id ? 'true' : undefined}>
            <span class="when">
              <b>{fmtDateTime(r.started_at)}</b>
              <span class="faint small">{fmtAgo(r.started_at)}, {KIND[r.kind] ?? r.kind}, {took(r)}</span>
            </span>
            <span class="nums">
              <span><span class="num">{fmtNum(r.sources_checked)}</span> checked</span>
              <span><span class="num">{fmtNum(r.events_new)}</span> new events</span>
              <span><span class="num">{fmtNum(r.people_new)}</span> new people</span>
              <span class:err={r.errors > 0}><span class="num">{fmtNum(r.errors)}</span> {r.errors === 1 ? 'error' : 'errors'}</span>
            </span>
            <span class="go" aria-hidden="true"><Icon name="chevron" /></span>
          </a>
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
  /* Room for the longest label, so the button keeps its width. */
  .start { min-width: 10.5em; justify-content: center; }
  .run { grid-template-columns: minmax(0, 1fr) auto; }
  .when { display: grid; gap: 1px; min-width: 0; }
  .nums { grid-column: 1; display: flex; flex-wrap: wrap; gap: 4px 16px; font-size: 13px; color: var(--ink-2); }
  .nums .num { color: var(--ink); font-weight: 500; }
  .nums .err, .nums .err .num { color: var(--bad); }
  .go { grid-column: 2; grid-row: 1 / span 2; color: var(--ink-3); }
</style>
