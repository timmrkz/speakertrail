<script lang="ts">
  import { untrack } from 'svelte'
  import { api, type LinkedinWhere } from '../api'
  import { undoHistory } from '../history.svelte'
  import { errorText } from '../toast.svelte'
  import Pick from './Pick.svelte'

  // Where the LinkedIn button opens a profile: one of the browser profiles
  // on this Mac, the one logged in to LinkedIn. The list is read from the
  // browsers each time Settings opens, so a profile made since shows up.
  // A pick saves at once, and Undo in the Edit menu puts the one before
  // it back.
  let where = $state<LinkedinWhere | null>(null)
  let status = $state<'idle' | 'saving' | 'saved'>('idle')
  let error = $state('')

  // What Undo and Redo took back or did again shows at once. Only a
  // change after Settings opened counts.
  const undoneAt = undoHistory.changed
  $effect(() => {
    if (undoHistory.changed === undoneAt) return
    untrack(() => {
      status = 'idle'
      api.linkedin().then((w) => (where = w)).catch((e) => (error = errorText(e)))
    })
  })

  // The LinkedIn button sends here when nothing is chosen, so the menu is
  // brought into view and focused, ready to pick.
  let menu = $state<HTMLButtonElement>()
  api.linkedin().then((w) => {
    where = w
    if (location.hash === '#linkedin') {
      requestAnimationFrame(() => {
        menu?.scrollIntoView({ block: 'center' })
        menu?.focus()
      })
    }
  }).catch((e) => (error = errorText(e)))

  const value = (app: string, folder: string) => JSON.stringify([app, folder])
  const chosen = $derived(where?.known ? value(where.app, where.folder) : '')
  // Every profile, under the browser it belongs to.
  const options = $derived(
    (where?.browsers ?? []).flatMap((b) => b.profiles.map((p) => ({ value: value(b.app, p.folder), label: p.name, group: b.app }))),
  )
  const profiles = $derived(where?.browsers.reduce((n, b) => n + b.profiles.length, 0) ?? 0)
  const name = $derived.by(() => {
    const b = where?.browsers.find((x) => x.app === where?.app)
    const p = b?.profiles.find((x) => x.folder === where?.folder)
    return p ? `${where?.app} · ${p.name}` : ''
  })

  async function choose(v: string) {
    if (!where || !v) return
    const [app, folder] = JSON.parse(v) as [string, string]
    status = 'saving'
    error = ''
    try {
      where = await api.setLinkedin(app, folder)
      status = 'saved'
    } catch (e) {
      status = 'idle'
      error = errorText(e)
    }
  }
</script>

<section class="group" id="linkedin">
  <h2>LinkedIn</h2>
  <div class="list">
    <div class="setting">
      <div class="info">
        <span class="desc">LinkedIn opens in</span>
        <span class="state small">
          {#if !where}&nbsp;
          {:else if where.known}{name}
          {:else if where.folder}The profile chosen is gone. Choose another
          {:else if profiles}Not chosen yet. Pick the profile logged in to LinkedIn
          {:else}No Chrome, Brave, Edge, Vivaldi or Chromium on this Mac{/if}
        </span>
      </div>
      <div class="editor">
        <Pick bind:trigger={menu} id="linkedin-profile" label="LinkedIn opens in" value={chosen} options={options}
          placeholder={profiles ? 'Choose a profile' : 'No browser profiles'} disabled={!profiles || status === 'saving'}
          tone={where && !where.known && where.folder ? 'warn' : undefined} align="right"
          title="The browser profile the LinkedIn button opens profiles in. Pick the one logged in to LinkedIn"
          onpick={(v) => choose(v)} />
      </div>
      <p class="foot small" aria-live="polite">
        {#if error}<span class="field-error">{error}</span>
        {:else if status === 'saving'}<span class="faint">Saving</span>
        {:else if status === 'saved'}<span class="faint">Saved</span>{/if}
      </p>
    </div>
  </div>
</section>

<style>
  .group { display: grid; gap: 10px; scroll-margin-top: 16px; }
  .group h2 { font-size: 15px; margin: 0; }
  /* The same row as every other setting, see SettingRow. */
  .setting {
    display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 4px 24px; padding: 14px 16px; align-items: center;
  }
  .info { display: grid; gap: 2px; min-width: 0; }
  .desc { font-weight: 500; font-size: 14px; }
  .state { color: var(--ink-3); overflow-wrap: anywhere; }
  .editor { display: flex; justify-content: flex-end; min-width: 0; }
  .foot { grid-column: 1 / -1; display: flex; gap: var(--gap-control); justify-content: flex-end; margin: 0; }
  .foot:empty { display: none; }
  @media (max-width: 760px) {
    .setting { grid-template-columns: minmax(0, 1fr); }
    .editor, .foot { justify-content: flex-start; }
  }
</style>
