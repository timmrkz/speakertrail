<script lang="ts">
  let { href = '/', sub = "Who's on stage in NRW" }: { href?: string; sub?: string } = $props()
</script>

<a class="brand" {href}>
  <span class="brand-mark" aria-hidden="true">ST</span>
  <span class="brand-text"><b>Speaker Trail</b>{#if sub}<small>{sub}</small>{/if}</span>
</a>

<style>
  .brand { display: flex; align-items: center; gap: 10px; color: var(--ink); text-decoration: none; min-width: 0; }
  .brand:hover { text-decoration: none; }
  .brand-mark {
    width: 30px; height: 30px; border-radius: 8px; background: var(--accent); color: var(--accent-ink); flex: none;
    display: grid; place-items: center; font-weight: 700; font-size: 13px; letter-spacing: .02em;
  }
  .brand-text { display: grid; min-width: 0; line-height: 1.25; }
  b { font-size: 15px; }
  small { font-size: 12px; color: var(--ink-3); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
</style>
