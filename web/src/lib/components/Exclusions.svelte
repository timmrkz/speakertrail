<script lang="ts">
  import { tick, untrack } from 'svelte'
  import { api, ApiError, type Exclusion, type Exclusions } from '../api'
  import { plural } from '../format'
  import { bringIntoView, undoHistory } from '../history.svelte'
  import { Load } from '../load.svelte'
  import { Problem } from '../problem.svelte'
  import { toast } from '../toast.svelte'
  import Icon from './Icon.svelte'
  import ProblemNote from './ProblemNote.svelte'

  // Tim's exclusions: words and phrases, like "Online Marketing", that
  // leave a person out of profile searches and People. Removing one brings
  // the people it hid back, and Undo in the Edit menu puts it back.
  const list = new Load<Exclusions>()
  $effect(() => {
    list.run(api.exclusions)
  })
  // What Undo and Redo took back or did again shows at once. The list
  // opens when it was an exclusion, and comes into view, so the change is
  // seen, whichever page the step was taken on.
  let open = $state(false)
  let box: HTMLDetailsElement | undefined = $state()
  const undoneAt = undoHistory.changed
  $effect(() => {
    if (undoHistory.changed === undoneAt) return
    untrack(() => list.run(api.exclusions, { quiet: true }))
  })
  $effect(() => {
    if (!undoHistory.reveal((t) => t.kind === 'exclusion')) return
    open = true
    void tick().then(() => box && bringIntoView(box, 'nearest'))
  })
  // In Go's order, with Go's counts, as adding and removing answer them.
  let all = $derived(list.data?.exclusions ?? [])
  let hiding = $derived(list.data?.hiding ?? 0)

  let phrase = $state('')
  // What went wrong adding or removing one says so under the field,
  // see lib/problem.svelte.ts.
  const problem = new Problem()
  let busy = $state(false)
  let field: HTMLInputElement | undefined = $state()

  async function add(input: string): Promise<boolean> {
    busy = true
    problem.clear()
    try {
      list.data = await api.addExclusion(input)
      return true
    } catch (err) {
      // One that is there already is not something that went wrong.
      if (err instanceof ApiError && err.status === 409) problem.needs(err.message)
      else problem.say(err)
      return false
    } finally {
      busy = false
    }
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    const input = phrase.trim()
    if (!input) {
      problem.needs('Enter a word or phrase, like Online Marketing')
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
    problem.clear()
    try {
      list.data = await api.deleteExclusion(x.id)
    } catch (err) {
      problem.failed(err)
    } finally {
      removing = null
    }
  }
</script>

<details class="exclusions" bind:open bind:this={box}>
  <summary title="Words that leave a person out of profile searches and People, matched in their headline and roles. People you connected with always stay">
    <span class="title">Exclusions</span>
    {#if list.data}<span class="faint small count">{all.length}{hiding ? `, hiding ${plural(hiding, 'person', 'people')}` : ''}</span>{/if}
  </summary>
  {#if list.error && !list.data}
    <p class="field-error">{list.error}</p>
  {:else}
    <ul class="phrases">
      {#each all as x (x.id)}
        <li class="phrase">
          <span class="ellipsis">{x.phrase}</span>
          {#if x.people}<span class="num" title="People it hides now">{x.people}</span>{/if}
          <button class="remove" type="button" aria-label="Remove {x.phrase}" title="Remove. The people it hides come back, and Undo puts it back"
            disabled={removing === x.id} onclick={() => remove(x)}><Icon name="x" size={14} /></button>
        </li>
      {/each}
    </ul>
    <form class="add" onsubmit={submit} novalidate>
      <label class="sr-only" for="new-exclusion">New exclusion</label>
      <input bind:this={field} bind:value={phrase} id="new-exclusion" class="input" type="text" placeholder="Online Marketing"
        autocomplete="off" maxlength="60" aria-invalid={problem.text ? 'true' : undefined} aria-describedby={problem.text ? 'exclusion-err' : undefined} />
      <button class="btn" type="submit" disabled={busy}><Icon name="plus" />{busy ? 'Adding' : 'Add'}</button>
    </form>
    <ProblemNote of={problem} id="exclusion-err" />
  {/if}
</details>

<style>
  .exclusions { display: block; }
  summary { display: flex; align-items: baseline; gap: 8px; cursor: pointer; list-style: revert; font-size: 14px; }
  .title { font-weight: 600; }
  .count { font-variant-numeric: tabular-nums; }
  details[open] > summary { margin-bottom: var(--gap-group); }
  .phrases { display: flex; flex-wrap: wrap; gap: var(--gap-control); margin: 0 0 var(--gap-group); padding: 0; list-style: none; min-width: 0; }
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
  .remove:hover { background: var(--hover); color: var(--ink); }
  .remove:active { background: var(--press); }
  .add { display: flex; gap: var(--gap-control); }
  .add .input { flex: 1; min-width: 0; }
  .add .btn { flex: none; }
</style>
