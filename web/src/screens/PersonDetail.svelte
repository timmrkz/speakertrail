<script lang="ts">
  // One person, with room to read: Connect, Draft and LinkedIn in one
  // row, the first message to them, why they fit, which search found
  // them, and Tim's notes. The pane beside the list in a wide window and
  // the sheet in a narrow one both show this, so the two never drift
  // apart. The name stands in the row of the cross above it, which the
  // pane and the sheet draw, so onname tells them the name.
  import { untrack, type Snippet } from 'svelte'
  import { api, ApiError, type PersonDetail, type Profile, type Review } from '../lib/api'
  import { Load } from '../lib/load.svelte'
  import { pasting } from '../lib/paste.svelte'
  import { errorText, fault, toast } from '../lib/toast.svelte'
  import Busy from '../lib/components/Busy.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import FirstMessage from '../lib/components/FirstMessage.svelte'
  import Icon from '../lib/components/Icon.svelte'

  // onmoved follows once the app took a change that moves the person to
  // another list, connected or the profile rejected, or turned it down,
  // so a list can read again what it shows.
  let { id, onupdate, onmoved, onname }: {
    id: number
    onupdate?: (p: PersonDetail) => void
    onmoved?: () => void
    onname?: (id: number, name: string) => void
  } = $props()

  const person = new Load<PersonDetail>()
  let notes = $state('')
  let savedNotes = $state('')
  let noteState = $state<'idle' | 'saving' | 'saved' | 'error'>('idle')

  // The parent keys this by id, so the id never changes here.
  function load() {
    person.run(() => api.person(id)).then(() => {
      notes = savedNotes = person.data?.notes ?? ''
      // The person's own id goes with the name, so an answer that comes
      // after another person opened is told apart.
      if (person.data) onname?.(person.data.id, person.data.name)
    })
  }
  load()

  // A LinkedIn page pasted as their profile, or Undo, shows at once,
  // with the name, headline and city the page gave.
  // Only a change after it opened counts.
  const pastedAt = pasting.changed
  $effect(() => {
    if (pasting.changed === pastedAt) return
    untrack(() => person.run(() => api.person(id), { quiet: true }).then(() => {
      if (person.data) {
        onname?.(person.data.id, person.data.name)
        onupdate?.(person.data)
      }
    }))
  })

  // The LinkedIn profile the search found, and the link the app opens,
  // which it leaves out once Tim said the profile is not them.
  let profile = $derived(person.data?.profiles.find((x) => x.platform === 'linkedin'))
  let linkedin = $derived(person.data?.linkedin?.url ?? '')

  // A search can find the wrong person's profile. Not this person rejects
  // it, so LinkedIn no longer opens it and the person moves to Rejected.
  // Like Connect, the button stays and the same button again takes it
  // back (Tim, testing #67 on 22 November 2026). It shows at once and
  // goes back if the app says no. Undo in the toast sets back what was
  // there before.
  let reviewing = $state(false)
  function showReview(of: Profile, next: Review) {
    if (!person.data) return
    person.data = { ...person.data, profiles: person.data.profiles.map((x) => (x.id === of.id ? { ...x, review: next } : x)) }
  }
  async function review(of: Profile, next: Review) {
    await api.patchProfile(of.id, next)
    // The app says what follows from it, the link and the facts.
    person.data = await api.person(id)
    onupdate?.(person.data)
    onmoved?.()
  }
  async function toggleRejected() {
    if (!profile || reviewing) return
    const of = profile
    const next: Review = of.review === 'rejected' ? 'confirmed' : 'rejected'
    reviewing = true
    showReview(of, next)
    try {
      await review(of, next)
      if (next === 'rejected') {
        toast.show('LinkedIn profile rejected as not this person', 'plain', {
          label: 'Undo',
          run: () => review(of, of.review).catch((e) => fault(e)),
        })
      }
    } catch (e) {
      showReview(of, of.review)
      fault(e)
    } finally {
      reviewing = false
    }
  }

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
      fault(e)
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
      await api.openLinkedin(id)
    } catch (err) {
      needsChoice = err instanceof ApiError && err.choose
      openProblem = errorText(err)
    } finally {
      opening = false
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
      fault(e)
    } finally {
      deciding = false
      onmoved?.()
    }
  }
</script>

{#if person.error && !person.data}
  <EmptyState icon="alert" tone="bad" title="Could not load this person" text={person.error}>
    <button class="btn" type="button" onclick={load}>Try again</button>
  </EmptyState>
{:else if person.data}
  {@const p = person.data}
  <article class="person" data-personal aria-label={p.name}>
    <!-- One row of what Tim does with a person, in the order he does it:
         connect, draft the first message, open LinkedIn to send it. Draft
         belongs to the first message, which hands it here. -->
    {#snippet row(draft?: Snippet)}
      <div class="top">
        <div class="actions">
          <button class="btn connect good" class:primary={!p.connected} type="button"
            aria-pressed={p.connected} disabled={deciding} onclick={toggleConnected}
            title={p.connected ? 'Connected on LinkedIn. Click again to take it back' : 'Mark as connected once you connected on LinkedIn. It counts for the search that found them'}>
            <Icon name={p.connected ? 'check' : 'connect'} />{p.connected ? 'Connected' : 'Connect'}
          </button>
          {#if draft}{@render draft()}{/if}
          {#if linkedin}
            <a class="btn" href={linkedin} target="_blank" rel="noopener noreferrer" aria-busy={opening} onclick={openLinkedin}
              title="Open the LinkedIn profile in the browser profile chosen in Settings">
              {#if opening}<Busy />{/if}<Icon name="external" />{opening ? 'Opening' : 'LinkedIn'}
            </a>
          {/if}
          {#if profile}
            {@const rejected = profile.review === 'rejected'}
            <button class="btn icon bad" type="button" aria-pressed={rejected} disabled={reviewing} onclick={toggleRejected}
              title={rejected ? 'Rejected as not this person. Click again to take it back' : "Not this person. The LinkedIn profile the search found is someone else's"}
              aria-label="Not this person">
              <Icon name="not-them" />
            </button>
          {/if}
        </div>
        {#if profile?.review === 'rejected'}
          <p class="rejected small">LinkedIn profile rejected as not this person.</p>
        {/if}
        {#if openProblem}
          <p class="open-problem small" role="alert">
            {#if needsChoice}Choose where LinkedIn opens in <a href="/app/settings#linkedin">Settings</a>.{:else}{openProblem}{/if}
          </p>
        {/if}
      </div>
    {/snippet}

    {#if linkedin}
      <FirstMessage person={p} {row} onsaved={(u) => { if (person.data) person.data = { ...person.data, draft: u.draft, drafted_at: u.drafted_at, profile_read_at: u.profile_read_at } }} />
    {:else}
      {@render row()}
    {/if}

    {#if p.fit_parts?.length}
      <!-- One list of what the fit adds up: each part with its points,
           and under it a tick or a cross for what it read, the signals
           with their passages under Who they are, docs/fit-score.md. -->
      <section class="section" aria-labelledby="fit-{p.id}">
        <h3 id="fit-{p.id}" class="label">Fit <span class="num">{p.fit_score}</span><span class="sr-only"> of 100</span></h3>
        <ul class="fit">
          {#each p.fit_parts as f (f.key)}
            <li class="part">
              <span class="pts num" aria-hidden="true">{f.points < 0 ? '−' : '+'}{Math.abs(f.points)}</span>
              <b class="part-label">{f.label}<span class="sr-only">, {f.points} points</span></b>
              <ul class="why">
                {#each f.lines as l, i (i)}
                  <li class:against={l.mark === 'against'} class:neither={l.mark === 'neither'}>
                    <span class="mark" aria-hidden="true"><Icon name={l.mark === 'for' ? 'check' : l.mark === 'against' ? 'x' : 'neither'} size={14} /></span>
                    <span class="what"><span class="text">{l.text}</span><span class="sr-only">{l.mark === 'for' ? ', speaks for' : l.mark === 'against' ? ', speaks against' : ''}</span>
                      {#if l.passage}<q title={l.found_in}>{l.passage}</q>{/if}</span>
                  </li>
                {/each}
              </ul>
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
    <div class="actions"><span class="skel" style:width="110px" style:height="var(--control-h)"></span><span class="skel" style:width="96px" style:height="var(--control-h)"></span></div>
  </div>
{/if}

<style>
  .person { display: grid; gap: var(--gap-page); min-width: 0; }
  .top { display: grid; gap: var(--gap-group); min-width: 0; }
  .actions { display: flex; flex-wrap: wrap; gap: var(--gap-control); }
  /* Each part: its points beside its name, and under the name its
     ticks and crosses. */
  .fit { display: grid; gap: 16px; }
  .fit .part { display: grid; grid-template-columns: 3ch minmax(0, 1fr); column-gap: 8px; row-gap: 8px; align-items: baseline; font-size: 14px; }
  .fit .pts { color: var(--ink-2); text-align: end; }
  .fit .part-label { font-weight: 600; }
  .why { grid-column: 2; display: grid; gap: 8px; }
  .why li { display: grid; grid-template-columns: 20px minmax(0, 1fr); gap: 8px; align-items: start; }
  .why .mark {
    width: 20px; height: 20px; border-radius: 50%; display: grid; place-items: center;
    background: var(--good-soft); color: var(--good);
  }
  .why .against .mark { background: var(--bad-soft); color: var(--bad); }
  .why .neither .mark { background: var(--surface-2); color: var(--ink-3); }
  .why .what { display: grid; gap: 2px; min-width: 0; line-height: 20px; }
  .why q { color: var(--ink-2); overflow-wrap: anywhere; line-height: 1.4; }
  .found { font-size: 13px; color: var(--ink-3); }
  .open-problem { color: var(--bad); }
  .rejected { color: var(--ink-3); }
  .note-state { color: var(--ink-3); }
</style>
