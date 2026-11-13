<script lang="ts">
  import { untrack } from 'svelte'
  import { api, type PeopleFilter, type PeopleSort, type Person, type PersonDetail as Detail } from '../lib/api'
  import { plural } from '../lib/format'
  import { Load } from '../lib/load.svelte'
  import { navigate, router } from '../lib/router.svelte'
  import { runShare, runWatch } from '../lib/run.svelte'
  import Busy from '../lib/components/Busy.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import RunOutcome from '../lib/components/RunOutcome.svelte'
  import RunProgress from '../lib/components/RunProgress.svelte'
  import SearchInput from '../lib/components/SearchInput.svelte'
  import Skeleton from '../lib/components/Skeleton.svelte'
  import PersonDetail from './PersonDetail.svelte'
  import PersonSheet from './PersonSheet.svelte'
  import RunSheet from './RunSheet.svelte'

  let q = $state('')
  let query = $state('')
  // The likely youngest come first, unless Tim chose another order on this
  // device.
  const SORTS: PeopleSort[] = ['young', 'new', 'fit', 'name']
  let sort = $state<PeopleSort>(savedSort())
  let filter = $state<PeopleFilter>('all')

  function savedSort(): PeopleSort {
    try {
      const v = localStorage.getItem('people-sort') as PeopleSort | null
      return v && SORTS.includes(v) ? v : 'young'
    } catch {
      return 'young'
    }
  }
  $effect(() => {
    try {
      localStorage.setItem('people-sort', sort)
    } catch {
      // Private mode. The order is forgotten, which is fine.
    }
  })
  let reload = $state(0)

  const people = new Load<Person[]>()
  // How many each filter shows, so an empty filter never hides the rest.
  let counts = $state<Record<PeopleFilter, number> | null>(null)

  $effect(() => {
    document.title = 'People · Speaker Trail'
  })

  const fetchPeople = (params: { q: string; sort: PeopleSort; filter: PeopleFilter }) => async () => {
    const r = await api.people(params)
    counts = r.counts
    return r.people
  }

  $effect(() => {
    const params = { q: query, sort, filter }
    void reload
    people.run(fetchPeople(params))
  })

  // While a run goes, new people come in without a refresh: the list reads
  // again every few seconds, and once more when the run ends.
  $effect(() => runWatch.watch())
  let lastLive = 0
  $effect(() => {
    void runWatch.tick
    void runWatch.ended
    if (!runWatch.run && !runWatch.ended) return
    const t = Date.now()
    if (runWatch.run && t - lastLive < 4000) return
    lastLive = t
    untrack(() => people.run(fetchPeople({ q: query, sort, filter }), { quiet: true }))
  })

  // People the going run found, or the one that just ended, are marked new.
  let since = $derived.by(() => {
    const at = (runWatch.run ?? runWatch.ended)?.started_at
    return at ? new Date(at).getTime() : null
  })
  let isNew = (p: Person) => since !== null && new Date(p.first_seen).getTime() >= since

  let openId = $derived.by(() => {
    const m = router.match('/app/people/:id')
    return m ? Number(m.id) : null
  })

  let filters = $derived(
    ([
      { value: 'all', label: 'All' },
      { value: 'young', label: 'Under 30' },
      { value: 'connected', label: 'Connected' },
    ] as { value: PeopleFilter; label: string }[]).map((f) => ({ ...f, count: counts?.[f.value] })),
  )

  // The run is started and stopped here, where its people show up.
  let going = $derived(runWatch.run)
  let runLabel = $derived(
    runWatch.busy === 'starting' ? 'Starting' : runWatch.busy === 'stopping' ? 'Stopping' : going ? 'Stop' : 'Run',
  )
  let runOpen = $state<number | null>(null)

  const year = new Date().getFullYear()
  const age = (p: Person) => (p.linkedin?.born_about ? year - p.linkedin.born_about : null)

  // Wide enough for the list and the person beside it, like Mail. Below
  // that the person slides in over the list.
  let wide = $state(false)
  $effect(() => {
    const mq = window.matchMedia('(min-width: 1000px)')
    wide = mq.matches
    const on = () => (wide = mq.matches)
    mq.addEventListener('change', on)
    return () => mq.removeEventListener('change', on)
  })

  // The arrow keys go up and down the list and Escape puts the person away,
  // the way Mail and Finder do, unless a field has the keyboard.
  function keydown(e: KeyboardEvent) {
    if (e.defaultPrevented || e.metaKey || e.ctrlKey || e.altKey) return
    const t = e.target as HTMLElement | null
    if (t && (t.closest('input, textarea, select, [contenteditable]') || t.closest('[role="dialog"]'))) return
    if (e.key === 'Escape' && wide && openId !== null) {
      e.preventDefault()
      navigate('/app/people')
      return
    }
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
    const list = people.data
    if (!list?.length) return
    e.preventDefault()
    const at = list.findIndex((p) => p.id === openId)
    const next = at < 0 ? 0 : Math.max(0, Math.min(list.length - 1, at + (e.key === 'ArrowDown' ? 1 : -1)))
    navigate(`/app/people/${list[next].id}`, { replace: at >= 0 })
    requestAnimationFrame(() => document.getElementById(`person-row-${list[next].id}`)?.scrollIntoView({ block: 'nearest' }))
  }

  let others = $derived(counts ? counts.all : 0)

  // Keep the list in step with changes made in the sheet.
  function updated(p: Detail) {
    if (!people.data) return
    people.data = people.data.map((x) => (x.id === p.id ? { ...x, profiles: p.profiles, headline: p.headline, connected: p.connected } : x))
  }
</script>

<svelte:window onkeydown={keydown} />

<div class="people" class:wide>
  <section class="list-pane" aria-label="People">
    <header class="bar">
      <h1>People{#if counts}<span class="num-count">{counts.all}</span>{/if}</h1>
      <button class="btn start" class:primary={!going} type="button" onclick={() => (going ? runWatch.stop() : runWatch.start())}
        disabled={runWatch.busy !== '' || (going !== null && going.id <= 0)}
        title={going ? 'Stop this run. What is running finishes, the rest waits for the next run' : 'Run the due searches now. Takes about a minute'}>
        {#if going}<Busy fraction={runShare(going)} />{/if}
        <Icon name={going ? 'stop' : 'run'} />{runLabel}
      </button>
    </header>

    <div class="controls">
      <div class="toolbar tight">
        <SearchInput bind:value={q} placeholder="Search people" label="Search people" onsearch={(v) => (query = v)} />
        <label class="sort" title="Order">
          <span class="sr-only">Sort by</span>
          <select class="select" bind:value={sort}>
            <option value="young">Youngest</option>
            <option value="new">Newest</option>
            <option value="fit">Best fit</option>
            <option value="name">Name</option>
          </select>
        </label>
      </div>
      <div class="segmented filters" role="group" aria-label="Show">
        {#each filters as f (f.value)}
          <button type="button" aria-pressed={filter === f.value} onclick={() => (filter = f.value)}>
            {f.label}{#if f.count !== undefined}<span class="n">{f.count}</span>{/if}
          </button>
        {/each}
      </div>
      {#if runWatch.run}
        <button class="run-now" type="button" disabled={runWatch.run.id <= 0} onclick={() => (runOpen = runWatch.run?.id ?? null)}>
          <RunProgress run={runWatch.run} />
        </button>
      {:else if runWatch.ended}
        <button class="run-now" type="button" onclick={() => (runOpen = runWatch.ended?.id ?? null)}>
          <RunOutcome run={runWatch.ended} />
        </button>
      {/if}
    </div>

    <div class="rows-wrap">
      {#if people.error && !people.data}
        <EmptyState icon="alert" tone="bad" title="Could not load people" text={people.error}>
          <button class="btn" type="button" onclick={() => reload++}><Icon name="refresh" />Try again</button>
        </EmptyState>
      {:else if !people.data}
        <Skeleton count={7} />
      {:else if !people.data.length}
        <EmptyState
          icon="people"
          title={filter === 'young' && !query ? 'Nobody under 30 yet' : filter === 'connected' && !query ? 'Nobody connected yet' : 'Nobody here yet'}
          text={filter !== 'all' && others > 0
            ? `${plural(others, 'person', 'people')} found so far, none of them ${filter === 'young' ? 'likely under 30 yet' : filter === 'connected' ? 'connected yet' : 'in this filter'}.`
            : query ? 'Nobody matches this search.' : 'Press Run. Each search asks for LinkedIn profiles and keeps the people who fit.'}>
          {#if query || filter !== 'all'}
            <button class="btn" type="button" onclick={() => ((q = ''), (query = ''), (filter = 'all'))}>Show everyone</button>
          {/if}
        </EmptyState>
      {:else}
        <ul class="rows" class:stale={people.loading} aria-label="{plural(people.data.length, 'person', 'people')}">
          {#each people.data as p (p.id)}
            <li>
              <a class="prow" id="person-row-{p.id}" href="/app/people/{p.id}" aria-current={openId === p.id ? 'true' : undefined}>
                <span class="line1">
                  <b class="ellipsis name">{p.name}</b>
                  {#if isNew(p)}<span class="dot new" title="Found by this run"></span><span class="sr-only">, new</span>{/if}
                  {#if p.connected}<span class="ok" title="Connected"><Icon name="check" size={14} line={2} /></span><span class="sr-only">, connected</span>{/if}
                  {#if age(p)}<span class="age" title="Age, a guess from the LinkedIn profile">{age(p)}</span>{/if}
                </span>
                <span class="ellipsis sub">{p.headline || p.city || 'No headline yet'}</span>
              </a>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </section>

  {#if wide}
    <section class="detail-pane" aria-label="Person">
      <header class="bar">
        <span class="grow"></span>
        {#if openId !== null}
          <button class="btn quiet icon flush-end" type="button" onclick={() => navigate('/app/people')} aria-label="Close" title="Close (Esc)"><Icon name="close" /></button>
        {/if}
      </header>
      <div class="detail-body">
        {#if openId !== null}
          {#key openId}
            <PersonDetail id={openId} onupdate={updated} />
          {/key}
        {:else}
          <div class="nobody">
            <Icon name="people" size={28} line={1.5} />
            <p>{people.data?.length ? 'Pick someone to see why they fit' : 'Nobody to show yet'}</p>
          </div>
        {/if}
      </div>
    </section>
  {/if}
</div>

{#if openId !== null && !wide}
  {#key openId}
    <PersonSheet id={openId} name={people.data?.find((x) => x.id === openId)?.name} onclose={() => navigate('/app/people')} onupdate={updated} />
  {/key}
{:else if runOpen !== null}
  {#key runOpen}
    <RunSheet id={runOpen} onclose={() => (runOpen = null)} />
  {/key}
{/if}

<style>
  /* The list on the left and the person on the right, each a pane of its
     own that scrolls by itself, the way Mail is laid out. */
  .people { display: grid; grid-template-columns: minmax(0, 1fr); height: 100dvh; min-width: 0; }
  .people.wide { grid-template-columns: var(--list-w) minmax(0, 1fr); }
  .list-pane, .detail-pane { min-width: 0; height: 100dvh; overflow-y: auto; overscroll-behavior: contain; display: flex; flex-direction: column; }
  .detail-pane { border-left: 1px solid var(--line); }
  /* Where everything in the list pane starts and ends. In a window too
     narrow for the person beside it, the list reads in the same centred
     column as the other pages. */
  .list-pane { --x: var(--pad); }
  .people:not(.wide) .list-pane { --x: max(var(--pad), calc((100% - var(--col-w)) / 2)); }
  .list-pane > .bar { position: sticky; top: 0; z-index: 2; background: var(--surface); padding-inline: var(--x); }
  @media (max-width: 760px) { .list-pane > .bar { padding-left: var(--lights-r); } }
  .controls { display: grid; gap: 10px; padding: 14px var(--x) 10px; }
  .filters { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); width: 100%; }
  .filters .n { margin-left: 6px; color: var(--ink-3); font-weight: 500; font-variant-numeric: tabular-nums; }
  .sort { flex: none; }
  /* Room for the longest label, Starting or Stopping, so the button keeps its width. */
  .start { min-width: 7.4em; }
  .run-now {
    display: block; width: 100%; text-align: left; font: inherit; color: var(--ink); padding: 12px 14px; cursor: pointer;
    background: var(--surface-2); border: 0; border-radius: var(--radius);
  }
  .run-now:hover { background: color-mix(in srgb, var(--ink) 8%, var(--surface-2)); }
  .rows-wrap { padding: 0 calc(var(--x) - 12px) 24px; display: grid; }
  .rows-wrap > :global(.empty) { margin: 8px 12px; }
  .rows { display: grid; gap: 2px; }
  .stale { opacity: .6; transition: opacity .2s; }
  /* A row's words stand where the bar's words stand: the row reaches 12
     px past them on either side for its highlight. */
  .prow {
    display: grid; gap: 2px; padding: 9px 12px; border-radius: 8px; color: inherit; text-decoration: none; min-width: 0;
  }
  .prow:hover { background: color-mix(in srgb, var(--ink) 5%, transparent); text-decoration: none; }
  .prow[aria-current="true"] { background: var(--accent-soft); }
  .prow:focus-visible { outline-offset: -2px; }
  .line1 { display: flex; align-items: center; gap: 6px; min-width: 0; }
  .name { font-weight: 600; font-size: 14px; }
  .dot.new { width: 7px; height: 7px; background: var(--accent); }
  .ok { color: var(--good); display: inline-flex; }
  .age { margin-left: auto; padding-left: 8px; font-size: 13px; color: var(--ink-3); font-variant-numeric: tabular-nums; flex: none; }
  .sub { font-size: 13px; color: var(--ink-2); }
  .prow[aria-current="true"] .sub, .prow[aria-current="true"] .age { color: var(--ink-2); }
  .grow { flex: 1 1 auto; }
  /* The person stands in a column of its own width, and the cross in the
     bar over it ends where the column ends. */
  .detail-pane > .bar { padding-inline: var(--dx); }
  .detail-pane { --dx: max(32px, calc((100% - 640px) / 2)); }
  .detail-body { padding: 28px var(--dx) 48px; min-width: 0; }
  .nobody { display: grid; justify-items: center; gap: 10px; padding-top: 18vh; color: var(--ink-3); font-size: 14px; }
</style>
