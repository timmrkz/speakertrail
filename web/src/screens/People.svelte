<script lang="ts">
  import { cubicOut } from 'svelte/easing'
  import { untrack } from 'svelte'
  import { api, type LinksFound, type PeopleFilter, type PeopleSort, type Person, type PersonDetail as Detail } from '../lib/api'
  import { fmtLeft, plural } from '../lib/format'
  import { typing } from '../lib/keys'
  import { Load } from '../lib/load.svelte'
  import { isPage, PASTE_HINT, PASTE_LINE, pasting } from '../lib/paste.svelte'
  import { navigate, router } from '../lib/router.svelte'
  import { runCount, runSays, runShare, runWatch } from '../lib/run.svelte'
  import Busy from '../lib/components/Busy.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import Pick from '../lib/components/Pick.svelte'
  import SearchInput from '../lib/components/SearchInput.svelte'
  import Skeleton from '../lib/components/Skeleton.svelte'
  import PersonDetail from './PersonDetail.svelte'
  import PersonSheet from './PersonSheet.svelte'

  let q = $state('')
  let query = $state('')
  // The best fit first, unless Tim chose another order on this device.
  const SORTS: PeopleSort[] = ['fit', 'young', 'new', 'name']
  let sort = $state<PeopleSort>(savedSort())
  // The orders the list can be in, in the app's own list, see Pick.
  const sorts = [
    { value: 'fit', label: 'Best fit' },
    { value: 'young', label: 'Youngest' },
    { value: 'new', label: 'Newest' },
    { value: 'name', label: 'Name' },
  ]
  let filter = $state<PeopleFilter>('all')

  // A new name, so the order kept before the fit, Youngest by default,
  // gives way to Best fit once.
  const SORT_KEY = 'people-sort-fit'
  function savedSort(): PeopleSort {
    try {
      const v = localStorage.getItem(SORT_KEY) as PeopleSort | null
      return v && SORTS.includes(v) ? v : 'fit'
    } catch {
      return 'fit'
    }
  }
  $effect(() => {
    try {
      localStorage.setItem(SORT_KEY, sort)
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

  // What the search holds when it holds links to LinkedIn profiles, as
  // the app reads them: then the list shows whom they lead to who is here
  // already, and Run turns into Add.
  let links = $state<LinksFound | null>(null)

  const fetchPeople = (params: { q: string; sort: PeopleSort; filter: PeopleFilter }) => async () => {
    const r = await api.people(params)
    counts = r.counts
    // An answer to a search since changed says nothing of this one.
    if (params.q === query) links = r.links
    return r.people
  }

  $effect(() => {
    const params = { q: query, sort, filter }
    void reload
    people.run(fetchPeople(params))
  })

  // While a run goes, new people come in without a refresh: the list reads
  // again every few seconds, with every answer while a profile in it is
  // read, and once more when the run ends.
  $effect(() => runWatch.watch())
  let lastLive = 0
  let reading = $derived(!!people.data?.some((p) => p.read === 'reading' || p.read === 'waiting'))
  $effect(() => {
    void runWatch.tick
    void runWatch.ended
    if (!runWatch.run && !runWatch.ended) return
    const t = Date.now()
    if (runWatch.run && t - lastLive < (untrack(() => reading) ? 1500 : 4000)) return
    lastLive = t
    untrack(() => people.run(fetchPeople({ q: query, sort, filter }), { quiet: true }))
  })

  // What adding or a pasted profile changed, or Undo took back, shows at
  // once.
  // Both are read, so both are followed, and only a change after People
  // opened counts.
  const changedAt = runWatch.changed + pasting.changed
  $effect(() => {
    if (runWatch.changed + pasting.changed === changedAt) return
    untrack(() => people.run(fetchPeople({ q: query, sort, filter }), { quiet: true }))
  })

  // People just added from links show under New, newest on top, the
  // search emptied for them. They may have been dropped on another page.
  $effect(() => {
    if (!runWatch.added) return
    untrack(() => {
      runWatch.added = null
      q = ''
      query = ''
      filter = 'new'
      reload++
    })
  })

  // The clock the fill of a profile being read follows, between the
  // answers, only while one is.
  let now = $state(Date.now())
  let answeredAt = $state(Date.now())
  $effect(() => {
    void runWatch.tick
    answeredAt = Date.now()
  })
  $effect(() => {
    if (!reading && !runWatch.run) return
    const t = setInterval(() => (now = Date.now()), 500)
    return () => clearInterval(t)
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

  // New counts how far a going run is, 3 of 12, its searches and the
  // profiles it reads, as the fill of Stop does, and whom it holds once
  // the run is done (Tim, 22 November 2026, #43).
  let count = $derived(runCount(runWatch.run))
  let filters = $derived(
    ([
      { value: 'all', label: 'All' },
      { value: 'new', label: 'New' },
      { value: 'connected', label: 'Connected' },
      { value: 'rejected', label: 'Rejected' },
    ] as { value: PeopleFilter; label: string }[]).map((f) => ({
      ...f,
      count: f.value === 'new' && count ? `${count.done} of ${count.of}` : counts?.[f.value],
      title: f.value === 'new' ? (count ? `${count.done} of ${count.of} searches and profiles of this run are done` : 'Found or added in the last 7 days, newest first') : undefined,
    })),
  )

  // The run is started and stopped here, where its people show up. While
  // the search holds links, the same button adds them: Add for profiles
  // new to the app, Open for the one person a link leads to who is here,
  // greyed out for a link that leads to nobody. Enter does the same.
  let going = $derived(runWatch.run)
  let linked = $derived(links && query === q ? links : null)
  let mode = $derived<'run' | 'add' | 'open' | 'none'>(linked ? linked.act : 'run')
  let runLabel = $derived(
    runWatch.busy === 'adding' ? 'Adding' : mode === 'add' ? (linked!.new > 1 ? `Add ${linked!.new}` : 'Add') : mode === 'open' ? 'Open' : mode === 'none' ? 'Add'
      : runWatch.busy === 'starting' ? 'Starting' : runWatch.busy === 'stopping' ? 'Stopping' : going ? 'Stop' : 'Run',
  )
  let runTitle = $derived.by(() => {
    if (mode === 'add') return `Add ${linked!.new > 1 ? `these ${linked!.new} people` : 'this person'}, marked connected, and read ${linked!.new > 1 ? 'their profiles' : 'the profile'} from Exa${going ? ' in this run' : ''}`
    if (mode === 'open') return 'Open this person, marked connected'
    if (mode === 'none') return linked!.here.length ? 'Everyone these links lead to is here already' : "Not a link to a person's LinkedIn profile, like linkedin.com/in/name"
    if (going) return `${runSays(going, now, answeredAt)}. Stop this run: what is running finishes, the rest waits for the next run`
    return 'Run the due searches now. Takes about a minute'
  })
  function act() {
    if (mode === 'add' || mode === 'open') void runWatch.add(q)
    else if (mode === 'run') void (going ? runWatch.stop() : runWatch.start())
  }
  // Enter in the search does what the button says, once the app has said
  // what the search holds. Words are only a search.
  let entered = $state<string | null>(null)
  function enter(v: string) {
    entered = v
  }
  $effect(() => {
    if (entered === null || people.loading || query !== entered) return
    const v = entered
    untrack(() => {
      entered = null
      if (links && (links.act === 'add' || links.act === 'open')) void runWatch.add(v)
    })
  })

  // What is pasted on People, not into a field, goes into the search,
  // links above all. A LinkedIn page copied in Chrome while a person is
  // open becomes their profile, see paste.svelte.ts.
  function paste(e: ClipboardEvent) {
    if (typing()) return
    const text = e.clipboardData?.getData('text/plain')?.trim() ?? ''
    if (!text) return
    e.preventDefault()
    if (openId !== null && isPage(text)) {
      void pasting.paste(openId, text)
      return
    }
    q = text
    query = text
    document.querySelector<HTMLInputElement>('.list-pane .search input')?.focus()
  }

  const year = new Date().getFullYear()
  const age = (p: Person) => (p.linkedin?.born_about ? year - p.linkedin.born_about : null)
  // A person's second line, the way a search has one: what they do,
  // where, and about how old, each only when known. Someone added from a
  // link says where reading their profile stands until it is read.
  function line(p: Person) {
    if (p.read === 'reading') return `Reading the profile from Exa · ${p.read_ms ? fmtLeft((p.read_ms - (now - new Date(p.read_since!).getTime())) / 1000) : 'measuring the time left'}`
    if (p.read === 'waiting') return 'Waits its turn in this run'
    if (p.read === 'next') return p.read_error ? `Read on the next run. ${p.read_error}` : 'Read on the next run'
    if (p.read === 'not_indexed') return PASTE_LINE
    if (p.read === 'failed') return p.read_error || 'The profile could not be read'
    const a = age(p)
    return [p.headline || 'No headline yet', p.city, a ? `about ${a}` : ''].filter(Boolean).join(' · ')
  }
  // How far the read of a profile is, from how long reads took before,
  // never quite full until it is done, or -1 while that is not known.
  function readFill(p: Person): number {
    if (!p.read_ms || !p.read_since) return -1
    return Math.min(0.95, Math.max(0, (now - new Date(p.read_since).getTime()) / p.read_ms))
  }

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
    if (t && (t.closest('input, textarea, select, button.pick, [contenteditable]') || t.closest('[role="dialog"]'))) return
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
    // The person open may have just left the list, connected or
    // rejected. Down goes to whom took their place, up to whom stood
    // above them.
    const left = at < 0 && gone?.id === openId ? gone.at : -1
    const step = e.key === 'ArrowDown' ? 1 : -1
    const next = at >= 0 ? at + step : left >= 0 ? (step > 0 ? left : left - 1) : 0
    const to = Math.max(0, Math.min(list.length - 1, next))
    navigate(`/app/people/${list[to].id}`, { replace: at >= 0 || left >= 0 })
    // The keyboard goes with the person, so a row the pointer clicked
    // does not stay marked as the one in hand.
    const inList = !!t?.closest('.prow')
    requestAnimationFrame(() => {
      const row = document.getElementById(`person-row-${list[to].id}`)
      row?.scrollIntoView({ block: 'nearest' })
      if (inList) row?.focus({ preventScroll: true })
    })
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
  // list 480 px, room for its columns, between a quarter and three
  // quarters of the width.
  let least = $derived(Math.min(0.75, Math.ceil(Math.max(0.25, 360 / (width || 1)) / STEP - 1e-6) * STEP))
  let most = $derived(Math.max(least, Math.floor(Math.min(0.75, 1 - 480 / (width || 1)) / STEP + 1e-6) * STEP))
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

  // The person grows in from the right edge and shrinks back into it, and
  // the list gives way beside it the whole time, so both move together
  // and nothing jumps at either end. A row has the same lines at any
  // width, so the list only narrows. The person's words keep their final
  // width meanwhile and slide in rather than wrap anew on every frame.
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

  // What an empty list says: why nobody is here, and whom the other
  // filters hold. Under All and New that may be someone connected or
  // rejected, whom a search by name finds only under Connected or
  // Rejected. show is the filter its button leads to.
  const PRESS_RUN = 'Press Run. Each search asks for LinkedIn profiles and keeps the people in NRW who run their own business.'
  let empty = $derived.by((): { title: string; text: string; show?: PeopleFilter } => {
    if (linked) {
      if (linked.new) return { title: 'Not in the app yet', text: `${linked.new > 1 ? `${linked.new} people` : 'This person'} ${linked.new > 1 ? 'are' : 'is'} new to the app. Press Add or Enter to add them, marked connected.` }
      return { title: "Not a person's profile", text: 'Only a link to a LinkedIn profile, like linkedin.com/in/name, adds someone.' }
    }
    const open = counts?.all ?? 0
    const connected = counts?.connected ?? 0
    const rejected = counts?.rejected ?? 0
    const found = query ? 'match this search' : 'found so far'
    if (filter === 'connected') {
      return { title: 'Nobody connected yet', text: open ? `${plural(open, 'person', 'people')} ${found}, none of them connected yet.` : query ? 'Nobody matches this search.' : PRESS_RUN }
    }
    if (filter === 'rejected') {
      return { title: 'Nobody rejected', text: query ? 'Nobody whose profile you rejected matches this search.' : 'People whose LinkedIn profile you said is not them show here.' }
    }
    if (filter === 'new' && open) return { title: 'Nobody new this week', text: `${plural(open, 'person', 'people')} ${found}, none of them found or added in the last 7 days.` }
    const matches = (n: number) => (n === 1 ? 'matches' : 'match')
    if (connected && rejected && query) return { title: 'Only under Connected and Rejected', text: `${plural(connected, 'person', 'people')} you connected with and ${rejected} whose profile you rejected match this search.`, show: 'connected' }
    if (connected && query) return { title: 'Only under Connected', text: `${plural(connected, 'person', 'people')} you connected with ${matches(connected)} this search.`, show: 'connected' }
    if (rejected && query) return { title: 'Only under Rejected', text: `${plural(rejected, 'person', 'people')} whose profile you rejected ${matches(rejected)} this search.`, show: 'rejected' }
    if (connected && !rejected) return { title: 'Everyone is connected', text: `${plural(connected, 'person', 'people')} found so far, all of them connected. Press Run to find more.`, show: 'connected' }
    if (rejected && !connected) return { title: 'Nobody left to go through', text: `${plural(rejected, 'person', 'people')} found so far, all of them rejected. Press Run to find more.`, show: 'rejected' }
    if (connected) return { title: 'Nobody left to go through', text: `${plural(connected + rejected, 'person', 'people')} found so far, ${connected} connected and ${rejected} rejected. Press Run to find more.`, show: 'connected' }
    return { title: filter === 'new' ? 'Nobody new this week' : 'Nobody here yet', text: query ? 'Nobody matches this search.' : PRESS_RUN }
  })

  // Keep the list in step with changes made in the sheet.
  function updated(p: Detail) {
    if (!people.data) return
    people.data = people.data.map((x) => (x.id === p.id ? { ...x, profiles: p.profiles, headline: p.headline, connected: p.connected } : x))
  }

  // Someone connected is only under Connected, and someone whose profile
  // was rejected only under Rejected, so once the app took it the list
  // reads again: they leave All and New, or come back when taken
  // back, and the counts follow. Where they stood is kept, so the arrow
  // keys go on from there.
  let gone = $state<{ id: number; at: number; name: string } | null>(null)
  function moved() {
    const at = people.data?.findIndex((x) => x.id === openId) ?? -1
    if (openId !== null && at >= 0) gone = { id: openId, at, name: people.data![at].name }
    people.run(fetchPeople({ q: query, sort, filter }), { quiet: true })
  }

  // The open person's name, from the person once loaded, and from the
  // list until then. A person can be open who is not in the list shown.
  let named = $state<{ id: number; name: string } | null>(null)
  let openName = $derived(
    openId === null ? '' : named?.id === openId ? named.name
      : (people.data?.find((x) => x.id === openId)?.name ?? (gone?.id === openId ? gone.name : '')),
  )
</script>

<svelte:window onkeydown={keydown} />
<svelte:document onpaste={paste} />

<div class="people" class:wide class:open={wide && openId !== null} class:dragging bind:this={peopleEl} bind:clientWidth={width}
  style:--detail-w="{percent}%">
  <section class="list-pane" aria-label="People">
    <div class="controls">
      <div class="toolbar tight">
        <SearchInput bind:value={q} placeholder="Search or paste LinkedIn links" label="Search people" onsearch={(v) => (query = v)} onenter={enter} />
        <span class="sort">
          <Pick label="Sort by" title="Order" value={sort} options={sorts} onpick={(v) => (sort = v as PeopleSort)} />
        </span>
        <button class="btn start" class:primary={!going || mode !== 'run'} type="button" onclick={act}
          disabled={runWatch.busy !== '' || mode === 'none' || (mode === 'run' && going !== null && going.id <= 0)}
          title={runTitle}>
          {#if going}<Busy fraction={runShare(going)} />{/if}
          <Icon name={mode === 'add' || mode === 'none' ? 'plus' : mode === 'open' ? 'chevron' : going ? 'stop' : 'run'} />{runLabel}
        </button>
      </div>
      <div class="segmented filters" role="group" aria-label="Show">
        {#each filters as f (f.value)}
          <button type="button" aria-pressed={filter === f.value} onclick={() => (filter = f.value)} title={f.title}>
            {f.label}{#if f.count !== undefined}<span class="n">{f.count}</span>{/if}
          </button>
        {/each}
      </div>
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
          title={empty.title}
          text={empty.text}>
          {#if empty.show}
            {@const show = empty.show}
            <button class="btn" type="button" onclick={() => (filter = show)}>Show under {show === 'connected' ? 'Connected' : 'Rejected'}</button>
          {:else if query || filter !== 'all'}
            <button class="btn" type="button" onclick={() => ((q = ''), (query = ''), (filter = 'all'))}>Show everyone</button>
          {/if}
        </EmptyState>
      {:else}
        <ul class="list rows" data-personal class:stale={people.loading} aria-label="{plural(people.data.length, 'person', 'people')}">
          {#each people.data as p (p.id)}
            <li>
              <a class="prow" class:reading={p.read === 'reading'} id="person-row-{p.id}" href="/app/people/{p.id}" aria-current={openId === p.id ? 'true' : undefined}>
                {#if p.read === 'reading'}<Busy fraction={readFill(p)} />{/if}
                <span class="who">
                  <b class="ellipsis">{p.name}</b>
                  {#if isNew(p)}<span class="dot new" title="Came in with this run"></span><span class="sr-only">, new</span>{/if}
                  {#if !p.read}<span class="fit num" title="Fit {p.fit_score} of 100. The person says why"><span class="sr-only">, fit </span>{p.fit_score}</span>{/if}
                </span>
                <span class="ellipsis sub" class:bad={p.read === 'not_indexed' || p.read === 'failed'} title={p.read === 'not_indexed' ? PASTE_HINT : p.read ? line(p) : age(p) ? 'The age is a guess from the LinkedIn profile' : undefined}>{#if p.connected && !p.read}<span class="connected">Connected</span>{' · '}{/if}{line(p)}</span>
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
      <!-- The name and the cross stand in the row the list's controls
           stand in, the same height, the cross's glyph on the edge of the
           person under it. Tim asked for the name there on 21 November
           2026, so the person starts with what he does. -->
      <div class="detail-top">
        <h2 class="person-name ellipsis" data-personal title={openName}>{openName}</h2>
        <button class="btn quiet icon flush-end" type="button" onclick={() => navigate('/app/people')} aria-label="Close" title="Close (Esc)"><Icon name="close" /></button>
      </div>
      <div class="detail-body">
        {#key openId}
          <PersonDetail id={openId} onupdate={updated} onmoved={moved} onname={(id, name) => (named = { id, name })} />
        {/key}
      </div>
    </section>
  {/if}
</div>

{#if openId !== null && !wide}
  {#key openId}
    <PersonSheet id={openId} name={people.data?.find((x) => x.id === openId)?.name ?? (gone?.id === openId ? gone.name : undefined)} onclose={() => navigate('/app/people')} onupdate={updated} onmoved={moved} />
  {/key}
{/if}

<style>
  /* The list on the left and the person on the right, each a pane of its
     own that scrolls by itself, the way Mail is laid out. */
  .people { display: flex; height: 100%; min-width: 0; overflow: hidden; }
  .list-pane, .detail-pane { min-width: 0; height: 100%; overflow-y: auto; overscroll-behavior: contain; display: flex; flex-direction: column; }
  /* A pane that scrolls holds what is placed absolutely in it, like a
     row's words for a screen reader. Without a position of its own those
     belong to the main area round it, which then grows as long as the
     whole list and scrolls the page away, issue #77. */
  .list-pane, .detail-body { position: relative; }
  .list-pane { flex: 1 1 0; }
  /* The person comes in from the right and takes its share of the width.
     Closed, it is not there at all. */
  /* Its body scrolls, not the pane, so the edge can reach out over the
     list a little and still be caught. */
  .detail-pane { flex: 0 0 var(--detail-w); position: relative; overflow: visible; border-left: 1px solid var(--line); background: var(--surface); }
  .detail-body { flex: 1 1 auto; overflow-y: auto; overscroll-behavior: contain; }
  .detail-top, .detail-body { min-width: var(--pane-w, 0); }
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
  .share {
    position: absolute; top: 14px; left: 14px; padding: 3px 8px; border-radius: 6px; white-space: nowrap;
    background: var(--accent); color: var(--accent-ink); font-size: 12px; font-weight: 600; font-variant-numeric: tabular-nums;
  }
  /* People reads like Searches and Settings: the same centred column,
     the controls over a list of rows, each a name over one line
     of what is known. The column is the same with a person open or not,
     only narrower when the list is, so nothing moves when one opens. */
  .list-pane { --x: max(var(--pad), calc((100% - var(--col-w)) / 2)); }
  .controls { display: grid; gap: var(--gap-control); padding: var(--gap-page) var(--x); }
  .filters { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); width: 100%; }
  /* Four filters with their counts need about 370 px, and a narrow
     window has 306 at 390. There they stand two by two, each segment as
     tall as in one row. The list keeps 480 px when a person is open, so
     opening one never changes this. */
  @media (max-width: 519px) {
    .filters {
      grid-auto-flow: row; grid-template-columns: repeat(2, minmax(0, 1fr)); height: auto;
      grid-auto-rows: calc(var(--control-h) - 2 * var(--gap-inset) - 2 * var(--line-w));
    }
  }
  .filters .n { margin-left: 6px; color: var(--ink-3); font-weight: 500; font-variant-numeric: tabular-nums; }
  .sort { flex: none; }
  .controls .toolbar { flex-wrap: nowrap; }
  .controls .toolbar > :global(.search) { flex: 1 1 0; }
  /* A phone-wide window gives the search field a line of its own, and
     the order and Run follow on the next, each as wide as its words. */
  @media (max-width: 520px) {
    .controls .toolbar { flex-wrap: wrap; }
    .controls .toolbar > :global(.search) { flex: 1 1 100%; }
  }
  /* Under the list the page's inset, so a list that fits never scrolls,
     issue #77. */
  .rows-wrap { padding: 0 var(--x) var(--gap-page); display: grid; }
  .stale { opacity: .6; transition: opacity .2s; }
  /* A row as Searches has one: 12 by 16 in, the name over its line. */
  .prow { display: grid; gap: 1px; padding: 12px 16px; color: inherit; text-decoration: none; min-width: 0; }
  /* A profile being read fills its row the way a clip being worked on
     fills its card in FrameFairy, several rows at once. The row holds the
     beam, see Busy, inside the lines between the rows. */
  .prow.reading { position: relative; isolation: isolate; --beam-edge: 0px; }
  .prow:hover { background: var(--hover); text-decoration: none; }
  .prow:active { background: var(--press); }
  .prow[aria-current="true"] { background: var(--chosen); }
  .prow[aria-current="true"]:hover { background: var(--chosen-hover); }
  .prow[aria-current="true"]:active { background: var(--chosen-press); }
  /* The person open is marked already, the keyboard on it adds nothing. */
  .prow[aria-current="true"]:focus-visible { background-image: none; outline: none; }
  .who { display: flex; align-items: center; gap: 6px; min-width: 0; }
  .who b { font-weight: 600; }
  .who .fit { margin-inline-start: auto; flex: none; font-size: 13px; color: var(--ink-3); }
  .dot.new { width: 7px; height: 7px; background: var(--accent); }
  .sub { font-size: 13px; color: var(--ink-2); font-variant-numeric: tabular-nums; }
  .connected { color: var(--good); font-weight: 600; }
  .sub.bad { color: var(--bad); }
  /* Over the fill the line takes the colour of the name, so it reads as
     well as the name does, see walks/look.mjs. */
  .reading .sub { color: var(--ink); }
  @media (max-width: 520px) { .prow { padding: 12px 14px; } }
  /* The person mirrors the list beside it: the same inset at the sides
     and over the top, the name and the cross on the row of the search
     field, and the person's buttons on the row of the filter, a
     --gap-control under it, see walks/look.mjs. Wider than its column,
     the person stands in the middle, as the list does. */
  .detail-top {
    flex: none; display: flex; justify-content: space-between; align-items: center; gap: var(--gap-control);
    box-sizing: content-box; height: var(--control-h); padding: var(--gap-page) var(--dx) 0;
  }
  .detail-pane { --dx: max(var(--pad), calc((100% - 640px) / 2)); }
  /* Under the last section the same inset as round the rest, so a person
     who fits never scrolls, issue #77. */
  .detail-body { padding: var(--gap-control) var(--dx) var(--gap-page); }
</style>
