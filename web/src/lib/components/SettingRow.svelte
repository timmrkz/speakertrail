<script lang="ts">
  import { tick } from 'svelte'
  import { api, type Json, type Setting } from '../api'
  import { fmtAgo } from '../format'
  import { bringIntoView, undoHistory } from '../history.svelte'
  import { errorText } from '../toast.svelte'

  // One setting with an editor that fits its JSON type. Bare, it is the
  // editor alone, named by the heading above it, labelledby, with no row
  // round it: the prompt in Settings.
  let { setting, onsaved, bare = false, labelledby }: {
    setting: Setting
    onsaved: (s: Setting) => void
    bare?: boolean
    labelledby?: string
  } = $props()

  type Kind = 'boolean' | 'number' | 'string' | 'list' | 'json'
  function kindOf(v: Json): Kind {
    if (typeof v === 'boolean') return 'boolean'
    if (typeof v === 'number') return 'number'
    if (typeof v === 'string') return 'string'
    if (Array.isArray(v) && v.every((x) => typeof x === 'string')) return 'list'
    return 'json'
  }
  function toText(v: Json, k: Kind): string {
    if (k === 'list') return (v as string[]).join('\n')
    if (k === 'json') return JSON.stringify(v, null, 2)
    if (k === 'boolean') return v ? 'true' : 'false'
    return String(v)
  }

  // svelte-ignore state_referenced_locally
  const kind = kindOf(setting.value)
  // svelte-ignore state_referenced_locally
  let text = $state(toText(setting.value, kind))
  let status = $state<'idle' | 'saving' | 'saved'>('idle')
  let error = $state('')
  let id = $derived(`set-${setting.key}`)
  // Text of several lines, a prompt above all, gets a box across the whole
  // row, tall enough to read it, rather than a field of one line.
  // svelte-ignore state_referenced_locally
  const long = kind === 'string' && (setting.value as string).includes('\n')

  let parsed = $derived.by((): { ok: true; value: Json } | { ok: false; msg: string } => {
    switch (kind) {
      case 'boolean':
        return { ok: true, value: text === 'true' }
      case 'number': {
        const n = Number(text.replace(',', '.'))
        return text.trim() !== '' && Number.isFinite(n) ? { ok: true, value: n } : { ok: false, msg: 'Enter a number' }
      }
      case 'string':
        return { ok: true, value: text }
      case 'list':
        return { ok: true, value: text.split('\n').map((s) => s.trim()).filter(Boolean) }
      case 'json':
        try {
          return { ok: true, value: JSON.parse(text) as Json }
        } catch {
          return { ok: false, msg: 'Not valid JSON. Check the brackets, quotes and commas.' }
        }
    }
  })

  let lines = $derived(Math.min(long ? 18 : 10, Math.max(3, text.split('\n').length + 1)))

  // A setting saves the moment it is left or Enter is pressed, the way
  // System Settings does, and Undo in the Edit menu puts the value before
  // it back.
  async function save(e?: Event) {
    e?.preventDefault()
    const next = parsed.ok ? parsed.value : undefined
    if (next === undefined || JSON.stringify(next) === JSON.stringify(setting.value)) return
    status = 'saving'
    error = ''
    try {
      const updated = await api.patchSetting(setting.key, next)
      shown = JSON.stringify(updated.value)
      onsaved(updated)
      text = toText(updated.value, kind)
      status = 'saved'
    } catch (err) {
      status = 'idle'
      text = toText(setting.value, kind)
      error = errorText(err)
    }
  }

  // A value the app holds now that this row did not save, put back by
  // Undo or done again by Redo, shows at once.
  // svelte-ignore state_referenced_locally
  let shown = JSON.stringify(setting.value)
  $effect(() => {
    const now = JSON.stringify(setting.value)
    if (now === shown) return
    shown = now
    text = toText(setting.value, kind)
    status = 'idle'
    error = ''
  })
  // A setting Undo or Redo changed comes into view, however far down
  // Settings it is.
  let row: HTMLFormElement | undefined = $state()
  $effect(() => {
    if (undoHistory.reveal((t) => t.kind === 'setting' && t.key === setting.key)) void tick().then(() => row && bringIntoView(row))
  })

  function toggle() {
    text = text === 'true' ? 'false' : 'true'
    save()
  }

  // Enter saves a one-line field, Escape puts back what is saved.
  function keydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      text = toText(setting.value, kind)
      ;(e.currentTarget as HTMLElement).blur()
    }
  }
</script>

<form class="setting" class:long class:bare onsubmit={save} novalidate bind:this={row}>
  {#if !bare}
    <div class="info">
      <label for={id} class="desc">{setting.description || setting.key}</label>
      <span class="key num" title="Changed {fmtAgo(setting.updated_at)}">{setting.key}</span>
    </div>
  {/if}
  <div class="editor" class:bool={kind === 'boolean'}>
    {#if kind === 'boolean'}
      <button {id} type="button" role="switch" class="switch" aria-checked={text === 'true'} onclick={toggle} disabled={status === 'saving'}>
        <span class="knob"></span>
        <span class="sr-only">{setting.description}</span>
      </button>
    {:else if kind === 'number'}
      <input {id} class="input num-input" type="text" inputmode="decimal" bind:value={text} oninput={() => (status = 'idle')}
        onblur={() => save()} onkeydown={keydown} aria-invalid={!parsed.ok ? 'true' : undefined} />
    {:else if long}
      <textarea {id} class="textarea" rows={lines} bind:value={text} oninput={() => (status = 'idle')}
        onblur={() => save()} onkeydown={keydown} aria-labelledby={labelledby}></textarea>
    {:else if kind === 'string'}
      <input {id} class="input" type="text" bind:value={text} oninput={() => (status = 'idle')} onblur={() => save()} onkeydown={keydown} />
    {:else}
      <textarea {id} class="textarea" class:code={kind === 'json'} rows={lines} bind:value={text} oninput={() => (status = 'idle')}
        onblur={() => save()} onkeydown={keydown} aria-invalid={!parsed.ok ? 'true' : undefined} spellcheck="false"></textarea>
    {/if}
  </div>
  <p class="foot small" aria-live="polite">
    {#if !parsed.ok}
      <span class="field-error">{parsed.msg}</span>
    {:else if error}
      <span class="field-error">{error}</span>
    {:else if status === 'saving'}
      <span class="faint">Saving</span>
    {:else if status === 'saved'}
      <span class="faint">Saved</span>
    {:else if kind === 'list'}
      <span class="faint">One per line</span>
    {/if}
  </p>
</form>

<style>
  .setting {
    display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 4px 24px; padding: 14px 16px; align-items: center;
  }
  .info { display: grid; gap: 2px; min-width: 0; }
  .desc { font-weight: 500; font-size: 14px; }
  .key { font-size: 12px; color: var(--ink-3); overflow-wrap: anywhere; }
  .editor { display: flex; gap: var(--gap-control); align-items: center; justify-content: flex-end; min-width: 0; }
  .editor .textarea { width: min(340px, 100%); }
  .long { grid-template-columns: minmax(0, 1fr); }
  .long .editor { justify-content: stretch; }
  .long .editor .textarea { width: 100%; }
  .long .foot { justify-content: flex-start; }
  .bare { padding: 0; }
  .editor .input { width: 220px; }
  .num-input { width: 96px !important; text-align: right; font-variant-numeric: tabular-nums; }
  .foot { grid-column: 1 / -1; display: flex; gap: var(--gap-control); justify-content: flex-end; }
  .foot:empty { display: none; }
  .switch {
    width: 40px; height: 24px; border-radius: 999px; border: 1px solid var(--line-strong); background: var(--surface-2);
    position: relative; cursor: pointer; padding: 0; flex: none;
  }
  .switch .knob {
    position: absolute; top: 2px; left: 2px; width: 18px; height: 18px; border-radius: 50%; background: var(--surface);
    border: 1px solid var(--line-strong); transition: transform .15s;
  }
  .switch:hover:enabled { border-color: var(--line-hover); background: var(--surface-hover); }
  .switch:active:enabled { background: var(--surface-press); }
  .switch[aria-checked="true"] { background: var(--accent); border-color: var(--accent); --ring: var(--accent); }
  .switch[aria-checked="true"]:hover:enabled { background: var(--accent-hover); border-color: var(--accent-hover); }
  .switch[aria-checked="true"]:active:enabled { background: var(--accent-press); border-color: var(--accent-press); }
  .switch:disabled { background: var(--disabled); border-color: transparent; cursor: default; }
  .switch[aria-checked="true"] .knob { transform: translateX(16px); }
  @media (max-width: 760px) {
    .setting { grid-template-columns: minmax(0, 1fr); }
    .editor { justify-content: flex-start; }
    .foot { justify-content: flex-start; }
  }
</style>
