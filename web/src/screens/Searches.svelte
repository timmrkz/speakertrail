<script lang="ts">
  import { api, type Source } from '../lib/api'
  import { fmtAgo, fmtDay, plural } from '../lib/format'
  import { Load } from '../lib/load.svelte'
  import { errorText, toast } from '../lib/toast.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Exclusions from '../lib/components/Exclusions.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import SearchInput from '../lib/components/SearchInput.svelte'
  import Skeleton from '../lib/components/Skeleton.svelte'

  // Profile searches: a kind of person and a place, like "Yogalehrerin
  // Köln". Each is one call to Exa, and a run takes the ten that are due.
  const searches = new Load<Source[]>()
  let reload = $state(0)
  $effect(() => {
    void reload
    searches.run(() => api.sources({ kind: 'profile_search' }))
  })
  $effect(() => {
    document.title = 'Searches · Speaker Trail'
  })

  let q = $state('')
  let all = $derived(searches.data ?? [])
  // The searches that led to the most connections on top, then those that
  // found the most people. Paused ones last. Runs take them in much the
  // same order, see RankSearches in internal/pipeline.
  let ranked = $derived(
    [...all].sort(
      (a, b) =>
        Number(a.status === 'retired') - Number(b.status === 'retired') ||
        b.people_connected - a.people_connected ||
        b.people - a.people ||
        (a.query ?? '').localeCompare(b.query ?? '', 'de'),
    ),
  )
  let shown = $derived(q ? ranked.filter((s) => (s.query ?? '').toLowerCase().includes(q.toLowerCase())) : ranked)
  let connecting = $derived(all.filter((s) => s.people_connected > 0).length)
  let paused = $derived(all.filter((s) => s.status === 'retired').length)

  let newQuery = $state('')
  let addError = $state('')
  let addBusy = $state(false)
  let field: HTMLInputElement | undefined = $state()

  async function add(e: SubmitEvent) {
    e.preventDefault()
    const input = newQuery.trim()
    if (!input) {
      addError = 'Enter a kind of person and a place, like Yogalehrerin Köln'
      field?.focus()
      return
    }
    addBusy = true
    addError = ''
    try {
      const s = await api.addProfileSearch(input)
      searches.data = [s, ...all]
      newQuery = ''
      toast.show(`Added ${s.query}. The next run takes it first`)
    } catch (err) {
      addError = errorText(err)
    } finally {
      addBusy = false
    }
  }

  // Pausing is undone with the same button.
  let busy = $state<number | null>(null)
  async function toggle(s: Source) {
    const status = s.status === 'retired' ? 'candidate' : 'retired'
    busy = s.id
    try {
      const n = await api.patchSource(s.id, { status })
      searches.data = all.map((x) => (x.id === n.id ? n : x))
    } catch (err) {
      toast.show(errorText(err))
    } finally {
      busy = null
    }
  }

  function line(s: Source): string {
    const parts: string[] = []
    if (s.last_checked_at) {
      parts.push(plural(s.people, 'person', 'people'))
      parts.push(`ran ${fmtAgo(s.last_checked_at)}`)
    } else {
      parts.push('Not run yet')
    }
    if (s.status !== 'retired' && s.next_check_at && new Date(s.next_check_at) > new Date()) parts.push(`next ${fmtDay(s.next_check_at)}`)
    return parts.join(' · ')
  }
</script>

<div class="view">
  <div class="view-head">
    <div>
      <h1>Searches</h1>
      <p>A kind of person and a place. Those that led to connections run first.</p>
    </div>
  </div>

  <form class="add" onsubmit={add} novalidate>
    <label class="sr-only" for="new-search">New search</label>
    <input bind:this={field} bind:value={newQuery} id="new-search" class="input" type="text" placeholder="Yogalehrerin Köln" autocomplete="off"
      maxlength="200" aria-invalid={addError ? 'true' : undefined} aria-describedby={addError ? 'search-err' : undefined}
      title="A kind of person and a place. Exa finds their LinkedIn profiles, and only people in NRW who were active lately and run their own business are kept" />
    <button class="btn primary" type="submit" disabled={addBusy}><Icon name="plus" /><span>{addBusy ? 'Adding' : 'Add'}</span></button>
  </form>
  {#if addError}<p id="search-err" class="field-error" role="alert">{addError}</p>{/if}

  <Exclusions />

  {#if searches.error && !searches.data}
    <EmptyState icon="alert" tone="bad" title="Could not load the searches" text={searches.error}>
      <button class="btn" type="button" onclick={() => reload++}><Icon name="refresh" />Try again</button>
    </EmptyState>
  {:else if !searches.data}
    <Skeleton count={7} />
  {:else if !all.length}
    <EmptyState icon="sources" title="No searches yet" text="Add one above, like Personal Trainer Düsseldorf." />
  {:else}
    {#if all.length > 8}
      <SearchInput bind:value={q} placeholder="Find a search" label="Find a search" />
    {/if}
    <p class="summary">{plural(all.length, 'search', 'searches')}{connecting ? `, ${connecting} led to connections` : ''}{paused ? `, ${paused} paused` : ''}</p>
    {#if !shown.length}
      <EmptyState icon="sources" title="No search matches" text="Try another word." />
    {:else}
      <ul class="list">
        {#each shown as s (s.id)}
          <li class="search" class:paused={s.status === 'retired'}>
            <span class="what">
              <b class="ellipsis">{s.query}</b>
              <span class="ellipsis sub">{#if s.people_connected}<span class="connected">{s.people_connected} connected</span>{' · '}{/if}{line(s)}</span>
              {#if s.health === 'error' || s.health === 'warning'}
                <span class="ellipsis note" class:bad={s.health === 'error'} title={s.health_note}>{s.health_note}</span>
              {/if}
            </span>
            <button class="btn sm toggle" type="button" onclick={() => toggle(s)} disabled={busy === s.id}
              title={s.status === 'retired' ? 'Runs take this search again' : 'Runs leave this search out until you resume it'}>
              {s.status === 'retired' ? 'Resume' : 'Pause'}
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</div>

<style>
  .add { display: flex; gap: 8px; }
  .add .input { flex: 1; min-width: 0; }
  .add .btn { flex: none; min-width: 6.4em; justify-content: center; }
  .summary { font-size: 13px; color: var(--ink-2); margin-bottom: -10px; }
  .search {
    display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 12px; align-items: center;
    padding: 12px 16px;
  }
  .what { display: grid; gap: 1px; min-width: 0; }
  .what b { font-weight: 600; }
  .sub { font-size: 13px; color: var(--ink-2); font-variant-numeric: tabular-nums; }
  .connected { color: var(--good); font-weight: 600; }
  .note { font-size: 12px; color: var(--warn); }
  .note.bad { color: var(--bad); }
  .paused .what { opacity: .5; }
  /* Room for the longer word, so the button keeps its width. */
  .toggle { min-width: 6.2em; justify-content: center; }
  @media (max-width: 520px) { .search { padding: 12px 14px; } }
</style>
