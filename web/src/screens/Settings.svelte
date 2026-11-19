<script lang="ts">
  import { api, type Setting } from '../lib/api'
  import { Load } from '../lib/load.svelte'
  import { toast } from '../lib/toast.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import SearchInput from '../lib/components/SearchInput.svelte'
  import KeyRow from '../lib/components/KeyRow.svelte'
  import LinkedinRow from '../lib/components/LinkedinRow.svelte'
  import LogsRow from '../lib/components/LogsRow.svelte'
  import SettingRow from '../lib/components/SettingRow.svelte'
  import SpeechRow from '../lib/components/SpeechRow.svelte'
  import LanguageRow from '../lib/components/LanguageRow.svelte'
  import Skeleton from '../lib/components/Skeleton.svelte'
  import ThemeRow from '../lib/components/ThemeRow.svelte'

  const settings = new Load<Setting[]>()
  settings.run(api.settings)

  let q = $state('')

  $effect(() => {
    document.title = 'Settings · Speaker Trail'
  })

  let matches = $derived(
    (settings.data ?? []).filter((s) => !q || `${s.key} ${s.description}`.toLowerCase().includes(q.toLowerCase())),
  )
  // The prompt a first message is written from sits with the language
  // model that writes it, the rest are about searches.
  const drafting = new Set(['first_message_prompt'])
  let forDrafts = $derived(matches.filter((s) => drafting.has(s.key)))
  let forSearches = $derived(matches.filter((s) => !drafting.has(s.key)))

  function saved(s: Setting) {
    if (settings.data) settings.data = settings.data.map((x) => (x.key === s.key ? s : x))
    toast.show(`Saved ${s.key}`)
  }
</script>

<div class="page">
  <div class="page-body">

  <SearchInput bind:value={q} placeholder="Search settings" label="Search settings" />

  <KeyRow />

  <LinkedinRow />

  <ThemeRow />

  <SpeechRow />

  <LanguageRow />

  <LogsRow />

  {#if settings.error && !settings.data}
    <EmptyState icon="alert" tone="bad" title="Could not load the settings" text={settings.error}>
      <button class="btn" type="button" onclick={() => settings.run(api.settings)}><Icon name="refresh" />Try again</button>
    </EmptyState>
  {:else if !settings.data}
    <Skeleton count={6} />
  {:else if !matches.length}
    <EmptyState icon="settings" title="No setting matches" text="Try another word." />
  {:else}
    {#if forDrafts.length}
      <section class="group">
        <h2>First message</h2>
        <div class="list">
          {#each forDrafts as s (s.key)}
            <SettingRow setting={s} onsaved={saved} />
          {/each}
        </div>
      </section>
    {/if}
    {#if forSearches.length}
      <section class="group">
        <h2>Searches</h2>
        <div class="list">
          {#each forSearches as s (s.key)}
            <SettingRow setting={s} onsaved={saved} />
          {/each}
        </div>
      </section>
    {/if}
  {/if}

</div>
</div>

<style>
  .group h2 { font-size: 15px; margin: 0; }
</style>
