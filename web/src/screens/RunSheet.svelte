<script lang="ts">
  import { api, fetchUrl, type Check, type Run, type RunFailure, type SourceStatus } from '../lib/api'
  import { fmtDateTime, fmtDuration, STATUS_LABEL } from '../lib/format'
  import { Load } from '../lib/load.svelte'
  import { runWatch } from '../lib/run.svelte'
  import { errorText, toast } from '../lib/toast.svelte'
  import Chips from '../lib/components/Chips.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import RunOutcome from '../lib/components/RunOutcome.svelte'
  import RunProgress from '../lib/components/RunProgress.svelte'
  import RunStatePill from '../lib/components/RunStatePill.svelte'
  import Sheet from '../lib/components/Sheet.svelte'

  let { id, onclose }: { id: number; onclose: () => void } = $props()

  const data = new Load<{ run: Run; checks: Check[]; failures: RunFailure[] }>()
  const load = (quiet = false) => data.run(() => api.run(id), { quiet })
  load()

  $effect(() => runWatch.watch())
  // A going run reads again with every answer of the watch, so its checks
  // come in while you watch, and once more when it ends.
  $effect(() => {
    void runWatch.tick
    void runWatch.ended
    if (data.data && (data.data.run.state === 'going' || runWatch.ended?.id === id)) load(true)
  })

  // The run as every screen shows it: from the watch while it goes or just
  // ended, from this sheet's own answer otherwise.
  let run = $derived(runWatch.run?.id === id ? runWatch.run : runWatch.ended?.id === id ? runWatch.ended : data.data?.run)

  let show = $state<'all' | 'errors' | 'found'>('all')
  let checks = $derived(data.data?.checks ?? [])
  let shown = $derived(
    show === 'errors' ? checks.filter((c) => c.error) : show === 'found' ? checks.filter((c) => c.events_found > 0) : checks,
  )
  let options = $derived([
    { value: 'all' as const, label: 'All', count: checks.length },
    { value: 'errors' as const, label: 'Errors', count: checks.filter((c) => c.error).length },
    { value: 'found' as const, label: 'Found events', count: checks.filter((c) => c.events_found > 0).length },
  ])

  let failures = $derived(data.data?.failures ?? [])

  // What a click on a failure did, per source, so it can be undone.
  let done = $state<Record<number, { what: 'retired' | 'checking'; before?: SourceStatus }>>({})
  let working = $state<number | null>(null)

  async function retire(f: RunFailure) {
    const src = f.source!
    const before = done[src.id]?.before ?? src.status
    working = src.id
    done[src.id] = { what: 'retired', before }
    try {
      await api.patchSource(src.id, { status: 'retired' })
      toast.show(`${src.name} retired`)
    } catch (e) {
      delete done[src.id]
      toast.show(errorText(e), 'bad')
    } finally {
      working = null
    }
  }

  async function undo(f: RunFailure) {
    const src = f.source!
    const state = done[src.id]
    if (!state?.before) return
    working = src.id
    delete done[src.id]
    try {
      await api.patchSource(src.id, { status: state.before })
      toast.show(`${src.name} is ${STATUS_LABEL[state.before]?.toLowerCase() ?? state.before} again`)
    } catch (e) {
      done[src.id] = state
      toast.show(errorText(e), 'bad')
    } finally {
      working = null
    }
  }

  async function checkAgain(f: RunFailure) {
    const src = f.source!
    working = src.id
    done[src.id] = { what: 'checking' }
    try {
      await api.checkSource(src.id)
      toast.show(`Checking ${src.name} again. Runs shows how it went`)
    } catch (e) {
      delete done[src.id]
      toast.show(errorText(e), 'bad')
    } finally {
      working = null
    }
  }

  const WHAT = { read: 'Reading event pages', lookup: 'Looking up startups' }
</script>

<Sheet label="Run" {onclose}>
  {#snippet head()}
    {#if run}
      <div class="head">
        <div class="title-row">
          <h2>Run of {fmtDateTime(run.started_at)}</h2>
          <RunStatePill state={run.state} />
        </div>
        <RunOutcome {run} />
        {#if run.state === 'going' && run.progress}<RunProgress {run} />{/if}
      </div>
    {:else}
      <div class="head"><span class="skel" style:width="70%" style:height="20px"></span><span class="skel" style:width="90%" style:height="12px"></span></div>
    {/if}
  {/snippet}

  {#if data.error && !data.data}
    <EmptyState icon="alert" tone="bad" title="Could not load this run" text={data.error}>
      <button class="btn" type="button" onclick={() => load()}>Try again</button>
    </EmptyState>
  {:else if data.data}
    {#if failures.length}
      <section class="failures" aria-labelledby="failed-h">
        <h3 id="failed-h">What failed</h3>
        <ul>
          {#each failures as f, i (f.source ? `s${f.source.id}` : `${f.what}${i}`)}
            <li class="failure">
              <div class="f-text">
                {#if f.source}
                  <a class="f-name" href="/app/sources/{f.source.id}">{f.source.name}</a>
                {:else}
                  <b class="f-name">{WHAT[f.what as 'read' | 'lookup']}{f.count > 1 ? `, ${f.count} times` : ''}</b>
                {/if}
                <span class="f-reason" title={f.detail}>{f.reason}</span>
              </div>
              {#if f.source}
                {@const d = done[f.source.id]}
                {#if d?.what === 'retired'}
                  <button class="btn sm act" type="button" disabled={working === f.source.id} onclick={() => undo(f)} title="Set the source back to how it was">Undo</button>
                {:else if d?.what === 'checking'}
                  <span class="pill act">Checking</span>
                {:else if f.action === 'retire'}
                  <button class="btn sm act" type="button" disabled={working === f.source.id} onclick={() => retire(f)} title="Stop checking this source. Undo sets it back">Retire</button>
                {:else if f.action === 'check'}
                  <button class="btn sm act" type="button" disabled={working === f.source.id} onclick={() => checkAgain(f)} title="Check this source again now">Check again</button>
                {:else}
                  <span class="pill act" title={f.source.status === 'manual' ? 'The engine no longer checks it. Follow it by hand' : 'The engine retired it'}>{STATUS_LABEL[f.source.status] ?? f.source.status}</span>
                {/if}
              {/if}
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    <Chips label="Show checks" options={options} bind:value={show} />
    {#if !shown.length}
      <p class="muted">No checks in this group.</p>
    {/if}
    <ul class="checks">
      {#each shown as c (c.id)}
        <li class="check" class:failed={!!c.error}>
          <div class="c-top">
            <a class="c-name" href="/app/sources/{c.source.id}">{c.source.name}</a>
            {#if c.error}
              <span class="pill bad"><span class="dot"></span>{c.http_status ? `HTTP ${c.http_status}` : 'Failed'}</span>
            {:else}
              <span class="pill good"><span class="dot"></span>HTTP {c.http_status}</span>
            {/if}
          </div>
          {#if c.error}<p class="c-err">{c.error}</p>{/if}
          <p class="c-nums num">
            {c.events_found} found · {c.events_kept} kept · {c.people_found} people · {fmtDuration(c.duration_ms)} · {c.mode}
          </p>
          {#if c.fetch_id}
            <div class="row c-links">
              <a class="btn sm" href={fetchUrl(c.fetch_id, 'text')} target="_blank" rel="noopener"><Icon name="doc" size={14} />Text</a>
              <a class="btn sm" href={fetchUrl(c.fetch_id, 'html')} target="_blank" rel="noopener"><Icon name="code" size={14} />HTML</a>
              <a class="btn sm" href={fetchUrl(c.fetch_id, 'screenshot')} target="_blank" rel="noopener"><Icon name="image" size={14} />Screenshot</a>
            </div>
          {/if}
        </li>
      {/each}
    </ul>
    <p class="note">Stored pages are deleted after 30 days.</p>
  {:else}
    <div class="loading">{#each Array(5) as _, i (i)}<span class="skel" style:height="64px"></span>{/each}</div>
  {/if}
</Sheet>

<style>
  .head { display: grid; gap: 6px; }
  .head h2 { font-size: 19px; }
  .title-row { display: flex; align-items: center; justify-content: space-between; gap: 10px; min-width: 0; }
  .failures { display: grid; gap: 8px; }
  .failures h3 { font-size: 14px; font-weight: 600; }
  .failures ul { display: grid; gap: 8px; }
  .failure {
    display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 10px; align-items: center;
    padding: 10px 12px; border: 1px solid color-mix(in srgb, var(--bad) 45%, var(--line)); border-radius: 8px;
  }
  .f-text { display: grid; gap: 2px; min-width: 0; }
  .f-name { font-weight: 600; min-width: 0; overflow-wrap: anywhere; }
  .f-reason { font-size: 13px; color: var(--bad); }
  /* Every action in the list has the width of the longest, Check again. */
  .act { min-width: 7.6em; justify-content: center; }
  .pill.act { height: 30px; border-radius: 8px; }
  .checks { display: grid; gap: 8px; }
  .check { display: grid; gap: 4px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; min-width: 0; }
  .check.failed { border-color: color-mix(in srgb, var(--bad) 45%, var(--line)); }
  .c-top { display: flex; justify-content: space-between; align-items: center; gap: 10px; min-width: 0; }
  .c-name { font-weight: 600; min-width: 0; overflow-wrap: anywhere; }
  .c-err { font-size: 13px; color: var(--bad); }
  .c-nums { font-size: 12px; color: var(--ink-2); }
  .c-links { margin-top: 4px; }
  .loading { display: grid; gap: 8px; }
</style>
