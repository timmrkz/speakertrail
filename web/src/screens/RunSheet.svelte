<script lang="ts">
  import { api, fetchUrl, type Check, type Run } from '../lib/api'
  import { fmtDateTime, fmtDuration, fmtNum } from '../lib/format'
  import { Load } from '../lib/load.svelte'
  import Chips from '../lib/components/Chips.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import Sheet from '../lib/components/Sheet.svelte'

  let { id, onclose }: { id: number; onclose: () => void } = $props()

  const data = new Load<{ run: Run; checks: Check[] }>()
  const load = () => data.run(() => api.run(id))
  load()

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
</script>

<Sheet label="Run" {onclose}>
  {#snippet head()}
    {#if data.data}
      <div class="head">
        <h2>Run of {fmtDateTime(data.data.run.started_at)}</h2>
        <p class="muted small">
          {fmtNum(data.data.run.sources_checked)} sources checked, {fmtNum(data.data.run.events_found)} events found,
          {fmtNum(data.data.run.events_new)} new, {fmtNum(data.data.run.pages_read)} event pages read, {fmtNum(data.data.run.people_new)} new people,
          {fmtNum(data.data.run.errors)} errors
        </p>
      </div>
    {:else}
      <div class="head"><span class="skel" style:width="70%" style:height="20px"></span><span class="skel" style:width="90%" style:height="12px"></span></div>
    {/if}
  {/snippet}

  {#if data.error && !data.data}
    <EmptyState icon="alert" tone="bad" title="Could not load this run" text={data.error}>
      <button class="btn" type="button" onclick={load}>Try again</button>
    </EmptyState>
  {:else if data.data}
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
  .head { display: grid; gap: 4px; }
  .head h2 { font-size: 19px; }
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
