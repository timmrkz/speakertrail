<script lang="ts">
  import { api } from '../api'
  import { fmtLeft, plural, size } from '../format'
  import { speechWatch } from '../speech.svelte'
  import { errorText, toast } from '../toast.svelte'
  import Busy from './Busy.svelte'
  import Confirm from './Confirm.svelte'
  import Icon from './Icon.svelte'

  // The speech model Dictate hears with, in an area of its own: the model,
  // the one line under it that says how it stands, Download beside it
  // while it is not here, Cancel with the beam and the fill while it
  // comes, and under it, folded, the models on this Mac with the room they
  // take, each with a bin. Ported from FrameFairy's Settings, Speech
  // model, its ModelCard.svelte, ModelList.svelte and install.svelte.ts,
  // in the rows every setting here has. See docs/api.md, Speech models.
  const watch = speechWatch
  $effect(() => watch.watch())

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
      const j = await api.installSpeechModel(model.name)
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
      await api.cancelSpeechDownload(running.id)
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
    if (!model) return { text: 'No speech model is offered.', tone: 'err', state: 'err' }
    if (running || starting) {
      const p = running?.progress
      if (p?.text === 'unpacking') return { text: 'Unpacking', tone: '', state: 'busy' }
      const done = p && p.fraction > 0 ? `${size(p.fraction * model.download)} of ${size(model.download)}` : size(model.download)
      const left = p && p.remaining > 0 ? `, ${fmtLeft(p.remaining)}` : ''
      return { text: `Downloading, ${done}${left}`, tone: '', state: 'busy' }
    }
    if (failed) return { text: failed, tone: 'err', state: 'err' }
    if (!model.installed) return { text: `Not downloaded yet. ${size(model.download)}.`, tone: 'warn', state: 'warn' }
    return { text: `By ${model.maker}. ${model.languages}.`, tone: '', state: 'ok' }
  })

  // The models on this Mac are only about the room they take, so they
  // stay folded until somebody wants some of it back.
  let open = $state(false)
  const here = $derived(models.filter((m) => m.installed))
  const room = $derived(size(here.reduce((sum, m) => sum + m.unpacked, 0)))

  // The model the box is asking about.
  let removing = $state<string | null>(null)
  const removingModel = $derived(models.find((m) => m.name === removing))

  async function remove(name: string) {
    removing = null
    try {
      await api.removeSpeechModel(name)
    } catch (err) {
      toast.show(errorText(err), 'bad')
    }
    watch.read()
  }
</script>

<section class="group speech">
  <h2 title="What you dictate in Report a Problem is heard on this Mac, and nothing of it is sent anywhere. The model is downloaded once from whoever published it, into {watch.state.folder || 'the app’s folder'}.">Speech model</h2>
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
        <span class="desc">{model?.title ?? 'Speech model'}</span>
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
            title="Fetch {model.title}, {size(model.download)}. Dictate hears with it once it is here">
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
            <span class="room small count">{size(m.unpacked)}</span>
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
      It gives back {size(removingModel.unpacked)}. Dictate hears with it, so it has to be
      downloaded again, {size(removingModel.download)}, before you can dictate.
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
