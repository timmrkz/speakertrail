<script lang="ts">
  import { navigate, router } from '../lib/router.svelte'
  import { runWatch } from '../lib/run.svelte'
  import { railDot, updates } from '../lib/updates.svelte'
  import Brand from '../lib/components/Brand.svelte'
  import Icon, { type IconName } from '../lib/components/Icon.svelte'
  import ThemeButton from '../lib/components/ThemeButton.svelte'
  import People from './People.svelte'
  import Settings from './Settings.svelte'
  import Searches from './Searches.svelte'
  import Updates from './Updates.svelte'
  import NotFound from './NotFound.svelte'

  type Item = { href: string; label: string; icon: IconName }
  // Three screens: the people the profile searches found, with the Run
  // button, the searches themselves, and the settings. Events, sources and
  // runs are paused with the rest of the engine and left out.
  const NAV: Item[] = [
    { href: '/app/people', label: 'People', icon: 'people' },
    { href: '/app/searches', label: 'Searches', icon: 'search' },
    { href: '/app/settings', label: 'Settings', icon: 'settings' },
  ]

  // The app has no login: nothing outside it can reach its data.
  const ready = true

  // A going run shows as a pulsing dot on Runs, from the same watch every
  // screen uses.
  $effect(() => {
    if (ready) return runWatch.watch()
  })
  let going = $derived(runWatch.run !== null)

  // Where the app's updates stand, from the start, so Updates wears a dot
  // when a newer build is coming or ready, or something needs a look.
  $effect(() => {
    if (ready) return updates.watch()
  })
  let updateDot = $derived(railDot(updates.shown))

  // The workspace opens on People.
  $effect(() => {
    if (router.path === '/app' || router.path === '/app/') navigate('/app/people', { replace: true })
  })

  function current(href: string): boolean {
    return router.under(href)
  }

  // In the narrow window's tab bar Updates is reached from Settings, so
  // Settings is lit there too.
  function currentTab(href: string): boolean {
    return current(href) || (href === '/app/settings' && router.under('/app/updates'))
  }

</script>

{#if ready}
  <div class="app">
    <aside class="side" aria-label="Workspace">
      <Brand href="/app/people" sub="Workspace" />
      <nav class="nav" aria-label="Main">
        {#each NAV as item (item.href)}
          <a href={item.href} aria-current={current(item.href) ? 'page' : undefined}>
            <Icon name={item.icon} /><span>{item.label}</span>
            {#if item.href === '/app/people' && going}<span class="dot busy" title="A run is going"></span>{/if}
          </a>
        {/each}
      </nav>
      <div class="nav side-foot">
        <a href="/app/updates" aria-current={current('/app/updates') ? 'page' : undefined}>
          <Icon name="update" /><span>Updates</span>
          {#if updateDot}<span class="dot {updateDot}" title="See Updates"></span>{/if}
        </a>
        <ThemeButton withLabel />
      </div>
    </aside>

    <main id="main" class="main">
      {#if router.under('/app/people')}
        <People />
      {:else if router.under('/app/searches')}
        <Searches />
      {:else if router.path === '/app/settings'}
        <Settings />
      {:else if router.path === '/app/updates'}
        <Updates />
      {:else}
        <NotFound embedded />
      {/if}
    </main>

    <nav class="tabbar" aria-label="Main">
      {#each NAV as item (item.href)}
        <a href={item.href} aria-current={currentTab(item.href) ? 'page' : undefined}>
          <span class="tab-icon"><Icon name={item.icon} size={22} />{#if item.href === '/app/people' && going}<span class="dot busy" title="A run is going"></span>{:else if item.href === '/app/settings' && updateDot}<span class="dot {updateDot}" title="See Updates, in Settings"></span>{/if}</span><span>{item.label}</span>
        </a>
      {/each}
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
  .nav a, .side-foot :global(.btn) {
    display: flex; align-items: center; gap: 10px; width: 100%; height: 38px; padding: 0 10px; border: 0; border-radius: 8px;
    background: none; cursor: pointer; text-align: left; color: var(--ink-2); font-weight: 500; font-size: 14px; text-decoration: none;
    justify-content: flex-start;
  }
  .nav a:hover, .side-foot :global(.btn:hover) { background: var(--surface-2); color: var(--ink); text-decoration: none; }
  .nav a[aria-current="page"] { background: var(--accent-soft); color: var(--accent); font-weight: 600; }
  .side-foot { margin-top: auto; border-top: 1px solid var(--line); padding-top: 12px; }
  .main { padding: 28px var(--gutter) 64px; min-width: 0; }
  .tabbar { display: none; }
  .nav a .dot { margin-left: auto; }
  .tab-icon { position: relative; display: inline-grid; }
  .tab-icon .dot { position: absolute; top: 0; right: -3px; }

  @media (max-width: 760px) {
    .app { grid-template-columns: minmax(0, 1fr); }
    .side { display: none; }
    .main { padding: 16px var(--gutter) calc(88px + env(safe-area-inset-bottom, 0px)); }
    .tabbar {
      display: grid; grid-template-columns: repeat(3, 1fr); position: fixed; left: 0; right: 0; bottom: 0; z-index: 30;
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
