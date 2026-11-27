<script lang="ts">
  import { tick, untrack } from 'svelte'
  import { api, ApiError, type Source } from '../lib/api'
  import { fmtAgo, fmtDay, plural } from '../lib/format'
  import { bringIntoView, undoHistory } from '../lib/history.svelte'
  import { Load } from '../lib/load.svelte'
  import { Problem } from '../lib/problem.svelte'
  import { toast } from '../lib/toast.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Exclusions from '../lib/components/Exclusions.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import ProblemNote from '../lib/components/ProblemNote.svelte'
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
  // What Undo and Redo in the Edit menu took back or did again shows at
  // once. Only a change after Searches opened counts.
  const undoneAt = undoHistory.changed
  $effect(() => {
    if (undoHistory.changed === undoneAt) return
    untrack(() => searches.run(() => api.sources({ kind: 'profile_search' }), { quiet: true }))
  })
  // A search Undo or Redo changed comes into view once the list holds it.
  $effect(() => {
    const ids = (searches.data ?? []).map((s) => s.id)
    const t = undoHistory.reveal((t) => t.kind === 'search' && ids.includes(t.id))
    if (t?.kind !== 'search') return
    void tick().then(() => {
      const row = document.querySelector<HTMLElement>(`li.search[data-id="${t.id}"]`)
      if (row) bringIntoView(row)
    })
  })

  // A person's "Found by" links here with the search's words, so the
  // list opens on that search.
  let q = $state(new URLSearchParams(location.search).get('q') ?? '')
  let all = $derived(searches.data ?? [])
  // The searches that led to the most connections on top, then those that
  // found the most people. Paused ones last. Runs take them in much the
  // same order, see RankSearches in internal/store.
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
  // What went wrong adding says so under the field, and pausing or
  // resuming a search on its row, see lib/problem.svelte.ts.
  const addProblem = new Problem()
  let addBusy = $state(false)
  let field: HTMLInputElement | undefined = $state()

  async function add(e: SubmitEvent) {
    e.preventDefault()
    const input = newQuery.trim()
    if (!input) {
      addProblem.needs('Enter a kind of person and a place, like Yogalehrerin Köln')
      field?.focus()
      return
    }
    addBusy = true
    addProblem.clear()
    try {
      const s = await api.addProfileSearch(input)
      searches.data = [s, ...all]
      newQuery = ''
      toast.show(`Added ${s.query}. The next run takes it first`)
    } catch (err) {
      // One that is there already is not something that went wrong.
      if (err instanceof ApiError && err.status === 409) addProblem.needs(err.message)
      else addProblem.say(err)
    } finally {
      addBusy = false
    }
  }

  // Pausing is undone with the same button, or Undo in the Edit menu.
  let busy = $state<number | null>(null)
  const rowProblem = new Problem()
  let failedOn = $state<number | null>(null)
  async function toggle(s: Source) {
    const status = s.status === 'retired' ? 'candidate' : 'retired'
    busy = s.id
    rowProblem.clear()
    failedOn = s.id
    try {
      const n = await api.patchSource(s.id, { status })
      searches.data = all.map((x) => (x.id === n.id ? n : x))
    } catch (err) {
      rowProblem.failed(err)
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

<div class="page">
  <div class="page-body">

  <form class="add" onsubmit={add} novalidate>
    <label class="sr-only" for="new-search">New search</label>
    <input bind:this={field} bind:value={newQuery} id="new-search" class="input" type="text" placeholder="Yogalehrerin Köln" autocomplete="off"
      maxlength="200" aria-invalid={addProblem.text ? 'true' : undefined} aria-describedby={addProblem.text ? 'search-err' : undefined}
      title="A kind of person and a place. Exa finds their LinkedIn profiles, and only people in NRW who were active lately and run their own business are kept" />
    <button class="btn primary" type="submit" disabled={addBusy}><Icon name="plus" />{addBusy ? 'Adding' : 'Add'}</button>
  </form>
  <ProblemNote of={addProblem} id="search-err" />

  <Exclusions />

  {#if searches.error && !searches.data}
    <EmptyState icon="alert" tone="bad" title="Could not load the searches" text={searches.error}>
      <button class="btn" type="button" onclick={() => reload++}><Icon name="refresh" />Try again</button>
    </EmptyState>
  {:else if !searches.data}
    <Skeleton count={7} />
  {:else if !all.length}
    <EmptyState icon="search" title="No searches yet" text="Add one above, like Personal Trainer Düsseldorf." />
  {:else}
    <div class="group">
    {#if all.length > 8 || q}
      <SearchInput bind:value={q} placeholder="Find a search" label="Find a search" />
    {/if}
    <p class="summary">{plural(all.length, 'search', 'searches')}{connecting ? `, ${connecting} led to connections` : ''}{paused ? `, ${paused} paused` : ''}</p>
    {#if !shown.length}
      <EmptyState icon="search" title="No search matches" text="Try another word." />
    {:else}
      <ul class="list">
        {#each shown as s (s.id)}
          <li class="search" class:paused={s.status === 'retired'} data-id={s.id}>
            <span class="what">
              <b class="ellipsis">{s.query}</b>
              <span class="ellipsis sub">{#if s.people_connected}<span class="connected">{s.people_connected} connected</span>{' · '}{/if}{line(s)}</span>
              {#if s.health === 'error' || s.health === 'warning'}
                <span class="ellipsis note" class:bad={s.health === 'error'} title={s.health_note}>{s.health_note}</span>
              {/if}
            </span>
            <button class="btn sm quiet toggle" type="button" onclick={() => toggle(s)} disabled={busy === s.id}
              title={s.status === 'retired' ? 'Runs take this search again' : 'Runs leave this search out until you resume it'}>
              {s.status === 'retired' ? 'Resume' : 'Pause'}
            </button>
            {#if failedOn === s.id && rowProblem.text}<div class="failed"><ProblemNote of={rowProblem} /></div>{/if}
          </li>
        {/each}
      </ul>
    {/if}
    </div>
  {/if}
</div>
</div>

<style>
  .add { display: flex; gap: var(--gap-control); }
  .add .input { flex: 1; min-width: 0; }
  .add .btn { flex: none; }
  .summary { font-size: 13px; color: var(--ink-2); }
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
  .failed { grid-column: 1 / -1; min-width: 0; }
  /* Room for the longer word, so the button keeps its width. */
  @media (max-width: 520px) { .search { padding: 12px 14px; } }
</style>
