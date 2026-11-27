<script lang="ts">
  import { api } from '../api'
  import { exaWatch } from '../exa.svelte'
  import { errorText } from '../problem.svelte'
  import { toast } from '../toast.svelte'

  // The Exa key, which every run needs. It goes into a file only Tim's
  // account can read and never comes back to the window: the row only
  // learns whether it is saved.
  const use = $derived(exaWatch.exa)
  let key = $state('')
  let status = $state<'idle' | 'saving' | 'removing'>('idle')
  let error = $state('')

  function load() {
    exaWatch.read().catch((e) => (error = errorText(e)))
  }
  load()

  async function save(e: SubmitEvent) {
    e.preventDefault()
    if (!key.trim()) {
      error = 'Paste the key from dashboard.exa.ai'
      return
    }
    status = 'saving'
    error = ''
    try {
      await api.setKey(key.trim())
      key = ''
      toast.show('Saved the Exa key')
      load()
    } catch (err) {
      error = errorText(err)
    } finally {
      status = 'idle'
    }
  }

  async function remove() {
    status = 'removing'
    error = ''
    try {
      await api.setKey('')
      toast.show('Removed the Exa key')
      load()
    } catch (err) {
      error = errorText(err)
    } finally {
      status = 'idle'
    }
  }
</script>

<section class="group">
  <h2>Exa</h2>
  <div class="list">
    <form class="setting" onsubmit={save} novalidate>
      <div class="info">
        <label for="exa-key" class="desc">The key runs ask Exa with</label>
        <span class="key small" title="Kept on this Mac in a file only you can read, exa-key in the app's folder">
          {#if !use}&nbsp;{:else if use.set}Saved · {use.used} of {use.budget} searches this month{:else}Not saved yet. Runs need it{/if}
        </span>
      </div>
      <div class="editor">
        <input id="exa-key" class="input" type="password" autocomplete="off" spellcheck="false" bind:value={key}
          placeholder={use?.set ? 'Paste a new key to replace it' : 'Paste the key from dashboard.exa.ai'}
          aria-invalid={error ? 'true' : undefined} oninput={() => (error = '')} />
        <button class="btn primary" type="submit" disabled={status !== 'idle'}>{status === 'saving' ? 'Saving' : 'Save'}</button>
        {#if use?.set}
          <button class="btn" type="button" onclick={remove} disabled={status !== 'idle'}
            title="Deletes the key from this Mac. Runs stop until a key is saved again">{status === 'removing' ? 'Removing' : 'Remove'}</button>
        {/if}
      </div>
      <p class="foot small" aria-live="polite">{#if error}<span class="field-error">{error}</span>{/if}</p>
    </form>
  </div>
</section>

<style>
  .group { display: grid; gap: 10px; }
  .group h2 { font-size: 15px; margin: 0; }
  .setting {
    display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.1fr); gap: 6px 20px; padding: 16px; align-items: start;
  }
  .info { display: grid; gap: 2px; min-width: 0; }
  .desc { font-weight: 600; font-size: 14px; }
  .key { color: var(--ink-3); }
  .editor { display: flex; gap: var(--gap-control); align-items: flex-start; min-width: 0; }
  .editor .input { flex: 1; min-width: 0; }
  .foot { grid-column: 2; min-height: 0; margin: 0; }
  @media (max-width: 760px) {
    .setting { grid-template-columns: minmax(0, 1fr); padding: 14px; }
    .foot { grid-column: 1; }
  }
</style>
