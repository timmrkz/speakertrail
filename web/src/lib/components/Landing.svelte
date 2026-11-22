<script lang="ts">
  // It holds nothing: what lights it is a class Wails puts on the page,
  // for files, or lit, for links the page itself takes, see PrivateApp.
  let { lit = false }: { lit?: boolean } = $props()
</script>

<!-- Where files dragged over the report window would land: all of it.
     The Wails runtime puts file-drop-target-active on the page while a
     file is held over it, so this is lit for exactly as long as a drop
     would take something, and nothing here has to follow the drag.
     Ported from FrameFairy's Landing.svelte. -->
<div class="landing" class:lit aria-hidden="true"></div>

<style>
  /* A frame round the whole window with a gap to its edge, so its corners
     stay clear of the window's own rounded corners, and the wash of the
     accent inside it. Never in the way of the pointer. */
  .landing {
    position: fixed; inset: var(--gap); z-index: 999; pointer-events: none;
    border: 2px solid var(--accent); border-radius: var(--radius); background: var(--accent-soft);
    opacity: 0; transition: opacity 120ms;
  }
  :global(:root.file-drop-target-active) .landing, .landing.lit { opacity: 1; }
</style>
