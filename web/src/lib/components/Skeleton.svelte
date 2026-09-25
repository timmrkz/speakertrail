<script lang="ts">
  // Placeholder shapes while a list loads.
  let { variant = 'rows', count = 5 }: { variant?: 'events' | 'rows'; count?: number } = $props()
</script>

<div class="skeleton" aria-busy="true" aria-live="polite">
  <span class="sr-only">Loading</span>
  {#if variant === 'events'}
    <span class="skel day"></span>
    {#each Array(count) as _, i (i)}
      <div class="card">
        <span class="skel t"></span>
        <div class="lines">
          <span class="skel" style:width="{62 - (i % 3) * 12}%"></span>
          <span class="skel sm" style:width="{44 + (i % 2) * 16}%"></span>
          <span class="skel sm" style:width="30%"></span>
        </div>
      </div>
    {/each}
  {:else}
    <div class="list">
      {#each Array(count) as _, i (i)}
        <div class="line-row">
          <span class="skel dot"></span>
          <div class="lines">
            <span class="skel" style:width="{40 + (i % 3) * 10}%"></span>
            <span class="skel sm" style:width="{58 - (i % 2) * 14}%"></span>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .skeleton { display: grid; gap: 10px; }
  .day { width: 110px; height: 14px; margin: 6px 0 2px; }
  .card {
    display: grid; grid-template-columns: 52px minmax(0, 1fr); gap: 14px; padding: 16px;
    background: var(--surface); border: 1px solid var(--line); border-radius: var(--radius);
  }
  .t { height: 16px; width: 44px; }
  .lines { display: grid; gap: 8px; }
  .lines .skel { height: 14px; }
  .lines .skel.sm { height: 11px; }
  .line-row { display: grid; grid-template-columns: 36px minmax(0, 1fr); gap: 12px; padding: 14px 16px; align-items: center; }
  .dot { width: 36px; height: 36px; border-radius: 50%; }
</style>
