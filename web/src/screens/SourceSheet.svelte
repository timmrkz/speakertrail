<script lang="ts">
  import { api, SOURCE_STATUSES, type FetchMode, type Source, type SourcePatch, type SourceStatus } from '../lib/api'
  import { fmtAgo, fmtDateTime, KIND_LABEL, MODE_LABEL, plural, shortUrl, STATUS_LABEL } from '../lib/format'
  import { errorText, toast } from '../lib/toast.svelte'
  import HealthPill from '../lib/components/HealthPill.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import Sheet from '../lib/components/Sheet.svelte'

  let { source, onclose, onupdate }: { source: Source | undefined; onclose: () => void; onupdate: (s: Source) => void } = $props()

  const STATUS_HELP: Record<SourceStatus, string> = {
    candidate: 'Found but not checked enough yet',
    probation: 'Being tried out for a few weeks',
    active: 'Checked in every run',
    manual: 'Kept by hand, never retired',
    retired: 'Checked rarely, found nothing lately',
  }

  let notes = $state('')
  let noteState = $state<'idle' | 'saving' | 'saved'>('idle')
  let saving = $state(false)
  let checking = $state(false)
  let loadedFor = 0

  $effect(() => {
    if (source && loadedFor !== source.id) {
      loadedFor = source.id
      notes = source.notes
    }
  })

  async function patch(p: SourcePatch, done: string) {
    if (!source) return
    saving = true
    try {
      const updated = await api.patchSource(source.id, p)
      onupdate(updated)
      toast.show(done)
    } catch (e) {
      toast.show(errorText(e), 'bad')
    } finally {
      saving = false
    }
  }

  async function saveNotes() {
    if (!source || notes === source.notes) return
    noteState = 'saving'
    try {
      onupdate(await api.patchSource(source.id, { notes }))
      noteState = 'saved'
    } catch (e) {
      noteState = 'idle'
      toast.show(errorText(e), 'bad')
    }
  }

  async function checkNow() {
    if (!source) return
    checking = true
    try {
      await api.checkSource(source.id)
      toast.show('Check queued. Results show here once it ran.')
    } catch (e) {
      toast.show(errorText(e), 'bad')
    } finally {
      checking = false
    }
  }
</script>

<Sheet label={source?.name ?? 'Source'} {onclose}>
  {#snippet head()}
    {#if source}
      <div class="head">
        <h2>{source.name || shortUrl(source.url) || source.query}</h2>
        <p class="muted small">{[KIND_LABEL[source.kind] ?? source.kind, source.category, source.city].filter(Boolean).join(' · ')}</p>
        {#if source.url}
          <a class="small wrap-anywhere" href={source.url} target="_blank" rel="noopener noreferrer">{shortUrl(source.url)} <Icon name="external" size={12} /></a>
        {:else if source.query}
          <p class="small">Search for <b>{source.query}</b></p>
        {/if}
      </div>
    {:else}
      <div class="head"><span class="skel" style:width="60%" style:height="20px"></span><span class="skel" style:width="40%" style:height="12px"></span></div>
    {/if}
  {/snippet}

  {#if source}
    <div class="row">
      <button class="btn primary" type="button" onclick={checkNow} disabled={checking}><Icon name="refresh" />{checking ? 'Queueing' : 'Check now'}</button>
      {#if source.url}
        <a class="btn" href={source.url} target="_blank" rel="noopener noreferrer"><Icon name="external" />Open the page</a>
      {/if}
    </div>

    <section class="section health">
      <h3 class="label">Health</h3>
      <div class="row"><HealthPill health={source.health} />{#if source.health_note}<span class="small">{source.health_note}</span>{/if}</div>
      <dl class="kv">
        <dt>Last check</dt>
        <dd>{source.last_checked_at ? `${fmtDateTime(source.last_checked_at)}, ${fmtAgo(source.last_checked_at)}` : 'Never'}</dd>
        {#if source.last_check}
          <dt>Found</dt><dd>{plural(source.last_check.events_found, 'event')}</dd>
          <dt>Fetched with</dt><dd>{source.last_check.mode}{source.last_check.http_status ? `, HTTP ${source.last_check.http_status}` : ''}</dd>
          {#if source.last_check.error}<dt>Error</dt><dd class="err">{source.last_check.error}</dd>{/if}
        {/if}
        <dt>Next check</dt><dd>{source.next_check_at ? fmtDateTime(source.next_check_at) : 'Not planned'}</dd>
        <dt>Checks</dt><dd>{source.checks}{source.empty_checks_in_row ? `, the last ${source.empty_checks_in_row} empty` : ''}</dd>
        <dt>Points</dt><dd class="num">{source.points}</dd>
        <dt>Found via</dt><dd>{source.discovered_from || 'Unknown'}</dd>
      </dl>
    </section>

    <section class="section">
      <div class="settings-grid">
        <div class="field">
          <label for="src-status">Status</label>
          <select id="src-status" class="select" value={source.status} disabled={saving}
            onchange={(e) => {
              const v = (e.currentTarget as HTMLSelectElement).value as SourceStatus
              patch({ status: v }, `Status set to ${STATUS_LABEL[v].toLowerCase()}`)
            }}>
            {#each SOURCE_STATUSES as s (s)}<option value={s}>{STATUS_LABEL[s]}</option>{/each}
          </select>
          <span class="field-hint">{STATUS_HELP[source.status]}</span>
        </div>
        <div class="field">
          <label for="src-mode">How to fetch</label>
          <select id="src-mode" class="select" value={source.fetch_mode} disabled={saving}
            onchange={(e) => {
              const v = (e.currentTarget as HTMLSelectElement).value as FetchMode
              patch({ fetch_mode: v }, `Fetch mode set to ${MODE_LABEL[v].toLowerCase()}`)
            }}>
            <option value="auto">{MODE_LABEL.auto}</option>
            <option value="http">{MODE_LABEL.http}</option>
            <option value="browser">{MODE_LABEL.browser}</option>
          </select>
          <span class="field-hint">{source.fetch_mode === 'browser' ? 'Opens the page in a browser first. Slower.' : source.fetch_mode === 'http' ? 'Reads the raw page. Fast.' : 'Tries the raw page, then a browser if needed'}</span>
        </div>
      </div>
    </section>

    <section class="section">
      <div class="row between">
        <label class="label" for="src-notes">Notes</label>
        <span class="small faint" aria-live="polite">{noteState === 'saving' ? 'Saving' : noteState === 'saved' ? 'Saved' : ''}</span>
      </div>
      <textarea id="src-notes" class="textarea" rows="3" bind:value={notes} oninput={() => (noteState = 'idle')} onblur={saveNotes}
        placeholder="Anything worth knowing about this source. Saves when you leave the field."></textarea>
    </section>
  {:else}
    <div class="loading">{#each Array(4) as _, i (i)}<span class="skel" style:height="44px"></span>{/each}</div>
  {/if}
</Sheet>

<style>
  .head { display: grid; gap: 4px; min-width: 0; }
  .head h2 { font-size: 19px; overflow-wrap: anywhere; }
  .head a :global(svg) { vertical-align: -1px; }
  .settings-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
  @media (max-width: 420px) { .settings-grid { grid-template-columns: 1fr; } }
  .err { color: var(--bad); }
  .loading { display: grid; gap: 10px; }
</style>
