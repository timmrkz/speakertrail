<script lang="ts">
  import { api, type Setting } from '../lib/api'
  import { Load } from '../lib/load.svelte'
  import { toast } from '../lib/toast.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import SearchInput from '../lib/components/SearchInput.svelte'
  import SettingRow from '../lib/components/SettingRow.svelte'
  import Skeleton from '../lib/components/Skeleton.svelte'
  import ThemeButton from '../lib/components/ThemeButton.svelte'

  // The phone has no sidebar, so logging out and the theme live here.
  let { logout }: { logout: () => void } = $props()

  const settings = new Load<Setting[]>()
  settings.run(api.settings)

  let q = $state('')

  $effect(() => {
    document.title = 'Settings · Speaker Trail'
  })

  // What profile searches use comes first. The rest belongs to the parts of
  // the engine that are paused, and waits behind one click.
  const MAIN = [
    'profile_searches_per_run',
    'profile_search_results',
    'profile_search_check_days',
    'profile_active_days',
    'exa_monthly_searches',
    'profile_searches_only',
  ]
  let matches = $derived(
    (settings.data ?? []).filter((s) => !q || `${s.key} ${s.description}`.toLowerCase().includes(q.toLowerCase())),
  )
  let main = $derived(MAIN.map((k) => matches.find((s) => s.key === k)).filter((s): s is Setting => !!s))
  let rest = $derived(matches.filter((s) => !MAIN.includes(s.key)))

  function saved(s: Setting) {
    if (settings.data) settings.data = settings.data.map((x) => (x.key === s.key ? s : x))
    toast.show(`Saved ${s.key}`)
  }
</script>

<div class="view">
  <div class="view-head">
    <div>
      <h1>Settings</h1>
      <p>Each row saves on its own.</p>
    </div>
  </div>

  <SearchInput bind:value={q} placeholder="Search settings" label="Search settings" />

  {#if settings.error && !settings.data}
    <EmptyState icon="alert" tone="bad" title="Could not load the settings" text={settings.error}>
      <button class="btn" type="button" onclick={() => settings.run(api.settings)}><Icon name="refresh" />Try again</button>
    </EmptyState>
  {:else if !settings.data}
    <Skeleton count={6} />
  {:else if !matches.length}
    <EmptyState icon="settings" title="No setting matches" text="Try another word." />
  {:else}
    {#if main.length}
      <section class="group">
        <h2>Profile searches</h2>
        <div class="list">
          {#each main as s (s.key)}
            <SettingRow setting={s} onsaved={saved} />
          {/each}
        </div>
      </section>
    {/if}
    {#if rest.length}
      <details class="group" open={q !== ''}>
        <summary><h2>The paused engine</h2><span class="faint small num">{rest.length}</span></summary>
        <div class="list">
          {#each rest as s (s.key)}
            <SettingRow setting={s} onsaved={saved} />
          {/each}
        </div>
      </details>
    {/if}
  {/if}

  <div class="account">
    <a class="btn" href="/"><Icon name="calendar" />Calendar</a>
    <ThemeButton withLabel />
    <button class="btn" type="button" onclick={logout}><Icon name="logout" />Log out</button>
  </div>
</div>

<style>
  .group { display: grid; gap: 10px; }
  .group h2 { font-size: 15px; margin: 0; }
  summary { display: flex; align-items: baseline; gap: 8px; cursor: pointer; list-style: revert; }
  summary h2 { display: inline; }
  details[open] > summary { margin-bottom: 10px; }
  /* The sidebar holds these on wide screens. */
  .account { display: none; flex-wrap: wrap; gap: 8px; }
  @media (max-width: 760px) { .account { display: flex; } }
</style>
