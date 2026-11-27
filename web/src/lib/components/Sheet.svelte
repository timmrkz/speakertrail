<script lang="ts">
  import { moveTab } from '../keys'
  import type { Snippet } from 'svelte'
  import Icon from './Icon.svelte'

  // A panel that slides in from the right, over the window.
  // Mount it to open it. It closes on Escape, the scrim and the close button.
  let {
    label,
    onclose,
    head,
    children,
    personal = false,
  }: {
    label: string
    onclose: () => void
    head?: Snippet
    children: Snippet
    // The sheet shows a person, so the app's log names nothing in it by
    // what it says, its label neither, see control in lib/said.ts.
    personal?: boolean
  } = $props()

  let panel: HTMLElement | undefined = $state()

  $effect(() => {
    const before = document.activeElement as HTMLElement | null
    panel?.focus({ preventScroll: true })
    const html = document.documentElement
    const prev = html.style.overflow
    html.style.overflow = 'hidden'
    return () => {
      html.style.overflow = prev
      if (before && document.contains(before)) before.focus({ preventScroll: true })
    }
  })

  // Escape closes from anywhere, even when focus left the sheet, but not
  // while text is being composed: on a Mac Escape also ends Dictation,
  // and closing then would lose what was said, as Report.svelte knows.
  function windowKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && !e.defaultPrevented && !e.isComposing) {
      e.preventDefault()
      onclose()
    }
  }

  // Tab stays inside the sheet and reaches its buttons too, through
  // moveTab, as every window of the app does.
  function keydown(e: KeyboardEvent) {
    if (e.key !== 'Tab' || !panel || e.defaultPrevented) return
    moveTab(panel, e)
  }
</script>

<svelte:window onkeydown={windowKeydown} />

<div class="scrim" onclick={onclose} aria-hidden="true"></div>
<div class="sheet" data-personal={personal ? '' : undefined} role="dialog" aria-modal="true" aria-label={label} tabindex="-1" bind:this={panel} onkeydown={keydown}>
  <div class="bar sheet-bar">
    <div class="head-body">{@render head?.()}</div>
    <button class="btn quiet icon close flush-end" type="button" onclick={onclose} aria-label="Close" title="Close (Esc)"><Icon name="close" /></button>
  </div>
  <div class="sheet-body">
    {@render children()}
  </div>
</div>

<style>
  .scrim { position: fixed; inset: 0; background: var(--scrim); z-index: 40; animation: fade .18s ease-out; }
  .sheet {
    position: fixed; top: 0; right: 0; bottom: 0; width: min(560px, 100%); z-index: 41; background: var(--surface);
    border-left: 1px solid var(--line-strong); overflow-y: auto; overscroll-behavior: contain;
    display: flex; flex-direction: column; outline: none;
    animation: slide-in .26s cubic-bezier(.2, .8, .2, 1);
  }
  /* The bar a sheet starts with is every pane's bar: the cross at its end
     stands where the content's right edge is. */
  .sheet-bar { position: sticky; top: 0; z-index: 1; background: var(--surface); border-bottom-color: transparent; }
  .head-body { flex: 1 1 auto; min-width: 0; }
  /* The body stands a --gap-control under the bar, as a person's buttons
     stand under the name in the pane, and its sections --gap-page apart. */
  /* Under the last section the inset of the sides, so what fits never
     scrolls, issue #77. */
  .sheet-body { display: grid; gap: var(--gap-page); min-width: 0; padding: var(--gap-control) var(--pad) calc(var(--gap-page) + env(safe-area-inset-bottom, 0px)); }
  @keyframes fade { from { opacity: 0; } }
  @keyframes slide-in { from { transform: translateX(48px); opacity: 0; } }
</style>
