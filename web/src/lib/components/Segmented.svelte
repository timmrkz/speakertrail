<script lang="ts" generics="T extends string">
  let {
    options,
    value = $bindable(),
    label,
    onchange,
  }: { options: { value: T; label: string }[]; value: T; label: string; onchange?: (v: T) => void } = $props()
</script>

<div class="segmented" role="group" aria-label={label}>
  {#each options as o (o.value)}
    <button
      type="button"
      aria-pressed={value === o.value}
      onclick={() => {
        value = o.value
        onchange?.(o.value)
      }}>{o.label}</button>
  {/each}
</div>
