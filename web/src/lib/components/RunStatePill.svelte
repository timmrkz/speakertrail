<script lang="ts">
  // A run's state as a pill, wide enough for the longest word, so a row
  // keeps its width as the state changes.
  import type { RunState } from '../api'
  import { RUN_STATE } from '../format'

  let { state }: { state: RunState } = $props()
  let s = $derived(RUN_STATE[state] ?? RUN_STATE.finished)
</script>

<span class="pill state {s.tone}">
  {#if state === 'going'}<span class="dot busy"></span>{:else}<span class="dot"></span>{/if}{s.label}
</span>

<style>
  .state { min-width: 7.6em; justify-content: center; }
</style>
