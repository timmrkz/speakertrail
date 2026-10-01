<script lang="ts">
  // How a run ended and what it brought, in one line. The one way a run's
  // outcome shows, in the list, in its sheet, on Overview and on People.
  import type { Run } from '../api'
  import { runBrought } from '../format'

  let { run }: { run: Run } = $props()
  let out = $derived(runBrought(run))
  let lead = $derived(run.state === 'stopped' ? 'Stopped by hand. ' : run.state === 'interrupted' ? 'Ended when the app restarted. ' : '')
</script>

<p class="outcome">
  {lead}{run.state === 'going' ? 'So far ' : ''}{out.brought}.
  {#if out.failed}<span class="failed">{out.failed}.</span>{:else if run.state !== 'going'}<span class="faint">Nothing failed.</span>{/if}
</p>

<style>
  .outcome { font-size: 13px; color: var(--ink-2); min-width: 0; }
  .failed { color: var(--bad); font-weight: 500; }
</style>
