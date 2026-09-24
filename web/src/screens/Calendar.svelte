<script lang="ts">
  import { api, EVENT_TYPES, type PublicConfig, type PublicEvent } from '../lib/api'
  import { fmtDayKey, plural, RANGES, rangeDays, TYPE_LABEL, type Range } from '../lib/format'
  import Brand from '../lib/components/Brand.svelte'
  import Chips from '../lib/components/Chips.svelte'
  import DayList from '../lib/components/DayList.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import EventCard from '../lib/components/EventCard.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import Segmented from '../lib/components/Segmented.svelte'
  import Skeleton from '../lib/components/Skeleton.svelte'
  import ThemeButton from '../lib/components/ThemeButton.svelte'

  let { config }: { config: PublicConfig } = $props()

  let range = $state<Range>('week')
  let city = $state('')
  let type = $state('')
  let events = $state<PublicEvent[]>([])
  let loading = $state(true)
  let error = $state('')
  let reload = $state(0)

  let days = $derived(rangeDays(range))
  let cityOptions = $derived([{ value: '', label: 'All NRW' }, ...config.cities.map((c) => ({ value: c, label: c }))])
  const typeOptions = [{ value: '', label: 'All types' }, ...EVENT_TYPES.map((t) => ({ value: t, label: TYPE_LABEL[t] }))]

  $effect(() => {
    document.title = "Speaker Trail · Who's on stage in NRW"
  })

  let seq = 0
  $effect(() => {
    const q = { from: days.from, to: days.to, city, type }
    void reload
    const mine = ++seq
    loading = true
    error = ''
    api
      .publicEvents(q)
      .then((list) => {
        if (mine === seq) events = list
      })
      .catch((e) => {
        if (mine === seq) error = e instanceof Error ? e.message : 'Could not load the events'
      })
      .finally(() => {
        if (mine === seq) loading = false
      })
  })

  let filtered = $derived(city !== '' || type !== '')
  let summary = $derived(
    `${plural(events.length, 'event')}${city ? ` in ${city}` : ''}, ${fmtDayKey(days.from)} to ${fmtDayKey(days.to)}`,
  )
</script>

<div class="public">
  <header class="topbar">
    <div class="topbar-inner">
      <Brand href="/" sub="" />
      <div class="row">
        <ThemeButton />
        <a class="btn quiet" href="/login"><Icon name="login" size={16} />Log in</a>
      </div>
    </div>
  </header>

  <main id="main" class="view narrow">
    <div class="hero">
      <h1>Who's on stage in NRW</h1>
      <p class="muted">Talks, pitches, panels and meetups across North Rhine-Westphalia, and the people on stage. Checked every night.</p>
    </div>

    <div class="filters">
      <Segmented label="When" options={RANGES.map((r) => ({ value: r.id, label: r.label }))} bind:value={range} />
      <Chips label="City" options={cityOptions} bind:value={city} />
      <Chips label="Type" options={typeOptions} bind:value={type} />
    </div>

    {#if loading && !events.length}
      <Skeleton variant="events" count={4} />
    {:else if error}
      <EmptyState icon="alert" tone="bad" title="Could not load the events" text={error}>
        <button class="btn" type="button" onclick={() => reload++}><Icon name="refresh" />Try again</button>
      </EmptyState>
    {:else if !events.length}
      <EmptyState
        title="Nobody on stage for this pick"
        text={filtered ? 'Nothing matches these filters in this range. Try all of NRW, every type or a longer range.' : 'No events found in this range yet. New ones come in every night.'}>
        {#if filtered}
          <button class="btn" type="button" onclick={() => ((city = ''), (type = ''))}>Clear filters</button>
        {/if}
        {#if range !== '30'}
          <button class="btn primary" type="button" onclick={() => (range = '30')}>Show the next 30 days</button>
        {/if}
      </EmptyState>
    {:else}
      <p class="summary" aria-live="polite">
        {summary}
        {#if loading}<span class="faint">Updating</span>{/if}
      </p>
      <div class:stale={loading}>
        <DayList items={events}>
          {#snippet card(ev)}
            <EventCard event={ev} />
          {/snippet}
        </DayList>
      </div>
    {/if}
  </main>

  <footer class="foot">
    <p>Speaker Trail lists in-person events across North Rhine-Westphalia. Details come from the organisers' own pages, so check there before you go.</p>
  </footer>
</div>

<style>
  .public { min-height: 100dvh; display: grid; grid-template-rows: auto 1fr auto; }
  .topbar { background: var(--surface); border-bottom: 1px solid var(--line); }
  .topbar-inner {
    max-width: 820px; margin: 0 auto; padding: 10px var(--gutter); display: flex; align-items: center; justify-content: space-between; gap: 12px;
  }
  main { padding: 24px var(--gutter) 48px; width: 100%; }
  .hero { display: grid; gap: 6px; }
  .hero h1 { font-size: 30px; letter-spacing: -.02em; }
  .hero p { max-width: 60ch; }
  .filters { display: grid; gap: 10px; min-width: 0; }
  .filters :global(.segmented) { justify-self: start; }
  .summary { font-size: 13px; color: var(--ink-2); display: flex; gap: 10px; margin-bottom: -10px; }
  .stale { opacity: .6; transition: opacity .15s; }
  .foot { padding: 20px var(--gutter) 32px; text-align: center; font-size: 13px; color: var(--ink-3); }
  .foot p { max-width: 60ch; margin: 0 auto; }
  @media (max-width: 760px) {
    main { padding-top: 16px; }
    .hero h1 { font-size: 24px; }
    .hero p { font-size: 14px; }
    .filters { gap: 8px; }
    .filters :global(.segmented) { justify-self: stretch; }
  }
</style>
