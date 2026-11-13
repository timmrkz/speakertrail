<script lang="ts">
  // One person, with room to read: who they are, Connect and LinkedIn,
  // why they fit, which search found them, and Tim's notes. The pane
  // beside the list in a wide window and the sheet in a narrow one both
  // show this, so the two never drift apart.
  import { api, ApiError, type PersonDetail } from '../lib/api'
  import { fmtDay, initials } from '../lib/format'
  import { Load } from '../lib/load.svelte'
  import { errorText, toast } from '../lib/toast.svelte'
  import Busy from '../lib/components/Busy.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Icon from '../lib/components/Icon.svelte'

  let { id, onupdate }: { id: number; onupdate?: (p: PersonDetail) => void } = $props()

  const person = new Load<PersonDetail>()
  let notes = $state('')
  let savedNotes = $state('')
  let noteState = $state<'idle' | 'saving' | 'saved' | 'error'>('idle')

  // The parent keys this by id, so the id never changes here.
  function load() {
    person.run(() => api.person(id)).then(() => {
      notes = savedNotes = person.data?.notes ?? ''
    })
  }
  load()

  const year = new Date().getFullYear()
  let facts = $derived.by(() => {
    const p = person.data
    if (!p) return ''
    const l = p.linkedin
    return [l?.born_about ? `About ${year - l.born_about}` : '', p.city, l?.last_active ? `active ${fmtDay(l.last_active)}` : '']
      .filter(Boolean)
      .join(' · ')
  })
  // The LinkedIn profile the search found, unless Tim said it is not them.
  let linkedin = $derived(person.data?.profiles.find((x) => x.platform === 'linkedin' && x.review !== 'rejected')?.url ?? '')
  let reasons = $derived((person.data?.signals ?? []).filter((s) => s.for))
  let doubts = $derived((person.data?.signals ?? []).filter((s) => !s.for))

  async function saveNotes() {
    if (!person.data || notes === savedNotes) return
    noteState = 'saving'
    try {
      const updated = await api.patchPerson(id, { notes })
      savedNotes = updated.notes
      person.data = { ...person.data, notes: updated.notes }
      noteState = 'saved'
    } catch (e) {
      noteState = 'error'
      toast.show(errorText(e), 'bad')
    }
  }

  // LinkedIn opens in the browser profile chosen in Settings, the one
  // logged in to LinkedIn, see internal/browser. The click shows at once.
  // Without a choice the app does not guess, and the line beside the
  // button leads to Settings.
  let opening = $state(false)
  let openProblem = $state('')
  let needsChoice = $state(false)
  async function openLinkedin(e: MouseEvent) {
    e.preventDefault()
    if (opening) return
    opening = true
    openProblem = ''
    needsChoice = false
    try {
      await api.open(linkedin)
    } catch (err) {
      needsChoice = err instanceof ApiError && err.status === 409 && /Settings/.test(err.message)
      openProblem = errorText(err)
    } finally {
      // Long enough to see, short enough not to wait for.
      setTimeout(() => (opening = false), 600)
    }
  }

  // Connected on LinkedIn, or not. The same button again undoes it. It
  // shows at once and goes back if the app says no.
  let deciding = $state(false)
  async function toggleConnected() {
    if (!person.data || deciding) return
    const before = person.data.connected
    deciding = true
    person.data = { ...person.data, connected: !before }
    onupdate?.(person.data)
    try {
      await api.patchPerson(id, { connected: !before })
    } catch (e) {
      person.data = { ...person.data, connected: before }
      onupdate?.(person.data)
      toast.show(errorText(e), 'bad')
    } finally {
      deciding = false
    }
  }
</script>

{#if person.error && !person.data}
  <EmptyState icon="alert" tone="bad" title="Could not load this person" text={person.error}>
    <button class="btn" type="button" onclick={load}>Try again</button>
  </EmptyState>
{:else if person.data}
  {@const p = person.data}
  <article class="person" aria-labelledby="person-{p.id}">
    <header class="who">
      <span class="avatar xl" class:connected={p.connected} aria-hidden="true">{initials(p.name)}</span>
      <div class="names">
        <h2 id="person-{p.id}">{p.name}</h2>
        {#if p.headline}<p class="headline">{p.headline}</p>{/if}
        {#if facts}<p class="facts" title={p.linkedin?.age_reason ? `The age is a guess from the LinkedIn profile: ${p.linkedin.age_reason}` : undefined}>{facts}</p>{/if}
      </div>
      <div class="actions">
        <button class="btn wide connect" class:primary={!p.connected} class:done={p.connected} type="button"
          aria-pressed={p.connected} disabled={deciding} onclick={toggleConnected}
          title={p.connected ? 'Connected on LinkedIn. Click again to take it back' : 'Mark as connected once you connected on LinkedIn. It counts for the search that found them'}>
          <Icon name={p.connected ? 'check' : 'connect'} />{p.connected ? 'Connected' : 'Connect'}
        </button>
        {#if linkedin}
          <a class="btn wide" href={linkedin} target="_blank" rel="noopener noreferrer" aria-busy={opening} onclick={openLinkedin}
            title="Open the LinkedIn profile in the browser profile chosen in Settings">
            {#if opening}<Busy />{/if}<Icon name="external" />{opening ? 'Opening' : 'LinkedIn'}
          </a>
        {/if}
      </div>
      {#if openProblem}
        <p class="open-problem small" role="alert">
          {#if needsChoice}Choose where LinkedIn opens in <a href="/app/settings#linkedin">Settings</a>.{:else}{openProblem}{/if}
        </p>
      {/if}
    </header>

    {#if reasons.length || doubts.length}
      <section class="section" aria-labelledby="why-{p.id}">
        <h3 id="why-{p.id}" class="label">Why</h3>
        <ul class="why">
          {#each [...reasons, ...doubts] as s (s.key)}
            <li class:against={!s.for}>
              <span class="mark" aria-hidden="true"><Icon name={s.for ? 'check' : 'x'} size={14} /></span>
              <span class="what"><b>{s.label}</b><span class="sr-only">{s.for ? ', speaks for' : ', speaks against'}</span>
                {#if s.passage}<q title={s.found_in}>{s.passage}</q>{/if}</span>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    {#if p.sightings.length}
      <p class="found">Found by {#each p.sightings as s, i (s.source_id)}{#if i}, {/if}<a href="/app/searches?q={encodeURIComponent(s.source)}">{s.source}</a>{/each}</p>
    {/if}

    <section class="section">
      <div class="row between">
        <label class="label" for="notes-{p.id}">Notes</label>
        <span class="small note-state" aria-live="polite">
          {noteState === 'saving' ? 'Saving' : noteState === 'saved' ? 'Saved' : noteState === 'error' ? 'Not saved' : ''}
        </span>
      </div>
      <textarea id="notes-{p.id}" class="textarea" rows="4" bind:value={notes} oninput={() => (noteState = 'idle')} onblur={saveNotes}
        placeholder="Where you met, what to ask"></textarea>
    </section>
  </article>
{:else}
  <div class="person" aria-busy="true">
    <div class="who">
      <span class="avatar xl skel" aria-hidden="true"></span>
      <div class="names"><span class="skel" style:width="55%" style:height="24px"></span><span class="skel" style:width="80%" style:height="14px"></span></div>
    </div>
  </div>
{/if}

<style>
  .person { display: grid; gap: 28px; min-width: 0; }
  .who { display: grid; gap: 16px; min-width: 0; }
  .names { display: grid; gap: 6px; min-width: 0; }
  .names h2 { font-size: 26px; font-weight: 650; letter-spacing: -.01em; overflow-wrap: anywhere; }
  .headline { font-size: 16px; line-height: 1.45; color: var(--ink-2); overflow-wrap: anywhere; }
  .facts { font-size: 14px; color: var(--ink-3); font-variant-numeric: tabular-nums; }
  .avatar.xl { width: 64px; height: 64px; font-size: 20px; font-weight: 650; }
  .avatar.connected { background: var(--good-soft); color: var(--good); }
  .actions { display: flex; flex-wrap: wrap; gap: 10px; }
  /* Both buttons are as wide as the longer word, Connected, so neither
     moves when Connect turns into it. */
  .btn.wide { min-width: 9.5em; }
  .btn.done { background: var(--good-soft); border-color: transparent; color: var(--good); }
  .why { display: grid; gap: 10px; }
  .why li { display: grid; grid-template-columns: 20px minmax(0, 1fr); gap: 8px; align-items: start; font-size: 14px; }
  .why .mark {
    width: 20px; height: 20px; border-radius: 50%; display: grid; place-items: center; margin-top: 1px;
    background: var(--good-soft); color: var(--good);
  }
  .why .against .mark { background: var(--bad-soft); color: var(--bad); }
  .why .what { display: grid; gap: 2px; min-width: 0; }
  .why b { font-weight: 600; }
  .why q { color: var(--ink-2); overflow-wrap: anywhere; }
  .found { font-size: 13px; color: var(--ink-3); }
  .open-problem { color: var(--bad); margin-top: -6px; }
  .note-state { color: var(--ink-3); }
</style>
