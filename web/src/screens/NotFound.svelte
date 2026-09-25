<script lang="ts">
  import EmptyState from '../lib/components/EmptyState.svelte'

  // `embedded` renders inside the workspace, which has its own <main>.
  let { embedded = false }: { embedded?: boolean } = $props()

  $effect(() => {
    document.title = 'Not found · Speaker Trail'
  })
</script>

{#snippet body()}
  <EmptyState icon="alert" title="This page does not exist" text="The link may be old or mistyped.">
    <a class="btn primary" href={embedded ? '/app' : '/'}>{embedded ? 'Go to the overview' : 'Go to the calendar'}</a>
  </EmptyState>
{/snippet}

{#if embedded}
  <div class="page">{@render body()}</div>
{:else}
  <main id="main" class="page">{@render body()}</main>
{/if}

<style>
  .page { max-width: 520px; margin: 0 auto; padding: 64px 16px; }
</style>
