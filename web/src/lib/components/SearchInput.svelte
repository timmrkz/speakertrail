<script lang="ts">
  import Icon from './Icon.svelte'

  // A search field that reports its value after a short pause in typing.
  let {
    value = $bindable(''),
    placeholder = 'Search',
    label = 'Search',
    onsearch,
    onenter,
    title,
  }: { value?: string; placeholder?: string; label?: string; onsearch?: (v: string) => void; onenter?: (v: string) => void; title?: string } = $props()

  let timer: ReturnType<typeof setTimeout> | undefined

  function input(e: Event) {
    value = (e.currentTarget as HTMLInputElement).value
    clearTimeout(timer)
    timer = setTimeout(() => onsearch?.(value), 250)
  }
</script>

<div class="search">
  <Icon name="search" />
  <input class="input" type="search" {placeholder} aria-label={label} {title} {value} oninput={input} autocomplete="off" spellcheck="false"
    onkeydown={(e) => {
      // Enter does what the search holds, see People, at once.
      if (e.key !== 'Enter' || !onenter || e.isComposing) return
      e.preventDefault()
      clearTimeout(timer)
      onsearch?.(value)
      onenter(value)
    }} />
</div>
