<script lang="ts" generics="T extends { id: number; starts_at: string }">
  import type { Snippet } from 'svelte'
  import { dayLabel, fmtDayKey, groupByDay, plural, todayKey } from '../format'

  // Events grouped by Berlin day under sticky day headers.
  let { items, card, top = 0 }: { items: T[]; card: Snippet<[T]>; top?: number } = $props()

  let today = todayKey()
  let groups = $derived(groupByDay(items))
</script>

<div class="days">
  {#each groups as g (g.key)}
    <section class="day" aria-label={fmtDayKey(g.key)}>
      <h2 class="day-head" style:top="{top}px">
        <span>{dayLabel(g.key, today)}</span>
        {#if dayLabel(g.key, today) !== fmtDayKey(g.key)}<span class="date">{fmtDayKey(g.key)}</span>{/if}
        <span class="count">{plural(g.items.length, 'event')}</span>
      </h2>
      <div class="cards">
        {#each g.items as item (item.id)}
          {@render card(item)}
        {/each}
      </div>
    </section>
  {/each}
</div>

<style>
  .days { display: grid; gap: 8px; min-width: 0; }
  .day { display: grid; gap: 8px; min-width: 0; }
  .day-head {
    position: sticky; z-index: 5; display: flex; align-items: baseline; gap: 8px;
    padding: 10px 2px 8px; font-size: 14px; font-weight: 700; background: var(--bg);
    box-shadow: 0 1px 0 var(--line);
  }
  .date { font-weight: 500; color: var(--ink-2); }
  .count { margin-left: auto; font-size: 12px; font-weight: 500; color: var(--ink-3); }
  .cards { display: grid; gap: 8px; min-width: 0; }
</style>
