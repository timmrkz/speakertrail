<script lang="ts">
  // What went wrong at a control, said where the click was, see
  // lib/problem.svelte.ts. Under a row of buttons or a field it is a line
  // of red words, as a field typed wrong says so. Over a text box it lies
  // on the box's top on the box's own ground with an edge in red, so it
  // reads over the words and covers no more than its line, the way a copy
  // the Mac refused first said so in #60. A failure the person cannot put
  // right offers Report, which opens Report a Problem with its words in
  // it, as a toast did. The report window's own problems are about the
  // report, so they offer none.
  import type { Snippet } from 'svelte'
  import { api } from '../api'
  import type { Problem } from '../problem.svelte'
  import { reporting, reportTitle } from '../reporting.svelte'

  let { of, over = false, id, children }: {
    of: Problem
    // Over the top of the text box it is in, which sets position.
    over?: boolean
    id?: string
    // Words that say it better than the app's, like a link to where it
    // is put right, drawn in their place.
    children?: Snippet
  } = $props()

  function reportIt() {
    const words = of.text
    of.clear()
    api.reportAbout(words).catch((e) => of.say(e))
  }
</script>

{#if of.text}
  <p class="field-error problem" class:over {id} role="alert">
    {#if children}{@render children()}{:else}{of.text}{/if}{#if of.report}{' '}<button class="report" type="button" disabled={reporting.open} title={reportTitle()} onclick={reportIt}>Report</button>{/if}
  </p>
{/if}

<style>
  .problem { margin: 0; overflow-wrap: anywhere; }
  .over {
    position: absolute; top: 4px; left: 4px; right: var(--problem-end, 4px); z-index: 1; padding: 5px 9px;
    background: var(--surface); border: var(--line-w) solid var(--bad); border-radius: 6px;
  }
  /* Report reads as a word of the line, underlined, as the action on a
     toast does. Greyed out, it is its own words at less strength. */
  .report {
    background: none; border: 0; padding: 0; font: inherit; color: inherit;
    cursor: pointer; text-decoration: underline; text-underline-offset: 3px;
  }
  .report:disabled { color: color-mix(in srgb, currentColor 60%, transparent); cursor: default; }
</style>
