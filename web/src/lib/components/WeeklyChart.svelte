<script lang="ts">
  import type { Stats } from '../api'
  import { fmtShortDayKey } from '../format'

  // New people and new events per week, grouped bars on one scale.
  let { weekly }: { weekly: Stats['weekly'] } = $props()

  const SERIES = [
    { key: 'people', label: 'New people', color: 'var(--series-1)' },
    { key: 'events', label: 'New events', color: 'var(--series-2)' },
  ] as const

  let width = $state(0)
  let active = $state<number | null>(null)

  const H = 200
  const M = { top: 22, right: 4, bottom: 26, left: 34 }

  function niceMax(v: number): number {
    if (v <= 4) return 4
    const step = Math.pow(10, Math.floor(Math.log10(v)))
    for (const f of [1, 2, 2.5, 5, 10]) if (f * step >= v) return f * step
    return 10 * step
  }

  let max = $derived(niceMax(Math.max(1, ...weekly.flatMap((w) => [w.people, w.events]))))
  let plotW = $derived(Math.max(0, width - M.left - M.right))
  let plotH = H - M.top - M.bottom
  let groupW = $derived(weekly.length ? plotW / weekly.length : 0)
  let barW = $derived(Math.max(4, Math.min(18, (groupW - 12) / 2 - 1)))
  let ticks = $derived([0, max / 2, max])
  let everyOther = $derived(groupW < 46)

  const y = (v: number) => M.top + plotH - (v / max) * plotH

  // A bar with 4px rounded top corners, flat on the baseline.
  function bar(x: number, v: number): string {
    const top = y(v)
    const h = M.top + plotH - top
    if (h <= 0) return ''
    const r = Math.min(4, h, barW / 2)
    return `M${x},${M.top + plotH}V${top + r}Q${x},${top} ${x + r},${top}H${x + barW - r}Q${x + barW},${top} ${x + barW},${top + r}V${M.top + plotH}Z`
  }

  let readout = $derived.by(() => {
    const i = active ?? weekly.length - 1
    const w = weekly[i]
    if (!w) return ''
    return `Week of ${fmtShortDayKey(w.week_start)}: ${w.people} new people, ${w.events} new events`
  })
</script>

<div class="chart">
  <div class="legend" aria-hidden="true">
    {#each SERIES as s (s.key)}
      <span><i style:background={s.color}></i>{s.label}</span>
    {/each}
  </div>
  <p class="readout" aria-live="polite">{readout}</p>
  <div class="plot" bind:clientWidth={width}>
    {#if width > 0}
      <svg {width} height={H} role="img" aria-label="New people and new events per week, last 8 weeks. The table below has the numbers.">
        {#each ticks as t (t)}
          <line x1={M.left} x2={width - M.right} y1={y(t)} y2={y(t)} class="grid" class:base={t === 0} />
          <text x={M.left - 8} y={y(t)} class="tick" text-anchor="end" dominant-baseline="middle">{t}</text>
        {/each}
        {#each weekly as w, i (w.week_start)}
          {@const gx = M.left + i * groupW}
          {@const x0 = gx + groupW / 2 - barW - 1}
          {#if active === i}
            <rect x={gx + 2} y={M.top - 6} width={groupW - 4} height={plotH + 6} rx="6" class="hover" />
          {/if}
          <path d={bar(x0, w.people)} fill="var(--series-1)" />
          <path d={bar(x0 + barW + 2, w.events)} fill="var(--series-2)" />
          {#if i === weekly.length - 1}
            <text x={x0 + barW / 2} y={y(w.people) - 6} class="val" text-anchor="middle">{w.people}</text>
            <text x={x0 + barW * 1.5 + 2} y={y(w.events) - 6} class="val" text-anchor="middle">{w.events}</text>
          {/if}
          {#if !everyOther || (weekly.length - 1 - i) % 2 === 0}
            <text x={gx + groupW / 2} y={H - 8} class="tick" text-anchor="middle">{fmtShortDayKey(w.week_start)}</text>
          {/if}
          <rect
            role="presentation"
            x={gx}
            y={M.top - 6}
            width={groupW}
            height={plotH + 6}
            fill="transparent"
            onpointerenter={() => (active = i)}
            onpointerleave={() => (active = null)} />
        {/each}
      </svg>
    {/if}
  </div>
  <table class="sr-only">
    <caption>New people and new events per week</caption>
    <thead><tr><th scope="col">Week of</th><th scope="col">New people</th><th scope="col">New events</th></tr></thead>
    <tbody>
      {#each weekly as w (w.week_start)}
        <tr><th scope="row">{fmtShortDayKey(w.week_start)}</th><td>{w.people}</td><td>{w.events}</td></tr>
      {/each}
    </tbody>
  </table>
</div>

<style>
  .chart { display: grid; gap: 6px; min-width: 0; }
  .legend { display: flex; gap: 14px; flex-wrap: wrap; font-size: 13px; color: var(--ink-2); }
  .legend span { display: inline-flex; align-items: center; gap: 6px; }
  .legend i { width: 10px; height: 10px; border-radius: 3px; display: inline-block; }
  .readout { font-size: 13px; color: var(--ink-2); min-height: 20px; font-variant-numeric: tabular-nums; }
  .plot { width: 100%; min-width: 0; height: 200px; }
  svg { display: block; overflow: visible; }
  .grid { stroke: var(--line); stroke-width: 1; stroke-dasharray: 2 3; }
  .grid.base { stroke: var(--line-strong); stroke-dasharray: none; }
  .tick { font: 11px var(--mono); fill: var(--ink-3); }
  .val { font: 500 11px var(--mono); fill: var(--ink-2); }
  .hover { fill: var(--surface-2); }
</style>
