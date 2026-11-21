<script lang="ts">
  // The template a first message is put together from, in Settings. Each
  // part has its variations, and a draft picks one of each at random. A
  // variation saves the moment it is left, and Undo in the toast puts the
  // one before back, as for every setting. The app checks every change
  // and says what is wrong, the window only shows it. See
  // internal/store/template.go and docs/api.md, First message template.
  import { api, type FirstMessageTemplate, type TemplatePart } from '../api'
  import { plural } from '../format'
  import { Load } from '../load.svelte'
  import { errorText, toast } from '../toast.svelte'
  import Icon from './Icon.svelte'
  import Info from './Info.svelte'

  const tpl = new Load<FirstMessageTemplate>()
  tpl.run(api.template)

  // One message put together by the app, with what the app and the model
  // fill in shown by name, so Tim sees how the parts fit.
  let example = $state('')
  let exampleError = $state('')
  let rolling = $state(false)
  async function another() {
    rolling = true
    try {
      example = await api.templateExample()
      exampleError = ''
    } catch (e) {
      exampleError = errorText(e)
    } finally {
      rolling = false
    }
  }
  another()

  // The part whose change is on its way, and what the app said was wrong
  // with a change, by part.
  let working = $state<string | null>(null)
  let errors = $state<Record<string, string>>({})

  async function change(name: string, run: () => Promise<FirstMessageTemplate>): Promise<boolean> {
    working = name
    try {
      tpl.data = await run()
      errors = { ...errors, [name]: '' }
      another()
      return true
    } catch (e) {
      errors = { ...errors, [name]: errorText(e) }
      return false
    } finally {
      working = null
    }
  }

  // Gives a part its variations, and Undo the ones before.
  async function put(part: TemplatePart, variations: string[], said: string) {
    const was = part.variations
    if (await change(part.name, () => api.putTemplatePart(part.name, variations))) {
      toast.show(said, 'plain', {
        label: 'Undo',
        run: () => void change(part.name, () => api.putTemplatePart(part.name, was)),
      })
    }
  }

  function edited(part: TemplatePart, i: number, e: FocusEvent) {
    const value = (e.currentTarget as HTMLTextAreaElement).value
    if (value === part.variations[i]) return
    put(part, part.variations.map((x, j) => (j === i ? value : x)), `Saved ${part.name}`)
  }

  function removeVariation(part: TemplatePart, i: number) {
    put(part, part.variations.filter((_, j) => j !== i), `Removed a variation of ${part.name}`)
  }

  // Escape puts back what is saved.
  function keydown(part: TemplatePart, i: number, e: KeyboardEvent) {
    if (e.key !== 'Escape') return
    const box = e.currentTarget as HTMLTextAreaElement
    box.value = part.variations[i]
    errors = { ...errors, [part.name]: '' }
    box.blur()
  }

  // What is typed for a new variation, by part.
  let adding = $state<Record<string, string>>({})
  async function addVariation(part: TemplatePart, e: SubmitEvent) {
    e.preventDefault()
    const value = adding[part.name] ?? ''
    if (await change(part.name, () => api.putTemplatePart(part.name, [...part.variations, value]))) {
      adding = { ...adding, [part.name]: '' }
      const was = part.variations
      toast.show(value ? `Added a variation to ${part.name}` : `Added an empty variation to ${part.name}`, 'plain', {
        label: 'Undo',
        run: () => void change(part.name, () => api.putTemplatePart(part.name, was)),
      })
    }
  }

  async function removePart(part: TemplatePart, at: number) {
    if (await change(part.name, () => api.deleteTemplatePart(part.name))) {
      toast.show(`Removed ${part.name}`, 'plain', {
        label: 'Undo',
        run: () => void change(part.name, () => api.putTemplatePart(part.name, part.variations, at)),
      })
    }
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
      open = { ...open, [name]: true }
      toast.show(`Added ${name}. Use it as #${name}# in another part`)
    } catch (err) {
      newPartError = errorText(err)
    } finally {
      addingPart = false
    }
  }

  async function reset() {
    const was = tpl.data?.parts ?? []
    if (await change('', api.resetTemplate)) {
      toast.show('Put back the first template', 'plain', { label: 'Undo', run: () => void change('', () => api.putTemplate(was)) })
    }
  }

  // Which parts are open. Origin is, because the message starts there.
  let open = $state<Record<string, boolean>>({ origin: true })

  // A variation's box is as tall as its text, whatever the width, so it
  // shows whole and never scrolls inside.
  function grow(box: HTMLTextAreaElement) {
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

<section class="group template" aria-labelledby="head-template">
  <div class="headrow">
    <h2 id="head-template">Template</h2>
    <span class="ask">
      <Info label="About the template" side="left">
        A draft starts at origin and takes one variation of each part at random. #opener# in a variation stands for a
        variation of the part opener. An empty variation leaves the part out, and a variation written twice comes twice as
        often.
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
        <button class="btn sm" type="button" onclick={another} disabled={rolling} title="Put another message together">
          <Icon name="refresh" />Another
        </button>
      </div>
      {#if exampleError}
        <p class="field-error" role="alert">{exampleError}</p>
      {:else}
        <p class="example-text" aria-live="polite">{example}</p>
      {/if}
    </div>

    <div class="list parts">
      {#each tpl.data.parts as part, at (part.name)}
        <details class="part" bind:open={() => !!open[part.name], (v) => (open = { ...open, [part.name]: v })}>
          <summary>
            <span class="name num">{part.name}</span>
            <span class="faint small count">{plural(part.variations.length, 'variation', 'variations')}</span>
            <span class="turn" aria-hidden="true"><Icon name="chevron" size={14} /></span>
          </summary>
          <div class="body">
            <ul class="variations">
              {#each part.variations as v, i (`${i}:${v}`)}
                <li class="variation">
                  <textarea class="input text" rows="1" value={v} use:grow placeholder="Empty, leaves {part.name} out"
                    aria-label="Variation {i + 1} of {part.name}" spellcheck="false" disabled={working === part.name}
                    onblur={(e) => edited(part, i, e)} onkeydown={(e) => keydown(part, i, e)}></textarea>
                  <button class="btn icon quiet" type="button" aria-label="Remove variation {i + 1} of {part.name}"
                    title={part.variations.length === 1 ? 'A part needs one variation. Remove the part instead' : 'Remove this variation'}
                    disabled={working === part.name || part.variations.length === 1} onclick={() => removeVariation(part, i)}>
                    <Icon name="x" />
                  </button>
                </li>
              {/each}
            </ul>
            <form class="add" onsubmit={(e) => addVariation(part, e)} novalidate>
              <input class="input" type="text" placeholder="A new variation" aria-label="New variation of {part.name}"
                spellcheck="false" bind:value={() => adding[part.name] ?? '', (v) => (adding = { ...adding, [part.name]: v })} />
              <button class="btn" type="submit" disabled={working === part.name} title="Add it. Left empty, it leaves {part.name} out now and then">
                <Icon name="plus" /><span>Add</span>
              </button>
            </form>
            {#if errors[part.name]}<p class="field-error" role="alert">{errors[part.name]}</p>{/if}
            {#if part.name !== 'origin'}
              <button class="btn sm quiet danger remove-part" type="button" disabled={working === part.name}
                onclick={() => removePart(part, at)} title="Take the part out. A part another one uses stays">
                <Icon name="trash" />Remove {part.name}
              </button>
            {/if}
          </div>
        </details>
      {/each}
    </div>

    <form class="add" onsubmit={addPart} novalidate>
      <label class="sr-only" for="new-part">New part</label>
      <input id="new-part" class="input" type="text" placeholder="A new part, like wave" autocomplete="off" spellcheck="false"
        bind:value={newPart} aria-invalid={newPartError ? 'true' : undefined} />
      <button class="btn" type="submit" disabled={addingPart}><Icon name="plus" /><span>Add part</span></button>
    </form>
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
  .part summary { display: flex; align-items: baseline; gap: 8px; padding: 10px 12px; cursor: pointer; list-style: none; min-width: 0; }
  .part summary::-webkit-details-marker { display: none; }
  /* Whether a part opens, at its far end, turned while it is open. */
  .turn { margin-left: auto; align-self: center; display: inline-grid; color: var(--ink-3); transition: transform .15s; }
  .part[open] .turn { transform: rotate(90deg); }
  .name { font-weight: 600; font-size: 14px; }
  .count { font-variant-numeric: tabular-nums; }
  .body { display: grid; gap: 8px; padding: 0 12px 12px; min-width: 0; }
  .variations { display: grid; gap: 6px; margin: 0; padding: 0; list-style: none; min-width: 0; }
  .variation { display: flex; align-items: flex-start; gap: 6px; min-width: 0; }
  /* A field of one line that grows with its variation, never a second
     size of box. */
  .variation .text { flex: 1; min-width: 0; height: auto; min-height: var(--control-h); padding-top: 8px; padding-bottom: 8px;
    resize: none; line-height: 1.4; white-space: pre-wrap; overflow-wrap: anywhere; }
  .add { display: flex; gap: 8px; min-width: 0; }
  .add .input { flex: 1; min-width: 0; }
  .add .btn { flex: none; min-width: 7.4em; justify-content: center; }
  .remove-part, .reset { justify-self: start; }
</style>
