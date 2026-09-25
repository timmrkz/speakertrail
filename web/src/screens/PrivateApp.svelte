<script lang="ts">
  import { api } from '../lib/api'
  import { navigate, router } from '../lib/router.svelte'
  import { toast } from '../lib/toast.svelte'
  import Brand from '../lib/components/Brand.svelte'
  import Icon, { type IconName } from '../lib/components/Icon.svelte'
  import ThemeButton from '../lib/components/ThemeButton.svelte'
  import Overview from './Overview.svelte'
  import Events from './Events.svelte'
  import People from './People.svelte'
  import Sources from './Sources.svelte'
  import Runs from './Runs.svelte'
  import Seeds from './Seeds.svelte'
  import Settings from './Settings.svelte'
  import More from './More.svelte'
  import NotFound from './NotFound.svelte'

  type Item = { href: string; label: string; icon: IconName }
  const NAV: Item[] = [
    { href: '/app', label: 'Overview', icon: 'overview' },
    { href: '/app/events', label: 'Events', icon: 'events' },
    { href: '/app/people', label: 'People', icon: 'people' },
    { href: '/app/sources', label: 'Sources', icon: 'sources' },
    { href: '/app/runs', label: 'Runs', icon: 'runs' },
    { href: '/app/seeds', label: 'Seeds', icon: 'seeds' },
    { href: '/app/settings', label: 'Settings', icon: 'settings' },
  ]
  const TABS = NAV.slice(0, 4)

  // Check the session once. Any later 401 also lands on the login.
  let ready = $state(false)
  api
    .me()
    .then((m) => {
      if (m.logged_in) ready = true
      else navigate(`/login?next=${encodeURIComponent(router.path)}`, { replace: true })
    })
    .catch(() => {
      ready = true
    })

  function current(href: string): boolean {
    return href === '/app' ? router.path === '/app' : router.under(href)
  }
  let moreActive = $derived(['/app/runs', '/app/seeds', '/app/settings', '/app/more'].some((h) => router.under(h)))

  async function logout() {
    try {
      await api.logout()
    } catch {
      // The cookie may already be gone. Leave anyway.
    }
    toast.show('Logged out')
    navigate('/login', { replace: true })
  }
</script>

{#if ready}
  <div class="app">
    <aside class="side" aria-label="Workspace">
      <Brand href="/app" sub="Workspace" />
      <nav class="nav" aria-label="Main">
        {#each NAV as item (item.href)}
          <a href={item.href} aria-current={current(item.href) ? 'page' : undefined}><Icon name={item.icon} /><span>{item.label}</span></a>
        {/each}
      </nav>
      <div class="nav side-foot">
        <a href="/"><Icon name="calendar" /><span>Calendar</span></a>
        <button type="button" onclick={logout}><Icon name="logout" /><span>Log out</span></button>
        <ThemeButton withLabel />
      </div>
    </aside>

    <main id="main" class="main">
      {#if router.path === '/app'}
        <Overview />
      {:else if router.under('/app/events')}
        <Events />
      {:else if router.under('/app/people')}
        <People />
      {:else if router.under('/app/sources')}
        <Sources />
      {:else if router.under('/app/runs')}
        <Runs />
      {:else if router.path === '/app/seeds'}
        <Seeds />
      {:else if router.path === '/app/settings'}
        <Settings />
      {:else if router.path === '/app/more'}
        <More {logout} />
      {:else}
        <NotFound embedded />
      {/if}
    </main>

    <nav class="tabbar" aria-label="Main">
      {#each TABS as item (item.href)}
        <a href={item.href} aria-current={current(item.href) ? 'page' : undefined}><Icon name={item.icon} size={22} /><span>{item.label}</span></a>
      {/each}
      <a href="/app/more" aria-current={moreActive ? 'page' : undefined}><Icon name="more" size={22} /><span>More</span></a>
    </nav>
  </div>
{/if}

<style>
  .app { display: grid; grid-template-columns: 224px minmax(0, 1fr); min-height: 100dvh; }
  .side {
    position: sticky; top: 0; height: 100dvh; border-right: 1px solid var(--line); background: var(--surface);
    padding: 18px 12px; display: flex; flex-direction: column; gap: 22px; overflow-y: auto;
  }
  .side > :global(.brand) { padding: 0 8px; }
  .nav { display: grid; gap: 2px; }
  .nav a, .nav button, .side-foot :global(.btn) {
    display: flex; align-items: center; gap: 10px; width: 100%; height: 38px; padding: 0 10px; border: 0; border-radius: 8px;
    background: none; cursor: pointer; text-align: left; color: var(--ink-2); font-weight: 500; font-size: 14px; text-decoration: none;
    justify-content: flex-start;
  }
  .nav a:hover, .nav button:hover { background: var(--surface-2); color: var(--ink); text-decoration: none; }
  .nav a[aria-current="page"] { background: var(--accent-soft); color: var(--accent); font-weight: 600; }
  .side-foot { margin-top: auto; border-top: 1px solid var(--line); padding-top: 12px; }
  .main { padding: 28px var(--gutter) 64px; min-width: 0; }
  .tabbar { display: none; }

  @media (max-width: 760px) {
    .app { grid-template-columns: minmax(0, 1fr); }
    .side { display: none; }
    .main { padding: 16px var(--gutter) calc(88px + env(safe-area-inset-bottom, 0px)); }
    .tabbar {
      display: grid; grid-template-columns: repeat(5, 1fr); position: fixed; left: 0; right: 0; bottom: 0; z-index: 30;
      background: var(--surface); border-top: 1px solid var(--line);
      padding: 4px 4px calc(4px + env(safe-area-inset-bottom, 0px));
    }
    .tabbar a {
      display: grid; justify-items: center; gap: 2px; padding: 6px 0 4px; border-radius: 8px;
      font-size: 11px; font-weight: 600; color: var(--ink-3); text-decoration: none;
    }
    .tabbar a[aria-current="page"] { color: var(--accent); }
  }
</style>
