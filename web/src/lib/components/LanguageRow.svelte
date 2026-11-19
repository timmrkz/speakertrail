<script lang="ts">
  import { api } from '../api'
  import { fmtLeft, plural, size } from '../format'
  import { languageWatch } from '../language.svelte'
  import { errorText, toast } from '../toast.svelte'
  import Busy from './Busy.svelte'
  import Confirm from './Confirm.svelte'
  import Icon from './Icon.svelte'
  import Info from './Info.svelte'

  // The language model that drafts first messages, in an area of its own,
  // the same as the speech model's, see SpeechRow: the model this Mac
  // holds comfortably, the one line under it that says how it stands,
  // Download beside it while it is not here, Cancel with the beam and the
  // fill while it comes, and under it, folded, the models on this Mac with
  // the room they take, each with a bin. FrameFairy's Settings, Clip
  // model, without its list to pick from. See docs/api.md, Language models.
  const watch = languageWatch
  $effect(() => watch.watch())

  // A draft that needs the model sends here, so the area is brought into
  // view once it knows how the model stands.
  let area = $state<HTMLElement>()
  let shown = false
  $effect(() => {
    if (shown || !watch.loaded || location.hash !== '#language') return
    shown = true
    requestAnimationFrame(() => area?.scrollIntoView({ block: 'center' }))
  })

  const models = $derived(watch.state.models)
  const job = $derived(watch.state.job)
  const running = $derived(job && (job.state === 'running' || job.state === 'queued') ? job : undefined)
  // The one in use is the one here, or the one the app offers.
  const model = $derived(watch.inUse ?? models.find((m) => m.recommended) ?? models[0])

  // Download pressed and not answered yet, so a click shows at once, and
  // Cancel pressed, which takes a moment to reach the download.
  let starting = $state(false)
  let cancelling = $state(false)
  // Why the last download did not start or stopped, said in its line.
  let failed = $state('')
  // The download whose end was seen, so the line says why it failed once.
  let seen = ''
  $effect(() => {
    const now = job ? `${job.id}:${job.state}` : ''
    if (now === seen) return
    seen = now
    if (!running) cancelling = false
    if (job?.state === 'failed') failed = job.error ?? 'The download stopped.'
  })

  async function download() {
    if (!model || model.installed) return
    failed = ''
    starting = true
    try {
      const j = await api.installLanguageModel(model.name)
      if (j.state === 'failed') failed = j.error ?? 'The download did not start.'
      watch.state = { ...watch.state, job: j }
    } catch (err) {
      failed = errorText(err)
    }
    starting = false
    // The app tells the rest as it comes. Asked once more, in case the
    // download ended before the answer came back.
    watch.read()
  }

  async function cancel() {
    if (!running) return
    cancelling = true
    try {
      await api.cancelLanguageDownload(running.id)
    } catch (err) {
      cancelling = false
      toast.show(errorText(err), 'bad')
    }
    watch.read()
  }

  const fraction = $derived(running?.progress && running.progress.fraction >= 0 ? running.progress.fraction : -1)

  // The one line under the model, one line in every state.
  const line = $derived.by((): { text: string; tone: '' | 'warn' | 'err'; state: 'ok' | 'busy' | 'warn' | 'err' } => {
    if (!watch.loaded) return { text: '', tone: '', state: 'busy' }
    if (!model) return { text: 'No language model is offered.', tone: 'err', state: 'err' }
    if (running || starting) {
      const p = running?.progress
      const done = p && p.fraction > 0 ? `${size(p.fraction * model.download)} of ${size(model.download)}` : size(model.download)
      const left = p && p.remaining > 0 ? `, ${fmtLeft(p.remaining)}` : ''
      return { text: `Downloading, ${done}${left}`, tone: '', state: 'busy' }
    }
    if (failed) return { text: failed, tone: 'err', state: 'err' }
    if (!model.installed) return { text: `Not downloaded yet. ${size(model.download)}.`, tone: 'warn', state: 'warn' }
    if (!watch.state.server) return { text: 'This build of the app has no llama-server to run it.', tone: 'err', state: 'err' }
    if (model.fit === 'too big') return { text: `It needs ${size(model.needs)} of memory, more than this Mac has.`, tone: 'err', state: 'err' }
    return { text: `By ${model.maker}. Takes about ${size(model.needs)} of memory while it writes.`, tone: '', state: 'ok' }
  })

  // The models on this Mac are only about the room they take, so they
  // stay folded until somebody wants some of it back.
  let open = $state(false)
  const here = $derived(models.filter((m) => m.installed))
  const room = $derived(size(here.reduce((sum, m) => sum + m.download, 0)))

  // The model the box is asking about.
  let removing = $state<string | null>(null)
  const removingModel = $derived(models.find((m) => m.name === removing))

  async function remove(name: string) {
    removing = null
    try {
      await api.removeLanguageModel(name)
    } catch (err) {
      toast.show(errorText(err), 'bad')
    }
    watch.read()
  }
</script>

<section class="group language" id="language" bind:this={area}>
  <div class="headrow">
    <h2>Language model</h2>
    <span class="ask">
      <Info label="About the language model" side="left">
        Draft on a person writes a first message to them on this Mac, from their profile and the prompt
        below. Nothing of it is sent anywhere. The model is the largest this Mac holds comfortably, run
        with llama.cpp, and downloaded once from Hugging Face into <b>{watch.state.folder || 'the app’s folder'}</b>.
      </Info>
    </span>
  </div>
  <div class="list">
    <div class="setting">
      <span class="mark {line.state}" aria-hidden="true">
        {#if line.state === 'busy'}
          <span class="dot busy"></span>
        {:else if line.state === 'ok'}
          <Icon name="check" size={14} />
        {:else}
          <Icon name="alert" size={14} />
        {/if}
      </span>
      <div class="info">
        <span class="desc">{model?.title ?? 'Language model'}</span>
        <span class="line small" class:warn={line.tone === 'warn'} class:error={line.tone === 'err'} title={line.text}>{line.text}</span>
      </div>
      <div class="actions">
        {#if running}
          <!-- The download carries the beam and the fill in the one control
               that can do something about it. -->
          <button class="btn act" type="button" disabled={cancelling} onclick={cancel}
            title="Stop the download. What has arrived stays, and Download carries on from it">
            <Busy {fraction} />{cancelling ? 'Cancelling' : 'Cancel'}
          </button>
        {:else if model && !model.installed}
          <button class="btn act" type="button" disabled={starting} onclick={download}
            title="Fetch {model.title}, {size(model.download)}. Drafts are written with it once it is here">
            {#if starting}<Busy />{/if}{starting ? 'Starting' : 'Download'}
          </button>
        {/if}
      </div>
    </div>
    {#if here.length > 0}
      <button class="setting disclose" type="button" aria-expanded={open} onclick={() => (open = !open)}>
        <span class="mark" aria-hidden="true"><Icon name="folder" size={14} /></span>
        <span class="info">
          <span class="desc">Downloaded models</span>
          <span class="line small count">{plural(here.length, 'model')} on this Mac, {room}.</span>
        </span>
        <span class="chevron" class:open aria-hidden="true"><Icon name="chevron" size={14} /></span>
      </button>
      {#if open}
        {#each here as m (m.name)}
          <div class="setting model">
            <span class="name" title={m.about}>{m.title}</span>
            <span class="maker small">{m.maker}</span>
            <span class="room small count">{size(m.download)}</span>
            <button class="btn quiet icon danger" type="button" title="Remove {m.title} from this Mac"
              aria-label="Remove {m.title}" aria-haspopup="dialog" onclick={() => (removing = m.name)}>
              <Icon name="trash" />
            </button>
          </div>
        {/each}
      {/if}
    {/if}
  </div>
</section>

{#if removingModel}
  <Confirm title="Remove {removingModel.title}?" oncancel={() => (removing = null)}>
    <p>
      It gives back {size(removingModel.download)}. Drafts are written with it, so it has to be
      downloaded again before the next draft.
    </p>
    {#snippet actions()}
      <button class="btn" type="button" onclick={() => (removing = null)}>Cancel</button>
      <button class="btn danger" type="button" onclick={() => remove(removingModel.name)}>Remove it</button>
    {/snippet}
  </Confirm>
{/if}

<style>
  .group { display: grid; gap: 10px; }
  .group h2 { font-size: 15px; margin: 0; }
  /* The heading with its info mark at the far end, as in FrameFairy. */
  .headrow { display: flex; align-items: center; justify-content: space-between; gap: 6px; }
  .headrow .ask { align-self: flex-start; margin: 1px 1px 0 0; }
  /* The same row as every other setting, see SettingRow, with the mark
     before it that LogsRow's toasts have. */
  .setting {
    display: grid; grid-template-columns: auto minmax(0, 1fr) auto; gap: 4px 10px; padding: 14px 16px; align-items: center;
  }
  .info { display: grid; gap: 2px; min-width: 0; }
  .desc { font-weight: 500; font-size: 14px; }
  /* A line under a name is one line. It is cut short, with the whole of it
     in its title, rather than wrapped, because a line that wraps when a
     button arrives beside it moves the row. */
  .line { color: var(--ink-3); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .line.warn { color: var(--warn); }
  .line.error { color: var(--bad); }
  .actions { display: flex; gap: 8px; }
  .act { position: relative; }
  .count { font-variant-numeric: tabular-nums; }
  .mark { display: flex; width: 14px; justify-content: center; color: var(--ink-3); }
  .mark.ok { color: var(--good); }
  .mark.warn { color: var(--warn); }
  .mark.err { color: var(--bad); }
  /* A row that opens what is under it: the whole row is the button, and
     the chevron at its end turns, the way a row in the Mac's own settings
     leads on. */
  .disclose { width: 100%; border: none; border-radius: 0; background: transparent; text-align: left; color: inherit; font: inherit; cursor: pointer; }
  .disclose:hover { background: var(--surface-2); }
  .chevron { display: flex; color: var(--ink-3); transition: transform .12s ease; }
  .chevron.open { transform: rotate(90deg); }
  @media (prefers-reduced-motion: reduce) { .chevron { transition: none; } }
  /* A model on this Mac, read like a table without its lines: the name,
     who made it and the room it takes, each in a column of its own. */
  .model { grid-template-columns: minmax(0, 1fr) auto auto auto; gap: 12px; padding-left: 40px; }
  .name { font-size: 14px; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .maker, .room { color: var(--ink-3); }
</style>
