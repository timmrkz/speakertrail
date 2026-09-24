<script lang="ts">
  import type { Profile } from '../api'
  import { PLATFORM_LABEL, PLATFORM_SHORT } from '../format'

  // Small platform tags. Confirmed ones are green, rejected ones struck out.
  let { profiles }: { profiles: Profile[] } = $props()
  const REVIEW = { open: 'not checked yet', confirmed: 'confirmed', rejected: 'rejected' }
</script>

{#if profiles.length}
  <span class="badges">
    {#each profiles as p (p.id)}
      <span class="plat" class:ok={p.review === 'confirmed'} class:no={p.review === 'rejected'} title="{PLATFORM_LABEL[p.platform]}, {REVIEW[p.review]}">
        {PLATFORM_SHORT[p.platform]}<span class="sr-only">, {PLATFORM_LABEL[p.platform]} {REVIEW[p.review]}</span>
      </span>
    {/each}
  </span>
{/if}

<style>
  .badges { display: inline-flex; gap: 4px; flex-wrap: nowrap; }
</style>
