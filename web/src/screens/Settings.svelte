<script lang="ts">
  import { api, type Setting } from '../lib/api'
  import { Load } from '../lib/load.svelte'
  import { toast } from '../lib/toast.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import SearchInput from '../lib/components/SearchInput.svelte'
  import KeyRow from '../lib/components/KeyRow.svelte'
  import SettingRow from '../lib/components/SettingRow.svelte'
  import Skeleton from '../lib/components/Skeleton.svelte'
  import ThemeButton from '../lib/components/ThemeButton.svelte'
  import { railDot, updates } from '../lib/updates.svelte'

  const updateDot = $derived(railDot(updates.shown))


  const settings = new Load<Setting[]>()
  settings.run(api.settings)

  let q = $state('')

  $effect(() => {
    document.title = 'Settings · Speaker Trail'
  })

  let matches = $derived(
    (settings.data ?? []).filter((s) => !q || `${s.key} ${s.description}`.toLowerCase().includes(q.toLowerCase())),
  )

  function saved(s: Setting) {
    if (settings.data) settings.data = settings.data.map((x) => (x.key === s.key ? s : x))
    toast.show(`Saved ${s.key}`)
  }
</script>

<div class="page">
  <header class="bar">
    <h1 title="Each row saves on its own.">Settings</h1>
  </header>
  <div class="page-body">

  <SearchInput bind:value={q} placeholder="Search settings" label="Search settings" />

  <KeyRow />

  {#if settings.error && !settings.data}
    <EmptyState icon="alert" tone="bad" title="Could not load the settings" text={settings.error}>
      <button class="btn" type="button" onclick={() => settings.run(api.settings)}><Icon name="refresh" />Try again</button>
    </EmptyState>
  {:else if !settings.data}
    <Skeleton count={6} />
  {:else if !matches.length}
    <EmptyState icon="settings" title="No setting matches" text="Try another word." />
  {:else}
    <section class="group">
      <h2>Searches</h2>
      <div class="list">
        {#each matches as s (s.key)}
          <SettingRow setting={s} onsaved={saved} />
        {/each}
      </div>
    </section>
  {/if}

  <!-- The sidebar holds Updates and the theme on wide screens. -->
  <div class="account">
    <a class="btn quiet" href="/app/updates"><Icon name="update" />Updates{#if updateDot}<span class="dot {updateDot}"></span>{/if}</a>
    <ThemeButton withLabel />
  </div>
</div>
</div>

<style>
  .group { display: grid; gap: 10px; }
  .group h2 { font-size: 15px; margin: 0; }
  /* The sidebar holds these on wide screens. */
  .account { display: none; flex-wrap: wrap; gap: 8px; }
  @media (max-width: 760px) { .account { display: flex; } }
</style>
