<script lang="ts">
  import { toast } from '../toast.svelte'
  import { api } from '../api'
  import { reporting, reportTitle } from '../reporting.svelte'

  // report says whether a toast about a failure the person cannot put
  // right offers Report here, see toast.report, which opens Report a
  // Problem with its words in it, the way FrameFairy's toasts do. The report window's own toasts are about the
  // report, so they offer none. There is one report at a time: while the
  // report window is open, Report is greyed out and says why.
  let { report = true }: { report?: boolean } = $props()

  function reportIt() {
    const words = toast.message
    toast.hide()
    api.reportAbout(words).catch(() => {})
  }
</script>

<div class="toast-region" role="status" aria-live="polite">
  {#if toast.message}
    <div class="toast" class:bad={toast.tone === 'bad'}>
      <span>{toast.message}</span>
      {#if toast.action}
        <!-- The action is taken before the toast goes: hiding it empties
             toast.action, and a const in the template reads it again. -->
        <button class="act" type="button" onclick={() => { const run = toast.action?.run; toast.hide(); run?.() }}>{toast.action.label}</button>
      {:else if report && toast.report}
        <button class="act" type="button" disabled={reporting.open} title={reportTitle()} onclick={reportIt}>Report</button>
      {/if}
    </div>
  {/if}
</div>

<style>
  .toast-region {
    position: fixed; left: 16px; right: 16px; bottom: calc(84px + env(safe-area-inset-bottom, 0px));
    z-index: 60; display: flex; justify-content: center; pointer-events: none;
  }
  .toast {
    background: var(--ink); color: var(--bg); padding: 10px 16px; border-radius: 8px; font-size: 14px; font-weight: 600;
    max-width: 480px; animation: pop .18s ease-out;
    display: flex; align-items: center; gap: 14px;
  }
  /* The action can be pressed, the rest of the region lets clicks through. */
  .act {
    pointer-events: auto; background: none; border: 0; padding: 0; font: inherit; color: inherit;
    cursor: pointer; text-decoration: underline; text-underline-offset: 3px; flex: none;
  }
  /* The toast is the ink with the page's colour for words, the other way
     round from everything else, so the grey of a button that cannot be
     pressed would not read on it. Its action greys out as its own words
     at less strength instead. */
  .act:disabled { color: color-mix(in srgb, currentColor 60%, transparent); cursor: default; }
  .toast.bad { background: var(--bad-soft); color: var(--bad); border: 1px solid var(--bad); }
  @media (min-width: 761px) { .toast-region { bottom: 28px; } }
  @keyframes pop { from { transform: translateY(8px); opacity: 0; } }
</style>
