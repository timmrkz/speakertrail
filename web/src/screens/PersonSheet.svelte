<script lang="ts">
  import { api, type PersonDetail, type Profile, type Review } from '../lib/api'
  import { fmtAgo, fmtDateTime, initials, PLATFORM_LABEL, ROLE_LABEL, shortUrl } from '../lib/format'
  import { Load } from '../lib/load.svelte'
  import { errorText, toast } from '../lib/toast.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import Sheet from '../lib/components/Sheet.svelte'

  let { id, onclose, onupdate }: { id: number; onclose: () => void; onupdate?: (p: PersonDetail) => void } = $props()

  const person = new Load<PersonDetail>()
  let notes = $state('')
  let savedNotes = $state('')
  let noteState = $state<'idle' | 'saving' | 'saved' | 'error'>('idle')
  let busyProfile = $state<number | null>(null)

  // The parent keys this sheet by id, so the id never changes here.
  function load() {
    person.run(() => api.person(id)).then(() => {
      notes = savedNotes = person.data?.notes ?? ''
    })
  }
  load()

  let now = new Date().toISOString()
  let upcoming = $derived((person.data?.appearances ?? []).filter((a) => a.event.starts_at >= now))
  let past = $derived((person.data?.appearances ?? []).filter((a) => a.event.starts_at < now).reverse())

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

  async function review(p: Profile, next: Review) {
    if (!person.data) return
    busyProfile = p.id
    try {
      await api.patchProfile(p.id, next)
      person.data = { ...person.data, profiles: person.data.profiles.map((x) => (x.id === p.id ? { ...x, review: next } : x)) }
      onupdate?.(person.data)
      const what = PLATFORM_LABEL[p.platform]
      toast.show(next === 'confirmed' ? `${what} profile confirmed` : next === 'rejected' ? `${what} profile rejected` : `${what} profile back to open`)
    } catch (e) {
      toast.show(errorText(e), 'bad')
    } finally {
      busyProfile = null
    }
  }
</script>

<Sheet label={person.data?.name ?? 'Person'} {onclose}>
  {#snippet head()}
    {#if person.data}
      <div class="head">
        <span class="avatar lg" aria-hidden="true">{initials(person.data.name)}</span>
        <div class="head-text">
          <h2>{person.data.name}</h2>
          {#if person.data.known_as}<p class="faint small">Also known as {person.data.known_as}</p>{/if}
          <p class="muted">{person.data.headline || 'No headline yet'}</p>
          <p class="faint small">{[person.data.city, `first seen ${fmtAgo(person.data.first_seen)}`].filter(Boolean).join(' · ')}</p>
        </div>
      </div>
    {:else}
      <div class="head">
        <span class="avatar lg skel" aria-hidden="true"></span>
        <div class="head-text"><span class="skel" style:width="60%" style:height="18px"></span><span class="skel" style:width="80%" style:height="12px"></span></div>
      </div>
    {/if}
  {/snippet}

  {#if person.error && !person.data}
    <EmptyState icon="alert" tone="bad" title="Could not load this person" text={person.error}>
      <button class="btn" type="button" onclick={load}>Try again</button>
    </EmptyState>
  {:else if person.data}
    {@const p = person.data}
    <section class="section" aria-labelledby="apps-h">
      <h3 id="apps-h" class="label">On stage</h3>
      {#if !p.appearances.length}
        <p class="muted small">No appearances recorded.</p>
      {/if}
      {#each [{ list: upcoming, label: 'Upcoming' }, { list: past, label: 'Earlier' }] as group (group.label)}
        {#if group.list.length}
          <ul class="apps" aria-label={group.label}>
            {#each group.list as a (a.event.id + a.role)}
              <li class:past={group.label === 'Earlier'}>
                <span class="when num">{fmtDateTime(a.event.starts_at)}</span>
                <span class="what">
                  {#if a.event.url}
                    <a href={a.event.url} target="_blank" rel="noopener noreferrer">{a.event.title}<Icon name="external" size={12} /></a>
                  {:else}
                    <b>{a.event.title}</b>
                  {/if}
                  <span class="faint">{ROLE_LABEL[a.role] ?? a.role} · {[a.event.venue, a.event.city].filter(Boolean).join(', ')}</span>
                  {#if a.evidence}<q class="evidence" title="From the event page">{a.evidence}</q>{/if}
                </span>
              </li>
            {/each}
          </ul>
        {/if}
      {/each}
    </section>

    <section class="section" aria-labelledby="prof-h">
      <h3 id="prof-h" class="label">Profiles</h3>
      {#if p.profiles.length}
        <ul class="profiles">
          {#each p.profiles as prof (prof.id)}
            <li class="profile {prof.review}">
              <div class="prof-main">
                <span class="where">{PLATFORM_LABEL[prof.platform]}
                  {#if prof.review === 'confirmed'}<span class="pill good"><Icon name="check" size={12} />Confirmed</span>{/if}
                  {#if prof.review === 'rejected'}<span class="pill">Rejected</span>{/if}
                </span>
                <a href={prof.url} target="_blank" rel="noopener noreferrer">{shortUrl(prof.url)}</a>
              </div>
              <div class="row prof-actions">
                {#if prof.review === 'open'}
                  <button class="btn sm" type="button" disabled={busyProfile === prof.id} onclick={() => review(prof, 'confirmed')}><Icon name="check" size={14} />Confirm</button>
                  <button class="btn sm" type="button" disabled={busyProfile === prof.id} onclick={() => review(prof, 'rejected')}><Icon name="x" size={14} />Reject</button>
                {:else}
                  <button class="btn sm quiet" type="button" disabled={busyProfile === prof.id} onclick={() => review(prof, 'open')}><Icon name="undo" size={14} />Undo</button>
                {/if}
              </div>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="muted small">No profile found yet. The crawler looks again later.</p>
      {/if}
    </section>

    {#if p.affiliations.length}
      <section class="section" aria-labelledby="aff-h">
        <h3 id="aff-h" class="label">Affiliations</h3>
        <ul class="plain">
          {#each p.affiliations as a (a.organisation + a.role)}
            <li><b>{a.organisation}</b> <span class="faint">{a.role}{a.current ? '' : ', before'}</span></li>
          {/each}
        </ul>
      </section>
    {/if}

    <section class="section" aria-labelledby="seen-h">
      <h3 id="seen-h" class="label">Seen on</h3>
      {#if p.sightings.length}
        <ul class="plain">
          {#each p.sightings as s (s.source_id)}
            <li><a href="/app/sources/{s.source_id}">{s.source}</a> <span class="faint">checked {fmtAgo(s.checked_at)}</span></li>
          {/each}
        </ul>
      {:else}
        <p class="muted small">No sightings recorded.</p>
      {/if}
    </section>

    <section class="section">
      <div class="row between">
        <label class="label" for="notes-{id}">Notes</label>
        <span class="small note-state" aria-live="polite">
          {noteState === 'saving' ? 'Saving' : noteState === 'saved' ? 'Saved' : noteState === 'error' ? 'Not saved' : ''}
        </span>
      </div>
      <textarea
        id="notes-{id}"
        class="textarea"
        rows="4"
        bind:value={notes}
        oninput={() => (noteState = 'idle')}
        onblur={saveNotes}
        placeholder="What you know, where you met, what to ask. Saves when you leave the field."></textarea>
    </section>
  {:else}
    <div class="loading">
      {#each Array(4) as _, i (i)}<span class="skel" style:height="44px"></span>{/each}
    </div>
  {/if}
</Sheet>

<style>
  .head { display: grid; grid-template-columns: 52px minmax(0, 1fr); gap: 14px; align-items: center; }
  .head-text { display: grid; gap: 3px; min-width: 0; }
  .head-text h2 { font-size: 19px; }
  .head-text p { overflow-wrap: anywhere; }
  .apps { display: grid; gap: 8px; }
  .apps li { display: grid; gap: 1px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; }
  .apps li.past { background: var(--surface-2); border-color: transparent; }
  .apps .when { font-size: 12px; color: var(--ink-2); }
  .apps .what { display: grid; font-size: 14px; min-width: 0; }
  .apps .what a { font-weight: 600; overflow-wrap: anywhere; }
  .apps .what a :global(svg) { margin-left: 4px; vertical-align: -1px; }
  .apps .what .faint { font-size: 13px; }
  .apps .evidence { margin-top: 4px; font-size: 13px; font-style: italic; color: var(--ink-2); overflow-wrap: anywhere; }
  .profiles { display: grid; gap: 8px; }
  .profile {
    display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 10px; align-items: center; padding: 10px 12px;
    border: 1px solid var(--line); border-radius: 8px;
  }
  .profile.confirmed { border-color: var(--good); background: var(--good-soft); }
  .profile.rejected { opacity: .65; }
  .profile.rejected a { text-decoration: line-through; }
  .prof-main { display: grid; gap: 2px; min-width: 0; }
  .where { display: flex; gap: 8px; align-items: center; font-size: 13px; color: var(--ink-2); font-weight: 600; }
  .prof-main a { font-weight: 600; font-size: 14px; overflow-wrap: anywhere; }
  .prof-actions { flex-wrap: nowrap; }
  .plain { display: grid; gap: 6px; font-size: 14px; }
  .note-state { color: var(--ink-3); }
  .loading { display: grid; gap: 10px; }
  @media (max-width: 420px) {
    .profile { grid-template-columns: minmax(0, 1fr); }
  }
</style>
