<script lang="ts">
  // Asks before something that cannot be taken back. A real modal, because
  // a click that throws work away has to be a click you meant. Ported from
  // FrameFairy's Confirm.svelte.
  import type { Snippet } from 'svelte'

  let {
    title,
    oncancel,
    children,
    actions,
  }: {
    title: string
    oncancel: () => void
    // What is about to happen, in plain words.
    children: Snippet
    // The buttons, the one that does the thing last.
    actions: Snippet
  } = $props()

  let panel: HTMLDialogElement

  $effect(() => {
    if (!panel) return
    panel.showModal()
    // The safe answer is where the keyboard starts.
    panel.querySelector('button')?.focus()
  })

  // macOS only tabs between buttons when full keyboard access is on, so the
  // box moves the focus itself. The arrow keys do the same, because the
  // buttons sit in a row and that is what a row of buttons looks like.
  function move(event: KeyboardEvent) {
    const steps: Record<string, number> = {
      ArrowLeft: -1,
      ArrowUp: -1,
      ArrowRight: 1,
      ArrowDown: 1,
      Tab: event.shiftKey ? -1 : 1,
    }
    const step = steps[event.key]
    if (!step) return
    const buttons = [...panel.querySelectorAll('button')]
    if (buttons.length < 2) return
    event.preventDefault()
    const here = buttons.indexOf(document.activeElement as HTMLButtonElement)
    buttons[(here + step + buttons.length) % buttons.length].focus()
  }
</script>

<dialog bind:this={panel} onkeydown={move} oncancel={oncancel} onclose={oncancel}>
  <h2>{title}</h2>
  <div class="body">{@render children()}</div>
  <div class="buttons">
    {@render actions()}
  </div>
</dialog>

<style>
  dialog {
    width: min(440px, calc(100vw - 32px));
    padding: 18px 20px 16px;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius);
    background: var(--surface);
    color: var(--ink);
  }
  dialog::backdrop { background: var(--scrim); }
  h2 { font-size: 15px; font-weight: 600; margin: 0 0 8px; }
  .body { color: var(--ink-2); margin-bottom: 16px; overflow-wrap: anywhere; }
  .body :global(p) { margin: 0; }
  .buttons { display: flex; justify-content: flex-end; gap: 8px; }
</style>
