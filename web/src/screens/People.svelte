<script lang="ts">
  import { untrack } from 'svelte'
  import { api, type PeopleFilter, type PeopleSort, type Person, type PersonDetail } from '../lib/api'
  import { ACTIVITY_TONE, activityText, fmtDay, initials, plural, ROLE_LABEL } from '../lib/format'
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

  let q = $state('')
  let query = $state('')
  // A new person is what Tim looks at first, so the list opens on everyone,
  // newest on top.
  let sort = $state<PeopleSort>('new')
  let filter = $state<PeopleFilter>('all')
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
      { value: 'founder', label: 'Founders' },
      { value: 'upcoming', label: 'Upcoming' },
      { value: 'profile', label: 'Profile found' },
    ] as { value: PeopleFilter; label: string }[]).map((f) => ({ ...f, count: counts?.[f.value] })),
  )
  let others = $derived(counts ? counts.all : 0)

  // Keep the list in step with changes made in the sheet.
  function updated(p: PersonDetail) {
    if (!people.data) return
    people.data = people.data.map((x) => (x.id === p.id ? { ...x, profiles: p.profiles, headline: p.headline } : x))
  }
</script>

<div class="view">
  <div class="view-head">
    <div>
      <h1>People</h1>
      <p>Everyone the engine found, on stage or behind a startup.</p>
    </div>
  </div>

  {#if runWatch.run}
    <a class="panel run-now" href={runWatch.run.id > 0 ? `/app/runs/${runWatch.run.id}` : '/app/runs'}>
      <RunProgress run={runWatch.run} />
    </a>
  {:else if runWatch.ended}
    <a class="panel run-now" href="/app/runs/{runWatch.ended.id}">
      <RunOutcome run={runWatch.ended} />
    </a>
  {/if}

  <div class="filters">
    <div class="toolbar tight">
      <SearchInput bind:value={q} placeholder="Search people" label="Search people" onsearch={(v) => (query = v)} />
      <label class="sort">
        <span class="sr-only">Sort by</span>
        <select class="select" bind:value={sort}>
          <option value="new">Newest</option>
          <option value="next">Next appearance</option>
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
      title={filter === 'founder' && !query ? 'No founders yet' : 'Nobody here yet'}
      text={filter !== 'all' && others > 0
        ? `${plural(others, 'person', 'people')} found so far, none of them ${filter === 'founder' ? 'a founder yet. Founders show up as runs read event pages' : 'in this filter'}.`
        : query ? 'Nobody matches this search.' : 'People show up once the crawler finds them on event pages.'}>
      {#if query || filter !== 'all'}
        <button class="btn" type="button" onclick={() => ((q = ''), (query = ''), (filter = 'all'))}>Show everyone</button>
      {/if}
    </EmptyState>
  {:else}
    <p class="summary">{plural(people.data.length, 'person', 'people')}{people.loading ? ', updating' : ''}</p>
    <ul class="list" class:stale={people.loading}>
      {#each people.data as p (p.id)}
        <li>
          <a class="row-btn person" href="/app/people/{p.id}" aria-current={openId === p.id ? 'true' : undefined}>
            <span class="avatar" aria-hidden="true">{initials(p.name)}</span>
            <b class="ellipsis name">{p.name}{#if isNew(p)}<span class="new" title="Found by this run">new</span>{/if}</b>
            <span class="who">
              <span class="ellipsis sub">{[p.headline, p.city].filter(Boolean).join(' · ') || 'No headline yet'}</span>
              <span class="next" class:none={!p.next_appearance && !p.activity}>
                {#if p.next_appearance}
                  <Icon name="calendar" size={13} /><span class="ellipsis">{fmtDay(p.next_appearance.starts_at)} · {p.next_appearance.title} · {ROLE_LABEL[p.next_appearance.role] ?? p.next_appearance.role}</span>
                {:else if p.activity}
                  <span class="dot-tone {ACTIVITY_TONE[p.activity.state]}" aria-hidden="true"></span><span class="ellipsis" title={p.activity.note}>{activityText(p.activity)}</span>
                {:else}
                  No upcoming appearance
                {/if}
              </span>
            </span>
            <span class="meta">
              <ProfileBadges profiles={p.profiles} />
              <span class="count num" title="Appearances">{p.appearances}<span class="sr-only"> appearances</span></span>
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
{/if}

<style>
  .filters { display: grid; gap: 10px; }
  .sort { flex: none; }
  .summary { font-size: 13px; color: var(--ink-2); margin-bottom: -10px; }
  .run-now { display: block; color: var(--ink); text-decoration: none; padding: 12px 14px; }
  .run-now:hover { border-color: var(--line-strong); text-decoration: none; }
  .new {
    margin-left: 6px; padding: 1px 6px; border-radius: 999px; font-size: 11px; font-weight: 600; vertical-align: 1px;
    background: var(--accent-soft); color: var(--accent);
  }
  .stale { opacity: .6; }
  .dot-tone { width: 7px; height: 7px; border-radius: 50%; background: var(--ink-3); flex: none; }
  .dot-tone.good { background: var(--good); }
  .dot-tone.warn { background: var(--warn); }
  .dot-tone.bad { background: var(--bad); }
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
  .next { font-size: 13px; color: var(--accent); display: flex; align-items: center; gap: 5px; min-width: 0; }
  .next :global(svg) { flex: none; }
  .next.none { color: var(--ink-3); }
  .meta { display: flex; gap: 8px; align-items: center; padding-top: 2px; }
  .count {
    min-width: 26px; height: 22px; padding: 0 6px; border-radius: 999px; background: var(--surface-2); color: var(--ink-2);
    font-size: 12px; display: inline-grid; place-items: center;
  }
  @media (max-width: 520px) {
    .person { padding: 12px 14px; column-gap: 10px; grid-template-areas: "av name meta" "av who who"; }
  }
</style>
