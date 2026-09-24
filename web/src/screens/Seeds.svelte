<script lang="ts">
  import { api, type Seed } from '../lib/api'
  import { fmtAgo } from '../lib/format'
  import { Load } from '../lib/load.svelte'
  import { errorText, toast } from '../lib/toast.svelte'
  import EmptyState from '../lib/components/EmptyState.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import Skeleton from '../lib/components/Skeleton.svelte'

  const seeds = new Load<Seed[]>()
  seeds.run(api.seeds)

  let input = $state('')
  let error = $state('')
  let busy = $state(false)

  $effect(() => {
    document.title = 'Seeds · Speaker Trail'
  })

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    const value = input.trim()
    if (!value) {
      error = 'Paste a link or type a name first.'
      return
    }
    busy = true
    error = ''
    try {
      const s = await api.addSeed(value)
      seeds.data = [s, ...(seeds.data ?? [])]
      input = ''
      toast.show('Seed added. The crawler follows it up soon.')
    } catch (err) {
      error = errorText(err)
    } finally {
      busy = false
    }
  }

  const isLink = (s: string) => /^https?:\/\//.test(s)
</script>

<div class="view narrow">
  <div class="view-head">
    <div>
      <h1>Seeds</h1>
      <p>Tips for the crawler. A link or a name becomes new sources, events and people.</p>
    </div>
  </div>

  <form class="panel" onsubmit={submit} novalidate>
    <div class="field">
      <label for="seed">Paste a link or a name you saw on LinkedIn</label>
      <div class="seed-row">
        <input id="seed" class="input" type="text" bind:value={input} placeholder="https://… or a person, event or organiser" autocomplete="off"
          aria-invalid={error ? 'true' : undefined} aria-describedby={error ? 'seed-err' : undefined} />
        <button class="btn primary" type="submit" disabled={busy}><Icon name="plus" />{busy ? 'Adding' : 'Add seed'}</button>
      </div>
      {#if error}<p id="seed-err" class="field-error" role="alert">{error}</p>{/if}
    </div>
  </form>

  {#if seeds.error && !seeds.data}
    <EmptyState icon="alert" tone="bad" title="Could not load the seeds" text={seeds.error}>
      <button class="btn" type="button" onclick={() => seeds.run(api.seeds)}><Icon name="refresh" />Try again</button>
    </EmptyState>
  {:else if !seeds.data}
    <Skeleton count={5} />
  {:else if !seeds.data.length}
    <EmptyState icon="seeds" title="No seeds yet" text="Paste the first link or name above." />
  {:else}
    <ul class="list">
      {#each seeds.data as s (s.id)}
        <li class="seed">
          <div class="s-top">
            {#if isLink(s.input)}
              <a class="s-input" href={s.input} target="_blank" rel="noopener noreferrer">{s.input}</a>
            {:else}
              <b class="s-input">{s.input}</b>
            {/if}
            {#if s.processed_at}
              <span class="pill good"><span class="dot"></span>Done</span>
            {:else}
              <span class="pill accent"><span class="dot"></span>Queued</span>
            {/if}
          </div>
          <p class="s-result" class:faint={!s.result}>
            {s.result || (s.processed_at ? 'Nothing came of it.' : 'Waiting for the crawler.')}
          </p>
          <p class="faint small">Added {fmtAgo(s.created_at)}{s.processed_at ? `, followed up ${fmtAgo(s.processed_at)}` : ''}</p>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .seed-row { display: flex; gap: 8px; }
  .seed-row .input { flex: 1; }
  @media (max-width: 520px) {
    .seed-row { flex-direction: column; }
    .seed-row .btn { width: 100%; }
  }
  .seed { padding: 12px 16px; display: grid; gap: 3px; }
  .s-top { display: flex; justify-content: space-between; gap: 10px; align-items: flex-start; }
  .s-input { font-weight: 600; overflow-wrap: anywhere; min-width: 0; }
  .s-result { font-size: 14px; }
</style>
