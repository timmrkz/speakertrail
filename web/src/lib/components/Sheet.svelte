<script lang="ts">
  import type { Snippet } from 'svelte'
  import Icon from './Icon.svelte'

  // A bottom sheet on phones and a side panel on wide screens.
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
  <div class="grip" aria-hidden="true"></div>
  <div class="sheet-head">
    <div class="head-body">{@render head?.()}</div>
    <button class="btn icon close" type="button" onclick={onclose} aria-label="Close"><Icon name="close" /></button>
  </div>
  <div class="sheet-body">
    {@render children()}
  </div>
</div>

<style>
  .scrim { position: fixed; inset: 0; background: var(--scrim); z-index: 40; animation: fade .18s ease-out; }
  .sheet {
    position: fixed; top: 0; right: 0; bottom: 0; width: min(540px, 100%); z-index: 41; background: var(--surface);
    border-left: 1px solid var(--line); box-shadow: var(--shadow-lg); overflow-y: auto; overscroll-behavior: contain;
    padding: calc(18px + env(safe-area-inset-top, 0px)) 22px calc(28px + env(safe-area-inset-bottom, 0px));
    display: grid; gap: 18px; align-content: start; outline: none;
    animation: slide-in .22s cubic-bezier(.2, .8, .2, 1);
  }
  .grip { display: none; }
  .sheet-head {
    display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 12px; align-items: start;
  }
  .head-body { min-width: 0; }
  .sheet-body { display: grid; gap: 22px; min-width: 0; }
  @media (max-width: 760px) {
    .sheet {
      top: auto; left: 0; width: 100%; max-height: 90dvh; border-left: 0; border-top: 1px solid var(--line);
      border-radius: 18px 18px 0 0; padding: 8px 16px calc(24px + env(safe-area-inset-bottom, 0px));
      animation-name: slide-up;
    }
    .grip { display: block; width: 40px; height: 4px; border-radius: 4px; background: var(--line-strong); margin: 0 auto 2px; }
  }
  @keyframes fade { from { opacity: 0; } }
  @keyframes slide-in { from { transform: translateX(40px); opacity: 0; } }
  @keyframes slide-up { from { transform: translateY(40px); opacity: 0; } }
</style>
