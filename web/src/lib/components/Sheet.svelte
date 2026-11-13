<script lang="ts">
  import type { Snippet } from 'svelte'
  import Icon from './Icon.svelte'

  // A panel that slides in from the right, over the window.
  // Mount it to open it. It closes on Escape, the scrim and the close button.
  let {
    label,
    onclose,
    head,
    children,
  }: { label: string; onclose: () => void; head?: Snippet; children: Snippet } = $props()

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

  // Escape closes from anywhere, even when focus left the sheet.
  function windowKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && !e.defaultPrevented) {
      e.preventDefault()
      onclose()
    }
  }

  function keydown(e: KeyboardEvent) {
    if (e.key !== 'Tab' || !panel) return
    // Keep keyboard focus inside the sheet.
    const items = [...panel.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), input, select, textarea, [tabindex]:not([tabindex="-1"])')]
    if (!items.length) return
    const first = items[0]
    const last = items[items.length - 1]
    if (e.shiftKey && (document.activeElement === first || document.activeElement === panel)) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first.focus()
    }
  }
</script>

<svelte:window onkeydown={windowKeydown} />

<div class="scrim" onclick={onclose} aria-hidden="true"></div>
<div class="sheet" role="dialog" aria-modal="true" aria-label={label} tabindex="-1" bind:this={panel} onkeydown={keydown}>
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
    border-left: 1px solid var(--line); box-shadow: var(--shadow-lg); overflow-y: auto; overscroll-behavior: contain;
    display: flex; flex-direction: column; outline: none;
    animation: slide-in .26s cubic-bezier(.2, .8, .2, 1);
  }
  /* The bar a sheet starts with is every pane's bar: the cross at its end
     stands where the content's right edge is. */
  .sheet-bar { position: sticky; top: 0; z-index: 1; background: var(--surface); border-bottom-color: transparent; }
  .head-body { flex: 1 1 auto; min-width: 0; }
  .sheet-body { display: grid; gap: 22px; min-width: 0; padding: 8px var(--pad) calc(32px + env(safe-area-inset-bottom, 0px)); }
  @keyframes fade { from { opacity: 0; } }
  @keyframes slide-in { from { transform: translateX(48px); opacity: 0; } }
</style>
