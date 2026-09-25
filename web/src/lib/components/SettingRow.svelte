<script lang="ts">
  import { api, type Json, type Setting } from '../api'
  import { fmtAgo } from '../format'
  import { errorText } from '../toast.svelte'

  // One setting with an editor that fits its JSON type.
  let { setting, onsaved }: { setting: Setting; onsaved: (s: Setting) => void } = $props()

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

  let dirty = $derived(parsed.ok && JSON.stringify(parsed.value) !== JSON.stringify(setting.value))
  let lines = $derived(Math.min(10, Math.max(3, text.split('\n').length + 1)))

  async function save(e?: SubmitEvent) {
    e?.preventDefault()
    if (!parsed.ok || !dirty) return
    status = 'saving'
    error = ''
    try {
      const updated = await api.patchSetting(setting.key, parsed.value)
      onsaved(updated)
      text = toText(updated.value, kind)
      status = 'saved'
    } catch (err) {
      status = 'idle'
      if (kind === 'boolean') text = toText(setting.value, kind)
      error = errorText(err)
    }
  }

  function toggle() {
    text = text === 'true' ? 'false' : 'true'
    save()
  }
</script>

<form class="setting" onsubmit={save} novalidate>
  <div class="info">
    <label for={id} class="desc">{setting.description || setting.key}</label>
    <span class="key num">{setting.key}</span>
  </div>
  <div class="editor" class:bool={kind === 'boolean'}>
    {#if kind === 'boolean'}
      <button {id} type="button" role="switch" class="switch" aria-checked={text === 'true'} onclick={toggle} disabled={status === 'saving'}>
        <span class="knob"></span>
        <span class="sr-only">{setting.description}</span>
      </button>
      <span class="small muted">{text === 'true' ? 'On' : 'Off'}</span>
    {:else if kind === 'number'}
      <input {id} class="input num-input" type="text" inputmode="decimal" bind:value={text} oninput={() => (status = 'idle')}
        aria-invalid={!parsed.ok ? 'true' : undefined} />
    {:else if kind === 'string'}
      <input {id} class="input" type="text" bind:value={text} oninput={() => (status = 'idle')} />
    {:else}
      <textarea {id} class="textarea" class:code={kind === 'json'} rows={lines} bind:value={text} oninput={() => (status = 'idle')}
        aria-invalid={!parsed.ok ? 'true' : undefined} spellcheck="false"></textarea>
    {/if}
    {#if kind !== 'boolean'}
      <button class="btn" class:primary={dirty} type="submit" disabled={!dirty || status === 'saving'}>
        {status === 'saving' ? 'Saving' : status === 'saved' && !dirty ? 'Saved' : 'Save'}
      </button>
    {/if}
  </div>
  <p class="foot small" aria-live="polite">
    {#if !parsed.ok}
      <span class="field-error">{parsed.msg}</span>
    {:else if error}
      <span class="field-error">{error}</span>
    {:else if kind === 'list'}
      <span class="faint">One per line. Changed {fmtAgo(setting.updated_at)}</span>
    {:else}
      <span class="faint">Changed {fmtAgo(setting.updated_at)}</span>
    {/if}
  </p>
</form>

<style>
  .setting {
    display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.1fr); gap: 6px 20px; padding: 16px; align-items: start;
  }
  .info { display: grid; gap: 2px; min-width: 0; }
  .desc { font-weight: 600; font-size: 14px; }
  .key { font-size: 12px; color: var(--ink-3); overflow-wrap: anywhere; }
  .editor { display: flex; gap: 8px; align-items: flex-start; min-width: 0; }
  .editor .input, .editor .textarea { flex: 1; }
  .num-input { max-width: 140px; font-family: var(--mono); }
  .foot { grid-column: 2; min-height: 0; }
  .switch {
    width: 44px; height: 26px; border-radius: 999px; border: 1px solid var(--line-strong); background: var(--surface-2);
    position: relative; cursor: pointer; padding: 0; flex: none;
  }
  .switch .knob {
    position: absolute; top: 2px; left: 2px; width: 20px; height: 20px; border-radius: 50%; background: var(--surface);
    box-shadow: 0 1px 2px rgba(0, 0, 0, .25); transition: transform .15s;
  }
  .switch[aria-checked="true"] { background: var(--accent); border-color: var(--accent); }
  .switch[aria-checked="true"] .knob { transform: translateX(18px); }
  .editor.bool { align-items: center; min-height: var(--control-h); }
  @media (max-width: 760px) {
    .setting { grid-template-columns: minmax(0, 1fr); padding: 14px; }
    .foot { grid-column: 1; }
  }
</style>
