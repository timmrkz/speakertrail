<script lang="ts">
  import { cubicOut } from 'svelte/easing'
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
  const reduced = typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
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

  // The person beside the list takes a share of the window that Tim sets
  // by dragging its edge. It snaps to steps of 5 %, so it lands on 40 %
  // or 50 % rather than anywhere near them. Each pane keeps room for its
  // words, so the steps on offer depend on how wide the window is. Closed,
  // the person takes nothing and the list has the whole width.
  const STEP = 0.05
  const SHARE_KEY = 'people.detail-share'
  let width = $state(0)
  let share = $state(readShare())
  let dragging = $state(false)
  function readShare(): number {
    try {
      const v = Number(localStorage.getItem(SHARE_KEY))
      return v > 0 && v < 1 ? v : 0.45
    } catch {
      return 0.45
    }
  }
  // The smallest and largest share that leave the person 360 px and the
  // list 320 px, between a quarter and three quarters of the width.
  let least = $derived(Math.min(0.75, Math.ceil(Math.max(0.25, 360 / (width || 1)) / STEP - 1e-6) * STEP))
  let most = $derived(Math.max(least, Math.floor(Math.min(0.75, 1 - 320 / (width || 1)) / STEP + 1e-6) * STEP))
  let shown = $derived(Math.min(most, Math.max(least, share)))
  let percent = $derived(Math.round(shown * 100))

  function setShare(v: number) {
    share = Math.min(most, Math.max(least, Math.round(v / STEP) * STEP))
    try {
      localStorage.setItem(SHARE_KEY, String(share))
    } catch {
      // A private window keeps the width for this visit only.
    }
  }

  // The person grows in from the right edge and shrinks back into it. Its
  // words keep their final width meanwhile, so they slide rather than
  // wrap anew on every frame.
  function grow(node: HTMLElement) {
    const w = node.getBoundingClientRect().width
    return { duration: reduced ? 0 : 220, easing: cubicOut, css: (t: number) => `flex-basis: ${t * w}px; overflow: hidden; --pane-w: ${w}px` }
  }

  let peopleEl = $state<HTMLElement>()
  function startDrag(e: PointerEvent) {
    if (e.button !== 0) return
    e.preventDefault()
    const edge = e.currentTarget as HTMLElement
    edge.setPointerCapture(e.pointerId)
    dragging = true
    const move = (ev: PointerEvent) => {
      const r = peopleEl?.getBoundingClientRect()
      if (r) setShare((r.right - ev.clientX) / r.width)
    }
    const end = () => {
      dragging = false
      edge.removeEventListener('pointermove', move)
      edge.removeEventListener('pointerup', end)
      edge.removeEventListener('pointercancel', end)
    }
    edge.addEventListener('pointermove', move)
    edge.addEventListener('pointerup', end)
    edge.addEventListener('pointercancel', end)
  }

  // The edge answers the keyboard too: the arrows move it a step, Home and
  // End to the ends.
  function edgeKey(e: KeyboardEvent) {
    const to = { ArrowLeft: shown + STEP, ArrowRight: shown - STEP, Home: most, End: least }[e.key]
    if (to === undefined) return
    e.preventDefault()
    setShare(to)
  }

  let others = $derived(counts ? counts.all : 0)

  // Keep the list in step with changes made in the sheet.
  function updated(p: Detail) {
    if (!people.data) return
    people.data = people.data.map((x) => (x.id === p.id ? { ...x, profiles: p.profiles, headline: p.headline, connected: p.connected } : x))
  }
</script>

<svelte:window onkeydown={keydown} />

<div class="people" class:wide class:open={wide && openId !== null} class:dragging bind:this={peopleEl} bind:clientWidth={width}
  style:--detail-w="{percent}%">
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
                <span class="who">
                  <b class="ellipsis name">{p.name}</b>
                  {#if isNew(p)}<span class="dot new" title="Found by this run"></span><span class="sr-only">, new</span>{/if}
                  {#if p.connected}<span class="ok" title="Connected"><Icon name="check" size={14} line={2} /></span><span class="sr-only">, connected</span>{/if}
                </span>
                <span class="ellipsis sub">{p.headline || p.city || 'No headline yet'}</span>
                <span class="ellipsis city">{p.headline ? p.city : ''}</span>
                <span class="age" title={age(p) ? 'Age, a guess from the LinkedIn profile' : undefined}>{age(p) ?? ''}</span>
              </a>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </section>

  {#if wide && openId !== null}
    <section class="detail-pane" aria-label="Person" transition:grow>
      <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
      <div class="edge" role="separator" aria-orientation="vertical" aria-label="Width of the person"
        aria-valuemin={Math.round(least * 100)} aria-valuemax={Math.round(most * 100)} aria-valuenow={percent} aria-valuetext="{percent} % of the window"
        tabindex="0" title="Drag to make the person wider or narrower. It snaps to steps of 5 %. Double-click for 45 %"
        onpointerdown={startDrag} onkeydown={edgeKey} ondblclick={() => setShare(0.45)}>
        {#if dragging}<span class="share">{percent} %</span>{/if}
      </div>
      <header class="bar">
        <span class="grow"></span>
        <button class="btn quiet icon flush-end" type="button" onclick={() => navigate('/app/people')} aria-label="Close" title="Close (Esc)"><Icon name="close" /></button>
      </header>
      <div class="detail-body">
        {#key openId}
          <PersonDetail id={openId} onupdate={updated} />
        {/key}
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
  .people { display: flex; height: 100dvh; min-width: 0; }
  .list-pane, .detail-pane { min-width: 0; height: 100dvh; overflow-y: auto; overscroll-behavior: contain; display: flex; flex-direction: column; }
  .list-pane { flex: 1 1 0; container-type: inline-size; }
  /* The person comes in from the right and takes its share of the width.
     Closed, it is not there at all. */
  /* Its body scrolls, not the pane, so the edge can reach out over the
     list a little and still be caught. */
  .detail-pane { flex: 0 0 var(--detail-w); position: relative; overflow: visible; border-left: 1px solid var(--line); background: var(--surface); }
  .detail-body { flex: 1 1 auto; overflow-y: auto; overscroll-behavior: contain; }
  .detail-pane > .bar, .detail-body { min-width: var(--pane-w, 0); }
  .dragging, .dragging * { cursor: col-resize !important; user-select: none; }
  /* The edge to drag: wider than the line it sits on, so it is easy to
     catch, and lit while it is held or has the keyboard. */
  .edge {
    position: absolute; z-index: 3; top: 0; bottom: 0; left: -5px; width: 9px; cursor: col-resize; touch-action: none;
    --wails-draggable: no-drag;
  }
  .edge::after {
    content: ""; position: absolute; top: 0; bottom: 0; left: 3px; width: 3px; border-radius: 2px; background: transparent;
    transition: background .15s;
  }
  .edge:hover::after { background: var(--line-strong); }
  .dragging .edge::after, .edge:focus-visible::after { background: var(--accent); }
  .edge:focus-visible { outline: none; }
  .share {
    position: absolute; top: calc(var(--bar-h) + 10px); left: 14px; padding: 3px 8px; border-radius: 6px; white-space: nowrap;
    background: var(--accent); color: var(--accent-ink); font-size: 12px; font-weight: 600; font-variant-numeric: tabular-nums;
  }
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
  /* A row is two lines in a narrow list: name and age, then the
     headline. With room it is one line of columns, name, headline, city
     and age, so a wide list uses its width rather than leaving it empty. */
  .prow {
    display: grid; grid-template-columns: minmax(0, 1fr) auto; grid-template-areas: "who age" "sub sub";
    gap: 2px 8px; padding: 9px 12px; border-radius: 8px; color: inherit; text-decoration: none; min-width: 0; align-items: center;
  }
  .who { grid-area: who; } .sub { grid-area: sub; } .age { grid-area: age; } .city { display: none; }
  @container (min-width: 760px) {
    .prow { grid-template-columns: minmax(0, 2fr) minmax(0, 4fr) minmax(0, 1.2fr) 2.5em; grid-template-areas: "who sub city age"; gap: 16px; }
    .city { display: block; grid-area: city; font-size: 13px; color: var(--ink-3); }
    .controls { grid-template-columns: minmax(0, 1fr) auto; align-items: center; }
    .controls .filters { width: auto; grid-auto-columns: minmax(7.5em, auto); grid-template-columns: none; grid-auto-flow: column; }
    .controls .run-now { grid-column: 1 / -1; }
  }
  .prow:hover { background: color-mix(in srgb, var(--ink) 5%, transparent); text-decoration: none; }
  .prow[aria-current="true"] { background: var(--accent-soft); }
  .prow:focus-visible { outline-offset: -2px; }
  .who { display: flex; align-items: center; gap: 6px; min-width: 0; }
  .name { font-weight: 600; font-size: 14px; }
  .dot.new { width: 7px; height: 7px; background: var(--accent); }
  .ok { color: var(--good); display: inline-flex; }
  .age { text-align: right; font-size: 13px; color: var(--ink-3); font-variant-numeric: tabular-nums; flex: none; }
  .sub { font-size: 13px; color: var(--ink-2); }
  .prow[aria-current="true"] .sub, .prow[aria-current="true"] .age { color: var(--ink-2); }
  .grow { flex: 1 1 auto; }
  /* The person stands in a column of its own width, and the cross in the
     bar over it ends where the column ends. */
  .detail-pane > .bar { padding-inline: var(--dx); }
  .detail-pane { --dx: max(32px, calc((100% - 640px) / 2)); }
  .detail-body { padding: 28px var(--dx) 48px; }
</style>
