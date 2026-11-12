<script lang="ts">
  import { api, type Exclusion } from '../api'
  import { plural } from '../format'
  import { Load } from '../load.svelte'
  import { errorText, toast } from '../toast.svelte'
  import Icon from './Icon.svelte'

  // Tim's exclusions: words and phrases, like "Online Marketing", that
  // leave a person out of profile searches and People. Removing one brings
  // the people it hid back, and Undo adds it again.
  const list = new Load<Exclusion[]>()
  $effect(() => {
    list.run(api.exclusions)
  })
  let all = $derived(list.data ?? [])
  let hiding = $derived(all.reduce((n, x) => n + x.people, 0))

  let phrase = $state('')
  let error = $state('')
  let busy = $state(false)
  let field: HTMLInputElement | undefined = $state()

  async function add(input: string): Promise<boolean> {
    busy = true
    error = ''
    try {
      const x = await api.addExclusion(input)
      list.data = [...all, x].sort((a, b) => a.phrase.localeCompare(b.phrase, 'de'))
      return true
    } catch (err) {
      error = errorText(err)
      return false
    } finally {
      busy = false
    }
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    const input = phrase.trim()
    if (!input) {
      error = 'Enter a word or phrase, like Online Marketing'
      field?.focus()
      return
    }
    if (await add(input)) {
      phrase = ''
      toast.show(`Leaving out ${input}`)
    }
  }

  let removing = $state<number | null>(null)
  async function remove(x: Exclusion) {
    removing = x.id
    try {
      await api.deleteExclusion(x.id)
      list.data = all.filter((y) => y.id !== x.id)
      toast.show(`Removed ${x.phrase}`, 'plain', { label: 'Undo', run: () => void add(x.phrase) })
    } catch (err) {
      toast.show(errorText(err), 'bad')
    } finally {
      removing = null
    }
  }
</script>

<details class="exclusions">
  <summary title="Words that leave a person out of profile searches and People, matched in their headline and roles. People you connected with always stay">
    <span class="title">Exclusions</span>
    {#if list.data}<span class="faint small num">{all.length}{hiding ? `, hiding ${plural(hiding, 'person', 'people')}` : ''}</span>{/if}
  </summary>
  {#if list.error && !list.data}
    <p class="field-error">{list.error}</p>
  {:else}
    <ul class="phrases">
      {#each all as x (x.id)}
        <li class="phrase">
          <span class="ellipsis">{x.phrase}</span>
          {#if x.people}<span class="num" title="People it hides now">{x.people}</span>{/if}
          <button class="remove" type="button" aria-label="Remove {x.phrase}" title="Remove. The people it hides come back"
            disabled={removing === x.id} onclick={() => remove(x)}><Icon name="x" size={14} /></button>
        </li>
      {/each}
    </ul>
    <form class="add" onsubmit={submit} novalidate>
      <label class="sr-only" for="new-exclusion">New exclusion</label>
      <input bind:this={field} bind:value={phrase} id="new-exclusion" class="input" type="text" placeholder="Online Marketing"
        autocomplete="off" maxlength="60" aria-invalid={error ? 'true' : undefined} aria-describedby={error ? 'exclusion-err' : undefined} />
      <button class="btn" type="submit" disabled={busy}><Icon name="plus" /><span>{busy ? 'Adding' : 'Add'}</span></button>
    </form>
    {#if error}<p id="exclusion-err" class="field-error" role="alert">{error}</p>{/if}
  {/if}
</details>

<style>
  .exclusions { display: grid; gap: 10px; }
  summary { display: flex; align-items: baseline; gap: 8px; cursor: pointer; list-style: revert; font-size: 14px; }
  .title { font-weight: 600; }
  details[open] > summary { margin-bottom: 10px; }
  .phrases { display: flex; flex-wrap: wrap; gap: 6px; margin: 0 0 10px; padding: 0; list-style: none; min-width: 0; }
  /* Shaped like a chip, but a label with its own remove button. */
  .phrase {
    height: 32px; padding: 0 2px 0 12px; border-radius: 999px; border: 1px solid var(--line); background: var(--surface);
    font-size: 13px; font-weight: 600; color: var(--ink-2); display: inline-flex; align-items: center; gap: 6px; max-width: 100%;
  }
  .phrase .num { font-size: 12px; opacity: .7; font-variant-numeric: tabular-nums; }
  .remove {
    width: 28px; height: 28px; border-radius: 50%; border: 0; background: none; color: var(--ink-3); cursor: pointer;
    display: inline-grid; place-items: center; flex: none;
  }
  .remove:hover { background: var(--surface-2); color: var(--ink); }
  .add { display: flex; gap: 8px; }
  .add .input { flex: 1; min-width: 0; }
  .add .btn { flex: none; min-width: 6.4em; justify-content: center; }
</style>
