<script lang="ts">
  import { api, type Setting } from '../lib/api'
  import { Load } from '../lib/load.svelte'
  import { toast } from '../lib/toast.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import SearchInput from '../lib/components/SearchInput.svelte'
  import SettingRow from '../lib/components/SettingRow.svelte'
  import Skeleton from '../lib/components/Skeleton.svelte'

  const settings = new Load<Setting[]>()
  settings.run(api.settings)

  let q = $state('')

  $effect(() => {
    document.title = 'Settings · Speaker Trail'
  })

  let shown = $derived(
    (settings.data ?? []).filter((s) => !q || `${s.key} ${s.description}`.toLowerCase().includes(q.toLowerCase())),
  )

  function saved(s: Setting) {
    if (settings.data) settings.data = settings.data.map((x) => (x.key === s.key ? s : x))
    toast.show(`Saved ${s.key}`)
  }
</script>

<div class="view">
  <div class="view-head">
    <div>
      <h1>Settings</h1>
      <p>How the crawler and the calendar behave. Each row saves on its own.</p>
    </div>
  </div>

  <SearchInput bind:value={q} placeholder="Search settings" label="Search settings" />

  {#if settings.error && !settings.data}
    <EmptyState icon="alert" tone="bad" title="Could not load the settings" text={settings.error}>
      <button class="btn" type="button" onclick={() => settings.run(api.settings)}><Icon name="refresh" />Try again</button>
    </EmptyState>
  {:else if !settings.data}
    <Skeleton count={8} />
  {:else if !shown.length}
    <EmptyState icon="settings" title="No setting matches" text="Try another word." />
  {:else}
    <div class="list">
      {#each shown as s (s.key)}
        <SettingRow setting={s} onsaved={saved} />
      {/each}
    </div>
  {/if}
</div>
