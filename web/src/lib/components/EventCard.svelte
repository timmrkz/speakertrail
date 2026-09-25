<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { EventPerson, PublicEvent } from '../api'
  import { dayKey, fmtDay, fmtTime, ROLE_LABEL, TYPE_LABEL } from '../format'
  import Icon from './Icon.svelte'

  // One event, compact. The private screens add a footer and person links.
  let {
    event,
    personHref,
    muted = false,
    footer,
  }: {
    event: PublicEvent
    personHref?: (p: EventPerson) => string | undefined
    muted?: boolean
    footer?: Snippet
  } = $props()

  const SHOWN = 4
  let expanded = $state(false)
  let people = $derived(expanded ? event.people : event.people.slice(0, SHOWN))
  let hidden = $derived(event.people.length - people.length)

  let until = $derived.by(() => {
    if (!event.ends_at) return ''
    if (dayKey(event.ends_at) === dayKey(event.starts_at)) return `to ${fmtTime(event.ends_at)}`
    return `to ${fmtDay(event.ends_at)}`
  })
  let place = $derived([event.venue, event.city].filter(Boolean).join(', '))
</script>

<article class="event" class:muted aria-labelledby="ev-{event.id}">
  <div class="when">
    <time class="num" datetime={event.starts_at}>{fmtTime(event.starts_at)}</time>
    {#if until}<span class="until">{until}</span>{/if}
  </div>
  <div class="body">
    <div class="top">
      <h3 id="ev-{event.id}">
        {#if event.url}
          <a href={event.url} target="_blank" rel="noopener noreferrer">{event.title}<span class="sr-only"> (opens the event page)</span><Icon name="external" size={13} /></a>
        {:else}
          {event.title}
        {/if}
      </h3>
      <span class="pill type-{event.type}">{TYPE_LABEL[event.type] ?? event.type}</span>
    </div>
    {#if place}
      <p class="place"><Icon name="pin" size={14} /><span class="ellipsis" title={event.address || place}>{place}</span></p>
    {/if}
    {#if event.organisers.length || event.format === 'hybrid' || event.price}
      <p class="meta">
        {#if event.organisers.length}<span>by {event.organisers.join(', ')}</span>{/if}
        {#if event.format === 'hybrid'}<span class="pill line">Hybrid</span>{/if}
        {#if event.price}<span class="price">{event.price}</span>{/if}
      </p>
    {/if}
    {#if event.people.length}
      <ul class="people" aria-label="People on stage">
        {#each people as p, i (i)}
          {@const href = personHref?.(p)}
          {@const role = `${ROLE_LABEL[p.role] ?? p.role}${p.affiliation ? `, ${p.affiliation}` : ''}`}
          <li>
            {#if href}<a {href} class="name">{p.name}</a>{:else}<span class="name">{p.name}</span>{/if}
            <span class="role" title={role}>{role}</span>
          </li>
        {/each}
        {#if hidden > 0}
          <li><button type="button" class="more" onclick={() => (expanded = true)}>and {hidden} more</button></li>
        {/if}
      </ul>
    {/if}
    {#if footer}<div class="footer">{@render footer()}</div>{/if}
  </div>
</article>

<style>
  .event {
    display: grid; grid-template-columns: 56px minmax(0, 1fr); gap: 14px; padding: 14px 16px;
    background: var(--surface); border: 1px solid var(--line); border-radius: var(--radius); min-width: 0;
  }
  .event.muted { background: var(--surface-2); }
  .event.muted h3, .event.muted .place { color: var(--ink-2); }
  .when { display: grid; align-content: start; gap: 2px; padding-top: 1px; }
  .when time { font-size: 15px; font-weight: 500; }
  .until { font-size: 12px; color: var(--ink-3); line-height: 1.3; }
  .body { display: grid; gap: 4px; min-width: 0; }
  .top { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; }
  h3 { font-size: 16px; font-weight: 600; line-height: 1.3; min-width: 0; overflow-wrap: anywhere; }
  h3 a { color: var(--ink); }
  h3 a :global(svg) { margin-left: 5px; vertical-align: -1px; color: var(--ink-3); }
  h3 a:hover { color: var(--accent); }
  h3 a:hover :global(svg) { color: var(--accent); }
  .place { display: flex; align-items: center; gap: 5px; color: var(--ink-2); font-size: 14px; min-width: 0; }
  .place :global(svg) { color: var(--ink-3); flex: none; }
  .meta { display: flex; flex-wrap: wrap; gap: 4px 10px; align-items: center; font-size: 13px; color: var(--ink-3); }
  .price { color: var(--ink-2); }
  .people { display: grid; gap: 2px; margin-top: 6px; padding-top: 8px; border-top: 1px dashed var(--line); font-size: 14px; }
  .people li { min-width: 0; display: flex; align-items: baseline; gap: 6px; }
  .name { font-weight: 600; color: var(--ink); flex: none; max-width: 70%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  a.name { color: var(--accent); }
  .role { color: var(--ink-3); min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .more {
    border: 0; background: none; padding: 0; font: inherit; font-size: 13px; font-weight: 600; color: var(--accent); cursor: pointer;
  }
  .footer { margin-top: 8px; padding-top: 10px; border-top: 1px solid var(--line); display: grid; gap: 10px; }
  .pill.type-pitch { background: var(--accent-soft); color: var(--accent); }
  .pill.type-sport { background: var(--good-soft); color: var(--good); }
  .pill.type-conference, .pill.type-workshop { background: var(--warn-soft); color: var(--warn); }
  @media (max-width: 520px) {
    .event { grid-template-columns: 46px minmax(0, 1fr); gap: 10px; padding: 12px 14px; }
    h3 { font-size: 15px; }
  }
</style>
