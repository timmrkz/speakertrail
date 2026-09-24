<script lang="ts">
  import { api, type PeopleFilter, type PeopleSort, type Person, type PersonDetail } from '../lib/api'
  import { fmtDay, initials, plural, ROLE_LABEL } from '../lib/format'
  import { Load } from '../lib/load.svelte'
  import { navigate, router } from '../lib/router.svelte'
  import Chips from '../lib/components/Chips.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import ProfileBadges from '../lib/components/ProfileBadges.svelte'
  import SearchInput from '../lib/components/SearchInput.svelte'
  import Skeleton from '../lib/components/Skeleton.svelte'
  import PersonSheet from './PersonSheet.svelte'

  let q = $state('')
  let query = $state('')
  let sort = $state<PeopleSort>('next')
  let filter = $state<PeopleFilter>('all')
  let reload = $state(0)

  const people = new Load<Person[]>()

  $effect(() => {
    document.title = 'People · Speaker Trail'
  })

  $effect(() => {
    const params = { q: query, sort, filter }
    void reload
    people.run(() => api.people(params))
  })

  let openId = $derived.by(() => {
    const m = router.match('/app/people/:id')
    return m ? Number(m.id) : null
  })

  const filters: { value: PeopleFilter; label: string }[] = [
    { value: 'all', label: 'All' },
    { value: 'upcoming', label: 'Upcoming' },
    { value: 'profile', label: 'Profile found' },
  ]

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
      <p>Everyone named on stage, next appearance first.</p>
    </div>
  </div>

  <div class="filters">
    <div class="toolbar tight">
      <SearchInput bind:value={q} placeholder="Search people" label="Search people" onsearch={(v) => (query = v)} />
      <label class="sort">
        <span class="sr-only">Sort by</span>
        <select class="select" bind:value={sort}>
          <option value="next">Next appearance</option>
          <option value="new">Newest</option>
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
    <EmptyState icon="people" title="Nobody here yet" text={query || filter !== 'all' ? 'Nobody matches this search or filter.' : 'People show up once the crawler finds them on event pages.'}>
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
            <b class="ellipsis name">{p.name}</b>
            <span class="who">
              <span class="ellipsis sub">{[p.headline, p.city].filter(Boolean).join(' · ') || 'No headline yet'}</span>
              <span class="next" class:none={!p.next_appearance}>
                {#if p.next_appearance}
                  <Icon name="calendar" size={13} /><span class="ellipsis">{fmtDay(p.next_appearance.starts_at)} · {p.next_appearance.title} · {ROLE_LABEL[p.next_appearance.role] ?? p.next_appearance.role}</span>
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
