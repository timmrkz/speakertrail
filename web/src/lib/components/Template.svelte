<script lang="ts">
  // The template a first message is put together from, in Settings. Each
  // part has its variations, and a draft picks one of each at random. A
  // variation saves the moment it is left, and Undo in the Edit menu puts
  // the one before back, as for every setting. The app checks every
  // change and says what is wrong, the window only shows it. See
  // internal/store/template.go and docs/api.md, First message template.
  import { tick, untrack } from 'svelte'
  import { api, type FirstMessageTemplate, type TemplatePart } from '../api'
  import { plural } from '../format'
  import { bringIntoView, undoHistory } from '../history.svelte'
  import { Load } from '../load.svelte'
  import { errorText, toast } from '../toast.svelte'
  import Icon from './Icon.svelte'
  import Info from './Info.svelte'

  const tpl = new Load<FirstMessageTemplate>()
  tpl.run(api.template)
  // What Undo and Redo in the Edit menu took back or did again shows at
  // once, in the example too. Only a change after Settings opened counts.
  const undoneAt = undoHistory.changed
  $effect(() => {
    if (undoHistory.changed === undoneAt) return
    untrack(() => {
      errors = {}
      void tpl.run(api.template, { quiet: true }).then(showExample)
    })
  })
  // The template comes into view when Undo or Redo changed it.
  let section: HTMLElement | undefined = $state()
  $effect(() => {
    if (undoHistory.reveal((t) => t.kind === 'template')) void tick().then(() => section && bringIntoView(section, 'nearest'))
  })

  // One message put together by the app, with what the app and the model
  // fill in shown by name, so Tim sees how the parts fit. It keeps its
  // seed, so a change shows in the same message, which keeps its shape
  // and moves nothing on the page. New example picks another seed.
  let example = $state('')
  let exampleError = $state('')
  let rolling = $state(false)
  let seed = newSeed()
  function newSeed() {
    return Math.floor(Math.random() * 2 ** 31)
  }
  async function showExample() {
    try {
      example = await api.templateExample(seed)
      exampleError = ''
    } catch (e) {
      exampleError = errorText(e)
    }
  }
  // Only New example says it is working, so a save leaves its button be.
  async function newExample() {
    seed = newSeed()
    rolling = true
    await showExample()
    rolling = false
  }
  showExample()

  // The part whose change is on its way, and what the app said was wrong
  // with a change, by part.
  let working = $state<string | null>(null)
  let errors = $state<Record<string, string>>({})

  // Changes go to the app one after another, each from the template the
  // one before it left, so a box left while another saves loses nothing.
  let queue: Promise<unknown> = Promise.resolve()
  function change(name: string, run: () => Promise<FirstMessageTemplate> | undefined): Promise<boolean> {
    const done = queue.then(async () => {
      const asked = run()
      if (!asked) return false
      working = name
      try {
        tpl.data = await asked
        errors = { ...errors, [name]: '' }
        showExample()
        return true
      } catch (e) {
        errors = { ...errors, [name]: errorText(e) }
        return false
      } finally {
        working = null
      }
    })
    queue = done
    return done
  }

  const latest = (name: string) => tpl.data?.parts.find((p) => p.name === name)?.variations ?? []

  // Gives a part the variations make makes of the ones it has then.
  function put(name: string, make: (was: string[]) => string[]): Promise<boolean> {
    return change(name, () => {
      const was = latest(name)
      const next = make(was)
      return JSON.stringify(next) === JSON.stringify(was) ? undefined : api.putTemplatePart(name, next)
    })
  }

  function edited(name: string, i: number, e: FocusEvent) {
    const value = (e.currentTarget as HTMLTextAreaElement).value
    put(name, (was) => was.map((x, j) => (j === i ? value : x)))
  }

  function removeVariation(name: string, i: number) {
    put(name, (was) => was.filter((_, j) => j !== i))
  }

  // Escape puts back what is saved.
  function keydown(name: string, i: number, e: KeyboardEvent) {
    if (e.key !== 'Escape') return
    const box = e.currentTarget as HTMLTextAreaElement
    box.value = latest(name)[i] ?? ''
    errors = { ...errors, [name]: '' }
    box.blur()
  }

  // What is typed for a new variation, by part.
  let adding = $state<Record<string, string>>({})
  async function addVariation(name: string, e: SubmitEvent) {
    e.preventDefault()
    const value = adding[name] ?? ''
    await put(name, (was) => [...was, value])
    if (!errors[name]) adding = { ...adding, [name]: '' }
  }

  // The × of a part the app keeps says why instead.
  function removePart(part: TemplatePart, e: MouseEvent) {
    e.preventDefault()
    e.stopPropagation()
    if (part.kept) {
      toast.show(part.kept)
      return
    }
    void change(part.name, () => api.deleteTemplatePart(part.name))
  }

  let newPart = $state('')
  let newPartError = $state('')
  let addingPart = $state(false)
  async function addPart(e: SubmitEvent) {
    e.preventDefault()
    const name = newPart.trim()
    if (!name) {
      newPartError = 'Name the part, like wave. Then use it as #wave# in another part'
      return
    }
    addingPart = true
    try {
      tpl.data = await api.putTemplatePart(name, [''])
      newPart = newPartError = ''
      openPart = name
      toast.show(`Added ${name}. Use it as #${name}# in another part`)
    } catch (err) {
      newPartError = errorText(err)
    } finally {
      addingPart = false
    }
  }

  function reset() {
    void change('', api.resetTemplate)
  }

  // The part that is open, one at a time, so the list stays short enough
  // to see whole. Origin is first, because the message starts there.
  let openPart = $state<string | null>('origin')
  const rows: Record<string, HTMLDetailsElement> = {}
  // The part above that closes would pull the one clicked up under the
  // pointer. Chromium holds it in place by itself, WebKit does not, so
  // the page scrolls back by what it moved.
  async function toggled(name: string, v: boolean) {
    if (!v) {
      if (openPart === name) openPart = null
      return
    }
    const row = rows[name]
    const was = row?.getBoundingClientRect().top
    openPart = name
    await tick()
    if (!row || was === undefined) return
    const moved = row.getBoundingClientRect().top - was
    let scroller = row.parentElement
    while (scroller && !/(auto|scroll)/.test(getComputedStyle(scroller).overflowY)) scroller = scroller.parentElement
    if (Math.abs(moved) >= 1) (scroller ?? document.scrollingElement)?.scrollBy(0, moved)
  }

  // A variation's box is as tall as its text, whatever the width, so it
  // shows whole and never scrolls inside.
  function grow(box: HTMLTextAreaElement, _value: string) {
    const fit = () => {
      box.style.height = 'auto'
      box.style.height = `${box.scrollHeight + box.offsetHeight - box.clientHeight}px`
    }
    fit()
    box.addEventListener('input', fit)
    const seen = new ResizeObserver(fit)
    seen.observe(box)
    return {
      update: fit,
      destroy() {
        box.removeEventListener('input', fit)
        seen.disconnect()
      },
    }
  }
  const fieldBy = (by: string) => (by === 'model' ? 'the language model' : 'the app')
</script>

<section class="group template" aria-labelledby="head-template" bind:this={section}>
  <div class="headrow">
    <h2 id="head-template">Template</h2>
    <span class="ask">
      <Info label="About the template" side="left">
        A draft starts at origin and takes one variation of each part at random. #opener# in a variation stands for a
        variation of the part opener. An empty variation leaves the part out now and then, a part with none leaves it out
        always, and a variation written twice comes twice as often.
        {#if tpl.data}
          {#each tpl.data.fields as f (f.name)}<br />#{f.name}# is filled in by {fieldBy(f.by)}: {f.about}.{/each}
        {/if}
      </Info>
    </span>
  </div>

  {#if tpl.error && !tpl.data}
    <p class="field-error" role="alert">{tpl.error}</p>
  {:else if tpl.data}
    <div class="example">
      <div class="row between">
        <span class="label">Example</span>
        <button class="btn sm" type="button" onclick={newExample} disabled={rolling}
          title="Put another example together, with a variation of each part picked at random">
          <Icon name="refresh" />New example
        </button>
      </div>
      {#if exampleError}
        <p class="field-error" role="alert">{exampleError}</p>
      {:else}
        <p class="example-text" aria-live="polite">{example}</p>
      {/if}
    </div>

    <div class="list parts">
      {#each tpl.data.parts as part (part.name)}
        <details class="part" bind:this={rows[part.name]} bind:open={() => openPart === part.name, (v) => toggled(part.name, v)}>
          <summary>
            <span class="turn" aria-hidden="true"><Icon name="chevron" size={14} /></span>
            <span class="name num">{part.name}</span>
            <span class="faint small count">{plural(part.variations.length, 'variation', 'variations')}</span>
            <!-- The app says why a part stays. Its × says it when pressed. -->
            <button class="btn icon quiet remove-part" type="button" aria-label="Remove {part.name}"
              aria-disabled={part.kept ? 'true' : undefined} disabled={working === part.name}
              title={part.kept ?? 'Take the part out. Undo brings it back'} onclick={(e) => removePart(part, e)}>
              <Icon name="x" />
            </button>
          </summary>
          <div class="body">
            <ul class="variations">
              <!-- Keyed by place, so a box being typed in stays itself while
                   the app answers for another. -->
              {#each part.variations as v, i (i)}
                <li class="variation">
                  <textarea class="input text" rows="1" value={v} use:grow={v} placeholder="Empty, leaves {part.name} out"
                    aria-label="Variation {i + 1} of {part.name}" spellcheck="false"
                    onblur={(e) => edited(part.name, i, e)} onkeydown={(e) => keydown(part.name, i, e)}></textarea>
                  <button class="btn icon quiet" type="button" aria-label="Remove variation {i + 1} of {part.name}"
                    title="Remove this variation" onclick={() => removeVariation(part.name, i)}>
                    <Icon name="x" />
                  </button>
                </li>
              {/each}
              <!-- A new variation is one more row of the same shape, its +
                   where the × of the others is. -->
              <li>
                <form class="variation add" onsubmit={(e) => addVariation(part.name, e)} novalidate>
                  <input class="input text" type="text" placeholder="A new variation" aria-label="New variation of {part.name}"
                    spellcheck="false" bind:value={() => adding[part.name] ?? '', (v) => (adding = { ...adding, [part.name]: v })} />
                  <button class="btn icon quiet" type="submit" disabled={working === part.name} aria-label="Add a variation to {part.name}"
                    title="Add it. Left empty, it leaves {part.name} out now and then">
                    <Icon name="plus" />
                  </button>
                </form>
              </li>
            </ul>
            {#if errors[part.name]}<p class="field-error" role="alert">{errors[part.name]}</p>{/if}
          </div>
        </details>
      {/each}
      <form class="add-part" onsubmit={addPart} novalidate>
        <label class="sr-only" for="new-part">New part</label>
        <input id="new-part" class="input" type="text" placeholder="A new part, like wave" autocomplete="off" spellcheck="false"
          bind:value={newPart} aria-invalid={newPartError ? 'true' : undefined} />
        <button class="btn icon quiet" type="submit" disabled={addingPart} aria-label="Add part"
          title="Add the part. Then use it as #name# in another part">
          <Icon name="plus" />
        </button>
      </form>
    </div>

    {#if newPartError}<p class="field-error" role="alert">{newPartError}</p>{/if}
    {#if errors['']}<p class="field-error" role="alert">{errors['']}</p>{/if}

    {#if tpl.data.changed}
      <button class="btn sm quiet reset" type="button" onclick={reset} disabled={working === ''}
        title="Put back the template Tim wrote first. Undo brings yours back">
        <Icon name="refresh" />Put back the first template
      </button>
    {/if}
  {/if}
</section>

<style>
  .template h2 { font-size: 15px; margin: 0; }
  .headrow { display: flex; align-items: center; justify-content: space-between; gap: 6px; }
  .headrow .ask { align-self: flex-start; margin: 1px 1px 0 0; }
  .example { display: grid; gap: 8px; min-width: 0; }
  /* The message as Tim would send it, line breaks and all, and text only. */
  .example-text { white-space: pre-wrap; overflow-wrap: anywhere; font-size: 14px; line-height: 1.5; color: var(--ink-2); margin: 0; }
  /* Every row of the list ends in one column of icons, the × of a part,
     of its variations and the + that adds, all the same size. */
  .part summary { display: flex; align-items: center; gap: 8px; padding: 6px 12px; cursor: pointer; list-style: none; min-width: 0; }
  .part summary::-webkit-details-marker { display: none; }
  /* Whether a part is open, before its name, turned while it is. */
  .turn { display: inline-grid; color: var(--ink-3); transition: transform .15s; }
  .part[open] .turn { transform: rotate(90deg); }
  .remove-part { margin-left: auto; flex: none; }
  .remove-part[aria-disabled='true'] { opacity: .55; cursor: default; }
  .name { font-weight: 600; font-size: 14px; }
  .count { font-variant-numeric: tabular-nums; }
  .body { display: grid; gap: 8px; padding: 0 12px 12px; min-width: 0; }
  .variations { display: grid; gap: var(--gap-control); margin: 0; padding: 0; list-style: none; min-width: 0; }
  .variation { display: flex; align-items: flex-start; gap: var(--gap-control); min-width: 0; }
  /* A field of one line that grows with its variation, never a second
     size of box. */
  .variation .text { flex: 1; min-width: 0; height: auto; min-height: var(--control-h); padding-top: 8px; padding-bottom: 8px;
    resize: none; line-height: 1.4; white-space: pre-wrap; overflow-wrap: anywhere; }
  .variation .btn { flex: none; }
  .add-part { display: flex; gap: var(--gap-control); padding: 6px 12px; min-width: 0; }
  .add-part .input { flex: 1; min-width: 0; }
  .add-part .btn { flex: none; }
  .reset { justify-self: start; }
</style>
