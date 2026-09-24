<script lang="ts">
  import { api, EVENT_TYPES, type Fit, type FitFilter, type PrivateEvent } from '../lib/api'
  import { fmtAgo, plural, RANGES, rangeDays, TYPE_LABEL, type Range } from '../lib/format'
  import { Load } from '../lib/load.svelte'
  import { errorText, toast } from '../lib/toast.svelte'
  import Chips from '../lib/components/Chips.svelte'
  import DayList from '../lib/components/DayList.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import EventCard from '../lib/components/EventCard.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import SearchInput from '../lib/components/SearchInput.svelte'
  import Segmented from '../lib/components/Segmented.svelte'
  import Skeleton from '../lib/components/Skeleton.svelte'

  let range = $state<Range>('30')
  let fit = $state<FitFilter>('kept')
  let q = $state('')
  let query = $state('')
  let city = $state('')
  let type = $state('')
  let busy = $state<number | null>(null)
  let reload = $state(0)

  const events = new Load<PrivateEvent[]>()

  $effect(() => {
    document.title = 'Events · Speaker Trail'
  })

  $effect(() => {
    const { from, to } = rangeDays(range)
    const params = { from, to, fit, q: query }
    void reload
    events.run(() => api.events(params))
  })

  // City and type filter locally so every chip can show its count.
  let all = $derived(events.data ?? [])
  let cityOptions = $derived.by(() => {
    const counts = new Map<string, number>()
    for (const e of all) if (!type || e.type === type) counts.set(e.city, (counts.get(e.city) ?? 0) + 1)
    const list = [...counts.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    if (city && !counts.has(city)) list.push([city, 0])
    return [{ value: '', label: 'All cities' }, ...list.map(([c, n]) => ({ value: c, label: c, count: n }))]
  })
  let typeOptions = $derived.by(() => {
    const counts = new Map<string, number>()
    for (const e of all) if (!city || e.city === city) counts.set(e.type, (counts.get(e.type) ?? 0) + 1)
    return [
      { value: '', label: 'All types' },
      ...EVENT_TYPES.filter((t) => counts.has(t) || t === type).map((t) => ({ value: t, label: TYPE_LABEL[t], count: counts.get(t) ?? 0 })),
    ]
  })
  let shown = $derived(all.filter((e) => (!city || e.city === city) && (!type || e.type === type)))

  const fitOptions: { value: FitFilter; label: string }[] = [
    { value: 'kept', label: 'Kept' },
    { value: 'dropped', label: 'Dropped' },
    { value: 'all', label: 'All' },
  ]

  async function setFit(ev: PrivateEvent, next: Fit) {
    if (ev.fit === next) return
    busy = ev.id
    try {
      const updated = await api.patchEvent(ev.id, { fit: next })
      if (events.data) events.data = events.data.map((e) => (e.id === ev.id ? { ...e, ...updated } : e))
      toast.show(next === 'kept' ? `Kept ${ev.title}` : `Dropped ${ev.title}`)
    } catch (e) {
      toast.show(errorText(e), 'bad')
    } finally {
      busy = null
    }
  }
</script>

<div class="view">
  <div class="view-head">
    <div>
      <h1>Events</h1>
      <p>Everything the crawler found. Keep what belongs in the calendar.</p>
    </div>
  </div>

  <div class="filters">
    <div class="toolbar tight">
      <SearchInput bind:value={q} placeholder="Search events" label="Search events" onsearch={(v) => (query = v)} />
      <Segmented label="Fit" options={fitOptions} bind:value={fit} />
    </div>
    <Segmented label="When" options={RANGES.map((r) => ({ value: r.id, label: r.label }))} bind:value={range} />
    <Chips label="City" options={cityOptions} bind:value={city} />
    <Chips label="Type" options={typeOptions} bind:value={type} />
  </div>

  {#if events.error && !events.data}
    <EmptyState icon="alert" tone="bad" title="Could not load the events" text={events.error}>
      <button class="btn" type="button" onclick={() => reload++}><Icon name="refresh" />Try again</button>
    </EmptyState>
  {:else if !events.data}
    <Skeleton variant="events" count={4} />
  {:else if !shown.length}
    <EmptyState title="No events match" text="Try another fit, a longer range or clear the search and filters.">
      <button class="btn" type="button" onclick={() => ((city = ''), (type = ''), (q = ''), (query = ''), (fit = 'all'))}>Show everything</button>
    </EmptyState>
  {:else}
    <p class="summary">{plural(shown.length, 'event')}{events.loading ? ', updating' : ''}</p>
    <div class:stale={events.loading}>
      <DayList items={shown}>
        {#snippet card(ev)}
          <EventCard event={ev} muted={ev.fit === 'dropped'} personHref={(p) => (p.id ? `/app/people/${p.id}` : undefined)}>
            {#snippet footer()}
              <div class="fit-row">
                <span class="pill {ev.fit === 'kept' ? 'good' : ev.fit === 'dropped' ? 'bad' : 'warn'}">
                  <span class="dot"></span>{ev.fit === 'kept' ? 'Kept' : ev.fit === 'dropped' ? 'Dropped' : 'Not sorted'}
                </span>
                {#if ev.fit_reason}<span class="reason">{ev.fit_reason}</span>{/if}
                <span class="row actions">
                  <button class="btn sm good" type="button" aria-pressed={ev.fit === 'kept'} disabled={busy === ev.id} onclick={() => setFit(ev, 'kept')}>
                    <Icon name="check" size={14} />Keep
                  </button>
                  <button class="btn sm bad" type="button" aria-pressed={ev.fit === 'dropped'} disabled={busy === ev.id} onclick={() => setFit(ev, 'dropped')}>
                    <Icon name="x" size={14} />Drop
                  </button>
                </span>
              </div>
              <div class="src">
                <span class="faint">Found on</span>
                {#each ev.sources as s, i (s.id)}
                  <a href="/app/sources/{s.id}">{s.name}</a>{i < ev.sources.length - 1 ? ', ' : ''}
                {:else}
                  <span class="faint">no source</span>
                {/each}
                <span class="faint">· first seen {fmtAgo(ev.first_seen)}</span>
              </div>
            {/snippet}
          </EventCard>
        {/snippet}
      </DayList>
    </div>
  {/if}
</div>

<style>
  .filters { display: grid; gap: 10px; min-width: 0; }
  .filters > :global(.segmented) { justify-self: start; }
  .summary { font-size: 13px; color: var(--ink-2); margin-bottom: -10px; }
  .stale { opacity: .6; }
  .fit-row { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; font-size: 13px; }
  .reason { color: var(--ink-2); font-family: var(--mono); font-size: 12px; }
  .src { font-size: 13px; overflow-wrap: anywhere; }
  .actions { margin-left: auto; flex-wrap: nowrap; }
  @media (max-width: 760px) {
    .filters { gap: 8px; }
    .filters > :global(.segmented) { justify-self: stretch; }
  }
</style>
