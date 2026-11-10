<script lang="ts">
  import { untrack } from 'svelte'
  import { api, type PeopleFilter, type PeopleSort, type Person, type PersonDetail } from '../lib/api'
  import { fmtDay, initials, plural } from '../lib/format'
  import { Load } from '../lib/load.svelte'
  import { navigate, router } from '../lib/router.svelte'
  import { runWatch } from '../lib/run.svelte'
  import Chips from '../lib/components/Chips.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import ProfileBadges from '../lib/components/ProfileBadges.svelte'
  import RunOutcome from '../lib/components/RunOutcome.svelte'
  import RunProgress from '../lib/components/RunProgress.svelte'
  import SearchInput from '../lib/components/SearchInput.svelte'
  import Skeleton from '../lib/components/Skeleton.svelte'
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
    runWatch.busy === 'starting' ? 'Starting' : runWatch.busy === 'stopping' ? 'Stopping' : going ? 'Stop the run' : 'Run',
  )
  let runOpen = $state<number | null>(null)

  // What the LinkedIn profile says, in one line: the likely age and when
  // the person was last active there.
  const year = new Date().getFullYear()
  function facts(p: Person): string {
    const l = p.linkedin
    if (!l) return ''
    const parts: string[] = []
    if (l.born_about) parts.push(`about ${year - l.born_about}`)
    if (l.last_active) parts.push(`active ${fmtDay(l.last_active)}`)
    return parts.join(' · ')
  }
  function factsTitle(p: Person): string {
    const l = p.linkedin
    if (!l) return ''
    return [l.born_about ? `A guess from the LinkedIn profile: ${l.age_reason}` : '', l.roles ? `Now: ${l.roles}` : '']
      .filter(Boolean)
      .join('\n')
  }
  let others = $derived(counts ? counts.all : 0)

  // Keep the list in step with changes made in the sheet.
  function updated(p: PersonDetail) {
    if (!people.data) return
    people.data = people.data.map((x) => (x.id === p.id ? { ...x, profiles: p.profiles, headline: p.headline, connected: p.connected } : x))
  }
</script>

<div class="view">
  <div class="view-head">
    <div>
      <h1>People</h1>
      <p>On LinkedIn, in NRW, running their own thing.</p>
    </div>
    <button class="btn start" class:primary={!going} type="button" onclick={() => (going ? runWatch.stop() : runWatch.start())}
      disabled={runWatch.busy !== '' || (going !== null && going.id <= 0)}
      title={going ? 'Stop this run. What is running finishes, the rest waits for the next run' : 'Run the due searches now. Takes about a minute'}>
      <Icon name={going ? 'close' : 'refresh'} />{runLabel}
    </button>
  </div>

  {#if runWatch.run}
    <button class="panel run-now" type="button" disabled={runWatch.run.id <= 0} onclick={() => (runOpen = runWatch.run?.id ?? null)}>
      <RunProgress run={runWatch.run} />
    </button>
  {:else if runWatch.ended}
    <button class="panel run-now" type="button" onclick={() => (runOpen = runWatch.ended?.id ?? null)}>
      <RunOutcome run={runWatch.ended} />
    </button>
  {/if}

  <div class="filters">
    <div class="toolbar tight">
      <SearchInput bind:value={q} placeholder="Search people" label="Search people" onsearch={(v) => (query = v)} />
      <label class="sort">
        <span class="sr-only">Sort by</span>
        <select class="select" bind:value={sort}>
          <option value="young">Youngest</option>
          <option value="new">Newest</option>
          <option value="fit">Best fit</option>
          <option value="name">Name</option>
        </select>
      </label>
    </div>
    <Chips label="Show" options={filters} bind:value={filter} scroll={false} />
  </div>

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
    <p class="summary">{plural(people.data.length, 'person', 'people')}{people.loading ? ', updating' : ''}</p>
    <ul class="list" class:stale={people.loading}>
      {#each people.data as p (p.id)}
        <li>
          <a class="row-btn person" href="/app/people/{p.id}" aria-current={openId === p.id ? 'true' : undefined}
            title={p.connected ? 'Connected' : undefined}>
            <span class="avatar" class:connected={p.connected} aria-hidden="true">{initials(p.name)}</span>
            <b class="ellipsis name">{p.name}{#if p.connected}<span class="sr-only">, connected</span>{/if}{#if isNew(p)}<span class="new" title="Found by this run">new</span>{/if}</b>
            <span class="who">
              <span class="ellipsis sub">{[p.headline, p.city].filter(Boolean).join(' · ') || 'No headline yet'}</span>
              {#if facts(p)}
                <span class="ellipsis facts" title={factsTitle(p)}>{facts(p)}</span>
              {/if}
              {#if p.signals.length}
                <span class="signals ellipsis" title={p.signals.map((s) => `${s.for ? 'For' : 'Against'}: ${s.label}`).join('\n')}>
                  {#each p.signals as s, i (s.key)}{#if i}<span class="sep" aria-hidden="true">·</span>{/if}<span class:for={s.for} class:against={!s.for}><span class="sr-only">{s.for ? 'For: ' : 'Against: '}</span>{s.label}</span>{/each}
                </span>
              {/if}
            </span>
            <span class="meta">
              <ProfileBadges profiles={p.profiles} />
            </span>
          </a>
        </li>
      {/each}
    </ul>
  {/if}
</div>

{#if openId !== null}
  {#key openId}
    <PersonSheet id={openId} onclose={() => navigate('/app/people')} onupdate={updated} />
  {/key}
{:else if runOpen !== null}
  {#key runOpen}
    <RunSheet id={runOpen} onclose={() => (runOpen = null)} />
  {/key}
{/if}

<style>
  .filters { display: grid; gap: 10px; }
  .sort { flex: none; }
  .summary { font-size: 13px; color: var(--ink-2); margin-bottom: -10px; }
  .run-now { display: block; width: 100%; text-align: left; font: inherit; color: var(--ink); padding: 12px 14px; cursor: pointer; }
  .run-now:hover { border-color: var(--line-strong); }
  /* Room for the longest label, so the button keeps its width. */
  .start { min-width: 10.2em; justify-content: center; }
  .new {
    margin-left: 6px; padding: 1px 6px; border-radius: 999px; font-size: 11px; font-weight: 600; vertical-align: 1px;
    background: var(--accent-soft); color: var(--accent);
  }
  .stale { opacity: .6; }
  .person {
    grid-template-columns: 36px minmax(0, 1fr) auto;
    grid-template-areas: "av name meta" "av who meta";
    align-items: start; row-gap: 1px;
  }
  .person .avatar { grid-area: av; }
  .name { grid-area: name; font-weight: 600; }
  .who { grid-area: who; display: grid; gap: 1px; min-width: 0; }
  .meta { grid-area: meta; }
  .sub { font-size: 13px; color: var(--ink-2); }
  .facts { font-size: 13px; color: var(--accent); font-variant-numeric: tabular-nums; }
  .meta { display: flex; gap: 8px; align-items: center; padding-top: 2px; }
  /* A connected person's initials turn green. */
  .avatar.connected { background: var(--good-soft); color: var(--good); }
  /* Relative, so the hidden words of signals cut off at the end stay inside
     the row and never widen the page. */
  .signals { font-size: 12px; font-weight: 500; position: relative; }
  .signals .for { color: var(--good); }
  .signals .against { color: var(--bad); }
  /* The sign says it without colour. Screen readers get the words instead. */
  .signals .for::before { content: "+\2009" / ""; }
  .signals .against::before { content: "\2212\2009" / ""; }
  .signals .sep { color: var(--ink-3); margin: 0 5px; }
  @media (max-width: 520px) {
    .person { padding: 12px 14px; column-gap: 10px; grid-template-areas: "av name meta" "av who who"; }
  }
</style>
