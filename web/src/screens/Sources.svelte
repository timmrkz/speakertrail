<script lang="ts">
  import { api, SOURCE_STATUSES, type Source, type SourceStatus } from '../lib/api'
  import { fmtAgo, KIND_LABEL, plural, shortUrl, STATUS_LABEL } from '../lib/format'
  import { Load } from '../lib/load.svelte'
  import { navigate, router } from '../lib/router.svelte'
  import { errorText, toast } from '../lib/toast.svelte'
  import Chips from '../lib/components/Chips.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import HealthPill from '../lib/components/HealthPill.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import SearchInput from '../lib/components/SearchInput.svelte'
  import Skeleton from '../lib/components/Skeleton.svelte'
  import SourceSheet from './SourceSheet.svelte'

  let q = $state('')
  let query = $state('')
  let status = $state<SourceStatus | ''>('')
  let reload = $state(0)
  let adding = $state(false)
  let newUrl = $state('')
  let newName = $state('')
  let addError = $state('')
  let addBusy = $state(false)
  let urlField: HTMLInputElement | undefined = $state()

  const sources = new Load<Source[]>()

  $effect(() => {
    document.title = 'Sources · Speaker Trail'
  })

  $effect(() => {
    const params = { q: query }
    void reload
    sources.run(() => api.sources(params))
  })

  // Status filters locally so each chip can show its count.
  let all = $derived(sources.data ?? [])
  let statusOptions = $derived([
    { value: '' as const, label: 'All', count: all.length },
    ...SOURCE_STATUSES.map((s) => ({ value: s, label: STATUS_LABEL[s], count: all.filter((x) => x.status === s).length })),
  ])
  let shown = $derived(status ? all.filter((s) => s.status === status) : all)

  let openId = $derived.by(() => {
    const m = router.match('/app/sources/:id')
    return m ? Number(m.id) : null
  })
  let open = $derived(openId === null ? undefined : all.find((s) => s.id === openId))

  // A deep link to a source outside the current search loads it on its own.
  let extra = $state<Source | undefined>()
  $effect(() => {
    if (openId === null || open || !sources.data || extra?.id === openId) return
    const want = openId
    api.sources().then((list) => (extra = list.find((s) => s.id === want)))
  })
  let sheetSource = $derived(open ?? (extra?.id === openId ? extra : undefined))

  function replace(s: Source) {
    if (sources.data) sources.data = sources.data.map((x) => (x.id === s.id ? s : x))
    if (extra?.id === s.id) extra = s
  }

  const STATUS_TONE: Record<SourceStatus, string> = { active: 'good', probation: 'accent', candidate: '', manual: 'accent', retired: '' }

  async function add(e: SubmitEvent) {
    e.preventDefault()
    const url = newUrl.trim()
    if (!/^https?:\/\/\S+\.\S+/.test(url)) {
      addError = 'Enter a full web address, starting with https://'
      urlField?.focus()
      return
    }
    addBusy = true
    addError = ''
    try {
      const s = await api.addSource(url, newName.trim() || undefined)
      sources.data = [s, ...(sources.data ?? [])]
      newUrl = ''
      newName = ''
      adding = false
      toast.show(`Added ${s.name} as a candidate`)
      navigate(`/app/sources/${s.id}`)
    } catch (err) {
      addError = errorText(err)
    } finally {
      addBusy = false
    }
  }
</script>

<div class="view">
  <div class="view-head">
    <div>
      <h1>Sources</h1>
      <p>The pages the crawler checks every night, and how they are doing.</p>
    </div>
    <button class="btn primary" type="button" aria-expanded={adding} onclick={() => ((adding = !adding), setTimeout(() => urlField?.focus()))}>
      <Icon name="plus" />Add source
    </button>
  </div>

  {#if adding}
    <form class="panel add" onsubmit={add} novalidate>
      <h2>Add a source</h2>
      <div class="add-grid">
        <div class="field">
          <label for="src-url">Web address</label>
          <input bind:this={urlField} bind:value={newUrl} id="src-url" class="input" type="url" inputmode="url" placeholder="https://" autocomplete="off"
            aria-invalid={addError ? 'true' : undefined} aria-describedby={addError ? 'src-err' : 'src-hint'} />
        </div>
        <div class="field">
          <label for="src-name">Name <span class="faint">(optional)</span></label>
          <input bind:value={newName} id="src-name" class="input" type="text" placeholder="Taken from the page if empty" autocomplete="off" />
        </div>
      </div>
      {#if addError}<p id="src-err" class="field-error" role="alert">{addError}</p>{:else}<p id="src-hint" class="field-hint">A link to one event adds the calendar it belongs to. It gets checked in the next run.</p>{/if}
      <div class="row end">
        <button class="btn" type="button" onclick={() => ((adding = false), (addError = ''))}>Cancel</button>
        <button class="btn primary" type="submit" disabled={addBusy}>{addBusy ? 'Adding' : 'Add source'}</button>
      </div>
    </form>
  {/if}

  <div class="filters">
    <SearchInput bind:value={q} placeholder="Search name, address, city" label="Search sources" onsearch={(v) => (query = v)} />
    <Chips label="Status" options={statusOptions} bind:value={status} />
  </div>

  {#if sources.error && !sources.data}
    <EmptyState icon="alert" tone="bad" title="Could not load the sources" text={sources.error}>
      <button class="btn" type="button" onclick={() => reload++}><Icon name="refresh" />Try again</button>
    </EmptyState>
  {:else if !sources.data}
    <Skeleton count={7} />
  {:else if !shown.length}
    <EmptyState icon="sources" title="No sources here" text={query ? 'Nothing matches this search.' : `No ${status ? STATUS_LABEL[status].toLowerCase() : ''} sources right now.`}>
      {#if query || status}<button class="btn" type="button" onclick={() => ((q = ''), (query = ''), (status = ''))}>Show all sources</button>{/if}
    </EmptyState>
  {:else}
    <p class="summary">{plural(shown.length, 'source')}{sources.loading ? ', updating' : ''}</p>
    <ul class="list" class:stale={sources.loading}>
      {#each shown as s (s.id)}
        <li>
          <a class="row-btn source" href="/app/sources/{s.id}" aria-current={openId === s.id ? 'true' : undefined}>
            <span class="c-name">
              <b class="ellipsis">{s.name || shortUrl(s.url) || s.query}</b>
              <span class="ellipsis sub">{[KIND_LABEL[s.kind] ?? s.kind, s.city, s.url ? shortUrl(s.url) : s.query].filter(Boolean).join(' · ')}</span>
            </span>
            <span class="c-status"><span class="pill {STATUS_TONE[s.status]}">{STATUS_LABEL[s.status]}</span><span class="pts"><span class="num">{s.points}</span> pts</span></span>
            <span class="c-health">
              <HealthPill health={s.health} />
              {#if s.health_note}<span class="ellipsis note-text" title={s.health_note}>{s.health_note}</span>{/if}
            </span>
            <span class="c-check">
              <span>{s.last_checked_at ? `Checked ${fmtAgo(s.last_checked_at)}` : 'Not checked yet'}</span>
              {#if s.last_check}<span class="found">{plural(s.last_check.events_found, 'event')} via {s.last_check.mode}</span>{/if}
            </span>
          </a>
        </li>
      {/each}
    </ul>
  {/if}
</div>

{#if openId !== null}
  {#key openId}
    <SourceSheet source={sheetSource} onclose={() => navigate('/app/sources')} onupdate={replace} />
  {/key}
{/if}

<style>
  .filters { display: grid; gap: 10px; }
  .summary { font-size: 13px; color: var(--ink-2); margin-bottom: -10px; }
  .stale { opacity: .6; }
  .add-grid { display: grid; grid-template-columns: minmax(0, 2fr) minmax(0, 1fr); gap: 12px; }
  @media (max-width: 640px) { .add-grid { grid-template-columns: 1fr; } }
  /* Phones stack the row. Wide screens lay it out in columns. */
  .source {
    grid-template-columns: minmax(0, 1fr) auto;
    grid-template-areas: "name status" "health health" "check check";
    gap: 6px 12px;
  }
  .c-name { grid-area: name; display: grid; gap: 1px; min-width: 0; }
  .c-name b { font-weight: 600; }
  .c-status { grid-area: status; align-self: start; display: grid; justify-items: end; gap: 2px; }
  .pts { font-size: 12px; color: var(--ink-3); }
  .c-health { grid-area: health; display: flex; gap: 8px; align-items: center; min-width: 0; font-size: 13px; }
  .c-check { grid-area: check; font-size: 12px; color: var(--ink-3); font-variant-numeric: tabular-nums; display: flex; flex-wrap: wrap; gap: 0 6px; }
  .c-check .found::before { content: "· "; }
  .pts .num { font-size: 12px; }
  .sub { font-size: 13px; color: var(--ink-2); }
  .note-text { color: var(--ink-2); }
  @media (min-width: 1000px) {
    .source {
      grid-template-columns: minmax(0, 2fr) minmax(0, 1.4fr) minmax(0, 1.3fr) 96px;
      grid-template-areas: "name health check status";
      align-items: center;
    }
    .c-status { align-self: center; justify-self: end; }
    .c-check { display: grid; }
    .c-check .found::before { content: none; }
    .c-health { flex-direction: column; align-items: flex-start; gap: 2px; }
    .c-health .note-text { max-width: 100%; font-size: 12px; }
  }
</style>
