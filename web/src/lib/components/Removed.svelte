<script lang="ts">
  // A picture removed from a report, in the tile it had, saying so and
  // offering itself back for ten seconds, in the one fill the app has,
  // running down to nothing, see Busy. Then onend, and the grid closes
  // over it. Ported from FrameFairy's Removed.svelte, which a clip's row
  // wears there too. Nothing vanishes under the pointer.
  import Busy from './Busy.svelte'
  import Icon from './Icon.svelte'

  let { onputback, onend }: { onputback?: () => void; onend?: () => void } = $props()

  // How long it can be put back, in seconds, FrameFairy's secondThoughts.
  const secondThoughts = 10
</script>

<div class="gone">
  <Icon name="trash" />
  <span class="what">Removed</span>
  <button class="back" type="button" onclick={() => onputback?.()}>Put it back</button>
  <Busy drain={secondThoughts} {onend} />
</div>

<style>
  /* It fills the tile, the words over each other in its middle, in the
     red of what was taken away. */
  .gone {
    position: absolute; inset: 0; isolation: isolate; overflow: hidden;
    display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 6px;
    border-radius: 8px; background: var(--bad-soft); color: var(--bad);
    --lit: var(--bad);
    --wash-from: color-mix(in srgb, var(--bad) 12%, transparent);
    --wash-to: color-mix(in srgb, var(--bad) 24%, transparent);
  }
  .what { font-weight: 600; }
  .back {
    border: 0; background: none; padding: 2px 6px; border-radius: 6px; font: inherit; font-size: 13px;
    color: var(--ink); cursor: pointer; text-decoration: underline; text-underline-offset: 3px;
  }
  .back:hover { background: color-mix(in srgb, var(--ink) 6%, transparent); }
</style>
