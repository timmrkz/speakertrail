<script lang="ts">
  import { api, type Stats } from '../lib/api'
  import { fmtAgo, fmtDateTime, fmtDuration, fmtNum } from '../lib/format'
  import { Load } from '../lib/load.svelte'
  import { runWatch } from '../lib/run.svelte'
  import RunOutcome from '../lib/components/RunOutcome.svelte'
  import RunProgress from '../lib/components/RunProgress.svelte'
  import RunStatePill from '../lib/components/RunStatePill.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import WeeklyChart from '../lib/components/WeeklyChart.svelte'

  const stats = new Load<Stats>()
  stats.run(api.stats)

  $effect(() => {
    document.title = 'Overview · Speaker Trail'
  })

  let tiles = $derived.by(() => {
    const s = stats.data
    if (!s) return []
    const src = s.totals.sources
    return [
      { label: 'People', value: s.totals.people, delta: s.last_7_days.people, note: `${fmtNum(s.totals.profiles)} profiles found`, href: '/app/people' },
      { label: 'Sources', value: src.active + src.probation + src.candidate, delta: s.last_7_days.sources, note: `${src.active} active, ${src.probation} probation, ${src.candidate} candidate`, href: '/app/sources' },
      { label: 'Organisations', value: s.totals.organisations, delta: s.last_7_days.organisations, note: 'hosts, venues and companies', href: '' },
      { label: 'Upcoming events kept', value: s.totals.events_kept_upcoming, delta: s.last_7_days.events, note: `of ${fmtNum(s.totals.events_upcoming)} upcoming`, href: '/app/events' },
    ]
  })

  let fit = $derived(stats.data?.fit ?? null)
  let decided = $derived((fit?.signals ?? []).some((s) => s.kept + s.skipped > 0))

  let topCities = $derived((stats.data?.cities ?? []).slice(0, 6))
  let cityMax = $derived(Math.max(1, ...topCities.map((c) => c.events)))
  $effect(() => runWatch.watch())
  // The numbers read again when a run ends.
  $effect(() => {
    if (runWatch.ended) stats.run(api.stats, { quiet: true })
  })
  // The last run as every screen shows it.
  let run = $derived.by(() => {
    const last = stats.data?.last_run ?? null
    if (runWatch.run && (!last || runWatch.run.id >= last.id || runWatch.run.id <= 0)) return runWatch.run
    if (runWatch.ended && (!last || runWatch.ended.id >= last.id)) return runWatch.ended
    return last
  })
</script>

<div class="view">
  <div class="view-head">
    <div>
      <h1>Where we are in numbers</h1>
      <p>Totals so far, and what came in during the last 7 days.</p>
    </div>
  </div>

  {#if stats.error && !stats.data}
    <EmptyState icon="alert" tone="bad" title="Could not load the numbers" text={stats.error}>
      <button class="btn" type="button" onclick={() => stats.run(api.stats)}><Icon name="refresh" />Try again</button>
    </EmptyState>
  {:else if !stats.data}
    <div class="stats" aria-busy="true">
      {#each Array(4) as _, i (i)}
        <div class="stat"><span class="skel" style:width="60%" style:height="12px"></span><span class="skel" style:width="45%" style:height="28px"></span><span class="skel" style:width="70%" style:height="12px"></span></div>
      {/each}
    </div>
  {:else}
    <div class="stats">
      {#each tiles as t (t.label)}
        <svelte:element this={t.href ? 'a' : 'div'} href={t.href || undefined} class="stat">
          <span class="stat-label">{t.label}</span>
          <span class="value num">{fmtNum(t.value)}</span>
          <span class="delta"><b class:zero={t.delta === 0}>+{fmtNum(t.delta)}</b> in the last 7 days</span>
          <span class="note">{t.note}</span>
        </svelte:element>
      {/each}
    </div>

    <section class="panel" aria-labelledby="run-h">
      <div class="panel-head">
        <h2 id="run-h">Last run</h2>
        <a class="small" href="/app/runs">All runs</a>
      </div>
      {#if run}
        <a class="run" href={run.id > 0 ? `/app/runs/${run.id}` : '/app/runs'}>
          <div class="run-when">
            <b>{fmtDateTime(run.started_at)}</b>
            <span class="faint small">
              {fmtAgo(run.started_at)}{run.state === 'going' ? ', still running' : run.finished_at ? `, took ${fmtDuration(new Date(run.finished_at).getTime() - new Date(run.started_at).getTime())}` : ''}
            </span>
          </div>
          <span class="run-state"><RunStatePill state={run.state} /></span>
          <div class="run-out">
            {#if run.state === 'going' && run.progress}<RunProgress {run} />{:else}<RunOutcome {run} />{/if}
          </div>
          <dl class="run-nums">
            <div><dt>Sources checked</dt><dd class="num">{fmtNum(run.sources_checked)}</dd></div>
            <div><dt>New people</dt><dd class="num">{fmtNum(run.people_new)}</dd></div>
            <div><dt>Who fit</dt><dd class="num">{fmtNum(run.fits_new)}</dd></div>
            <div class:err={run.errors > 0}><dt>Sources failed</dt><dd class="num">{fmtNum(run.errors)}</dd></div>
          </dl>
        </a>
      {:else}
        <p class="muted">No run yet. The first one starts tonight.</p>
      {/if}
    </section>

    {#if fit}
      <section class="panel" aria-labelledby="fit-h">
        <div class="panel-head">
          <h2 id="fit-h" title="Keep or skip people on their sheet. Each decision counts for the signals the person had, so a signal that misleads shows up">Keeps and skips</h2>
          <a class="small" href="/app/people">People</a>
        </div>
        <p class="top">
          Of the top {fit.top.size} by fit, <b class="good">{fit.top.kept} kept</b>, <b class="bad">{fit.top.skipped} skipped</b>,
          {fit.top.open} not decided yet.
        </p>
        {#if decided}
          <ul class="fit-signals">
            {#each fit.signals as s (s.key)}
              <li class:none={s.kept + s.skipped === 0}>
                <span class="ellipsis" class:for={s.for} class:against={!s.for}>{s.for ? '+' : '−'}&thinsp;{s.label}</span>
                <span class="num good" title="Kept">{s.kept}</span>
                <span class="num bad" title="Skipped">{s.skipped}</span>
              </li>
            {/each}
          </ul>
        {:else}
          <p class="muted small">No one kept or skipped yet. Each decision shows here, by the signals the person had.</p>
        {/if}
      </section>
    {/if}

    <div class="grid-2">
      <section class="panel" aria-labelledby="weekly-h">
        <div class="panel-head">
          <h2 id="weekly-h">New each week</h2>
          <span class="faint small">Last 8 weeks</span>
        </div>
        <WeeklyChart weekly={stats.data.weekly} />
      </section>

      <section class="panel" aria-labelledby="cities-h">
        <div class="panel-head">
          <h2 id="cities-h">Upcoming events by city</h2>
          <a class="small" href="/app/events">All events</a>
        </div>
        {#if topCities.length}
          <ul class="cities">
            {#each topCities as c (c.city)}
              <li>
                <span class="city ellipsis">{c.city}</span>
                <span class="bar" aria-hidden="true"><i style:width="{(c.events / cityMax) * 100}%"></i></span>
                <span class="num">{c.events}</span>
              </li>
            {/each}
          </ul>
        {:else}
          <p class="muted">No upcoming events yet.</p>
        {/if}
      </section>
    </div>
  {/if}
</div>

<style>
  .stats { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
  @media (max-width: 1100px) { .stats { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  .stat {
    background: var(--surface); border: 1px solid var(--line); border-radius: var(--radius); padding: 14px 16px;
    display: grid; gap: 4px; color: var(--ink); text-decoration: none; min-width: 0; align-content: start;
  }
  a.stat:hover { border-color: var(--line-strong); text-decoration: none; }
  .stat-label { font-size: 13px; font-weight: 600; color: var(--ink-2); }
  .value { font-size: 30px; font-weight: 500; letter-spacing: -.02em; line-height: 1.1; }
  .delta { font-size: 13px; color: var(--ink-2); }
  .delta b { color: var(--good); font-weight: 600; }
  .delta b.zero { color: var(--ink-3); }
  .note { font-size: 12px; color: var(--ink-3); }
  .top { font-size: 14px; color: var(--ink-2); }
  .top .good, .fit-signals .good { color: var(--good); }
  .top .bad, .fit-signals .bad { color: var(--bad); }
  .fit-signals { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 6px 24px; }
  .fit-signals li { display: grid; grid-template-columns: minmax(0, 1fr) 28px 28px; gap: 8px; font-size: 13px; align-items: center; }
  .fit-signals li .num { text-align: right; }
  .fit-signals li.none { color: var(--ink-3); }
  .fit-signals li.none .num { color: var(--ink-3); }
  .fit-signals .for { color: var(--ink); }
  .fit-signals .against { color: var(--ink-2); }
  @media (max-width: 760px) { .fit-signals { grid-template-columns: minmax(0, 1fr); } }
  .cities { display: grid; gap: 10px; }
  .cities li { display: grid; grid-template-columns: 110px minmax(0, 1fr) 32px; gap: 12px; align-items: center; font-size: 14px; }
  .bar { height: 20px; background: var(--surface-2); border-radius: 5px; overflow: hidden; }
  .bar i { display: block; height: 100%; background: var(--series-1); border-radius: 5px; min-width: 3px; }
  .cities .num { text-align: right; }
  .run {
    display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 10px 14px; align-items: center; color: var(--ink);
    padding: 12px 14px; margin: -4px -6px; border-radius: 8px; text-decoration: none;
  }
  .run:hover { background: var(--surface-2); text-decoration: none; }
  .run-when { display: grid; gap: 2px; grid-column: 1; }
  .run-state { grid-column: 2; grid-row: 1; align-self: start; }
  .run-out { grid-column: 1 / span 2; min-width: 0; }
  .run-nums { grid-column: 1 / span 2; display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; margin: 0; }
  .run-nums div { display: grid; gap: 2px; }
  .run-nums dt { font-size: 12px; color: var(--ink-3); }
  .run-nums dd { margin: 0; font-size: 20px; }
  .run-nums .err dd { color: var(--bad); }
  @media (max-width: 760px) {
    .stats { gap: 8px; }
    .stat { padding: 12px; }
    .value { font-size: 24px; }
    .note { display: none; }
    .run-nums { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .cities li { grid-template-columns: 96px minmax(0, 1fr) 28px; }
  }
</style>
