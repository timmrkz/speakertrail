<script lang="ts">
  import { untrack } from 'svelte'
  import { api, type Setting } from '../lib/api'
  import { undoHistory } from '../lib/history.svelte'
  import { Load } from '../lib/load.svelte'
  import { toast } from '../lib/toast.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import Info from '../lib/components/Info.svelte'
  import SearchInput from '../lib/components/SearchInput.svelte'
  import KeyRow from '../lib/components/KeyRow.svelte'
  import LinkedinRow from '../lib/components/LinkedinRow.svelte'
  import LogsRow from '../lib/components/LogsRow.svelte'
  import SettingRow from '../lib/components/SettingRow.svelte'
  import SpeechRow from '../lib/components/SpeechRow.svelte'
  import LanguageRow from '../lib/components/LanguageRow.svelte'
  import Skeleton from '../lib/components/Skeleton.svelte'
  import Template from '../lib/components/Template.svelte'
  import ThemeRow from '../lib/components/ThemeRow.svelte'

  const settings = new Load<Setting[]>()
  settings.run(api.settings)
  // What Undo and Redo in the Edit menu took back or did again shows at
  // once. Only a change after Settings opened counts.
  const undoneAt = undoHistory.changed
  $effect(() => {
    if (undoHistory.changed === undoneAt) return
    untrack(() => settings.run(api.settings, { quiet: true }))
  })

  let q = $state('')

  $effect(() => {
    document.title = 'Settings · Speaker Trail'
  })

  let matches = $derived(
    (settings.data ?? []).filter((s) => !q || `${s.key} ${s.description}`.toLowerCase().includes(q.toLowerCase())),
  )
  // The prompt a first message is written from sits with the language
  // model that writes it, the rest are about searches.
  const drafting = new Set(['hook_prompt'])
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

  <!-- The model that drafts first, the one Tim uses most, then the one
       Dictate hears with, as Tim asked on 20 November 2026. -->
  <LanguageRow />

  <SpeechRow />

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
      <!-- The prompt is a box on its own under the heading, with no card
           round it, because a box in a frame read as one thing wrapped in
           another, Tim, 20 November 2026. What it is for waits behind the
           info mark. -->
      <section class="group prompt">
        {#each forDrafts as s (s.key)}
          <div class="headrow">
            <h2 id="head-{s.key}">First message prompt</h2>
            <span class="ask"><Info label="About the prompt" side="left">{s.description}</Info></span>
          </div>
          <SettingRow setting={s} onsaved={saved} bare labelledby="head-{s.key}" />
        {/each}
      </section>
    {/if}
    {#if !q || 'template first message variation part'.includes(q.toLowerCase())}
      <!-- The rest of a first message, picked at random from Tim's
           variations, beside the prompt for the model's part. -->
      <Template />
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
  /* The heading with its info mark at the far end, as on the model rows. */
  .headrow { display: flex; align-items: center; justify-content: space-between; gap: 6px; }
  .headrow .ask { align-self: flex-start; margin: 1px 1px 0 0; }
</style>
