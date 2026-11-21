<script lang="ts">
  // A small mark that explains one thing when you want it explained,
  // instead of a line of help on screen at all times. Ported from
  // FrameFairy's Info.svelte, in Speaker Trail's tokens.
  import type { Snippet } from 'svelte'
  import Icon from './Icon.svelte'
  import { placeBubble, widths, type Place } from '../bubble'

  let {
    label,
    side = 'left',
    children,
  }: {
    label: string
    // Which side of the mark the bubble grows from, so it stays on screen.
    side?: 'left' | 'right'
    children: Snippet
  } = $props()

  let open = $state(false)
  let nearMark = $state(false)
  let nearBubble = $state(false)
  let root: HTMLSpanElement
  let bubble = $state<HTMLSpanElement>()
  // The bubble is only in the page while it is up.
  const shown = $derived(open || nearMark || nearBubble)

  // The bubble hangs from the page itself rather than from the mark, so
  // nothing the mark stands in can clip it and nothing can lie over it.
  function loose(node: HTMLElement) {
    document.body.appendChild(node)
    return {
      destroy() {
        node.remove()
      },
    }
  }

  // Where it goes is worked out in lib/bubble.ts: under the mark, over
  // it or beside it, the first that holds all of it. It stays out of
  // sight until it has been measured and put somewhere.
  let at = $state<Place | null>(null)

  function place(node: HTMLElement): Place {
    return placeBubble(root.getBoundingClientRect(), { width: window.innerWidth, height: window.innerHeight }, side, (width) => {
      node.style.width = `${width}px`
      node.style.maxHeight = 'none'
      const height = node.getBoundingClientRect().height
      node.style.maxHeight = ''
      return height
    })
  }

  $effect(() => {
    if (!shown || !bubble) {
      at = null
      return
    }
    const node = bubble
    at = place(node)
    const again = () => (at = place(node))
    window.addEventListener('resize', again)
    return () => window.removeEventListener('resize', again)
  })

  // Opened by a click, it stays until you click somewhere else or press
  // Escape. The text inside can be read and copied for as long as it does.
  $effect(() => {
    if (!open) return
    const away = (event: PointerEvent) => {
      const on = event.target as Node
      if (!root.contains(on) && !bubble?.contains(on)) open = false
    }
    const key = (event: KeyboardEvent) => {
      if (event.key === 'Escape') open = false
    }
    window.addEventListener('pointerdown', away)
    window.addEventListener('keydown', key)
    return () => {
      window.removeEventListener('pointerdown', away)
      window.removeEventListener('keydown', key)
    }
  })
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<span class="info" bind:this={root} onpointerenter={() => (nearMark = true)} onpointerleave={() => (nearMark = false)}>
  <button class="mark" type="button" aria-label={label} aria-expanded={shown} onclick={() => (open = !open)}>
    <Icon name="info" size={15} />
  </button>
</span>
{#if shown}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <span
    class="bubble"
    class:placed={!!at}
    class:above={at?.where === 'above'}
    class:beside={at?.where === 'beside'}
    class:toleft={side === 'right'}
    class:scroll={at?.scroll}
    bind:this={bubble}
    use:loose
    style="top: {at?.top ?? 0}px; left: {at?.left ?? 0}px; width: {at?.width ?? widths[0]}px"
    onpointerenter={() => (nearBubble = true)}
    onpointerleave={() => (nearBubble = false)}>{@render children()}</span>
{/if}

<style>
  /* Exactly as tall as the mark, so it stands on whole pixels. */
  .info { position: relative; display: inline-flex; align-items: center; height: 20px; }
  .mark {
    display: flex; align-items: flex-start; justify-content: flex-start; width: 20px; height: 20px;
    padding: 3px 3px 2px 2px; border: none; border-radius: 50%; background: transparent; color: var(--ink-3); cursor: pointer;
  }
  .mark:hover, .info:hover .mark { color: var(--ink); }
  /* Over everything, because it is the answer to a question just asked.
     The gap between the mark and the bubble belongs to the bubble, so
     moving the pointer into it does not lose the hover. */
  .bubble {
    position: fixed; z-index: 200; padding: 8px 10px; background: var(--surface); border: 1px solid var(--line-strong);
    border-radius: var(--radius); color: var(--ink); font-size: 13px; line-height: 1.5;
    text-align: left; white-space: normal; cursor: text; user-select: text; visibility: hidden;
  }
  .bubble.scroll { max-height: calc(100vh - 16px); overflow: auto; }
  .bubble.placed { visibility: visible; }
  .bubble::before { content: ''; position: absolute; left: 0; right: 0; top: -8px; height: 8px; }
  .bubble.above::before { top: auto; bottom: -8px; }
  .bubble.beside::before { top: 0; bottom: 0; height: auto; left: auto; right: -8px; width: 8px; }
  .bubble.beside:not(.toleft)::before { right: auto; left: -8px; }
</style>
