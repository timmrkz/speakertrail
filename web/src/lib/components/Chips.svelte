<script lang="ts" generics="T extends string">
  // One choice from a row of chips. Scrolls sideways on phones.
  type Option = { value: T; label: string; count?: number }
  let {
    options,
    value = $bindable(),
    label,
    scroll = true,
    onchange,
  }: { options: Option[]; value: T; label: string; scroll?: boolean; onchange?: (v: T) => void } = $props()
</script>

<div class="chips" class:scroll role="group" aria-label={label}>
  {#each options as o (o.value)}
    <button
      type="button"
      class="chip"
      aria-pressed={value === o.value}
      onclick={() => {
        value = o.value
        onchange?.(o.value)
      }}>
      {o.label}
      {#if o.count !== undefined}<span class="num">{o.count}</span>{/if}
    </button>
  {/each}
</div>
