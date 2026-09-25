<script lang="ts">
  import { theme } from '../theme.svelte'
  import Icon from './Icon.svelte'

  let { withLabel = false }: { withLabel?: boolean } = $props()
  let icon = $derived(theme.choice === 'dark' ? 'moon' : theme.choice === 'light' ? 'sun' : 'auto') as 'moon' | 'sun' | 'auto'
</script>

<button
  type="button"
  class="btn quiet"
  class:icon={!withLabel}
  onclick={() => theme.cycle()}
  aria-label={withLabel ? undefined : `${theme.label}. Switch theme`}
  title={theme.label}>
  <Icon name={icon} />
  {#if withLabel}<span>{theme.label}</span>{/if}
</button>
