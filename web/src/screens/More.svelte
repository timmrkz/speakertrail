<script lang="ts">
  import Icon, { type IconName } from '../lib/components/Icon.svelte'
  import ThemeButton from '../lib/components/ThemeButton.svelte'

  // The phone tab bar has room for four screens. The rest live here.
  let { logout }: { logout: () => void } = $props()

  const LINKS: { href: string; label: string; text: string; icon: IconName }[] = [
    { href: '/app/runs', label: 'Runs', text: 'Nightly runs and each check', icon: 'runs' },
    { href: '/app/seeds', label: 'Seeds', text: 'Tips for the crawler', icon: 'seeds' },
    { href: '/app/settings', label: 'Settings', text: 'How the crawler behaves', icon: 'settings' },
    { href: '/', label: 'Public calendar', text: 'What everyone else sees', icon: 'calendar' },
  ]

  $effect(() => {
    document.title = 'More · Speaker Trail'
  })
</script>

<div class="view narrow">
  <h1>More</h1>
  <ul class="list">
    {#each LINKS as l (l.href)}
      <li>
        <a class="row-btn item" href={l.href}>
          <span class="ic"><Icon name={l.icon} /></span>
          <span class="t"><b>{l.label}</b><span class="faint small">{l.text}</span></span>
          <Icon name="chevron" />
        </a>
      </li>
    {/each}
    <li>
      <button class="row-btn item" type="button" onclick={logout}>
        <span class="ic"><Icon name="logout" /></span>
        <span class="t"><b>Log out</b><span class="faint small">End this session on this device</span></span>
      </button>
    </li>
  </ul>
  <div class="theme"><ThemeButton withLabel /></div>
</div>

<style>
  .item { grid-template-columns: 36px minmax(0, 1fr) auto; color: var(--ink); }
  .item > :global(svg:last-child) { color: var(--ink-3); }
  .ic { width: 36px; height: 36px; border-radius: 10px; background: var(--surface-2); color: var(--ink-2); display: grid; place-items: center; }
  .t { display: grid; }
  .theme { justify-self: start; }
</style>
