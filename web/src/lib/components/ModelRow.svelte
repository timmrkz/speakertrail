<script lang="ts" module>
  import type { ModelJob, ModelProgress } from '../api'

  // What a model row needs of a model, the speech model's and the
  // language model's alike.
  export interface RowModel {
    name: string
    title: string
    maker: string
    about: string
    download: number
    recommended: boolean
    installed: boolean
    inUse: boolean
  }

  // A watch of the models, see speech.svelte.ts and language.svelte.ts.
  export interface RowWatch<M extends RowModel> {
    state: { models: M[]; folder: string; job?: ModelJob }
    readonly loaded: boolean
    readonly inUse: M | undefined
    read(): Promise<void>
    watch(): () => void
  }

  // The one line under the model.
  export type Line = { text: string; tone: '' | 'warn' | 'err'; state: 'ok' | 'busy' | 'warn' | 'err' }
</script>

<script lang="ts" generics="M extends RowModel">
  import type { Snippet } from 'svelte'
  import { plural, size } from '../format'
  import { errorText, Problem as Trouble } from '../problem.svelte'
  import Busy from './Busy.svelte'
  import Confirm from './Confirm.svelte'
  import Icon, { type IconName } from './Icon.svelte'
  import Info from './Info.svelte'
  import Pick from './Pick.svelte'
  import Problem from './Problem.svelte'

  // A model in an area of its own: the model, the one line under it that
  // says how it stands, Download beside it while it is not here, Cancel
  // with the beam and the fill while it comes, and under it, folded, the
  // models on this Mac with the room they take, each with a bin. Ported
  // from FrameFairy's Settings, its ModelCard.svelte, ModelList.svelte and
  // install.svelte.ts, which draw its speech model and its clip model
  // alike, in the rows every setting here has. SpeechRow and LanguageRow
  // say what differs. See docs/api.md, Speech models and Language models.
  let {
    id,
    heading,
    watch,
    install,
    cancel: stop,
    remove: drop,
    about,
    gives,
    room,
    downloading,
    ready,
    cancelTitle,
    downloadTitle,
    pick = undefined,
  }: {
    // The area's id and class, which Settings and the walks find it by.
    id: string
    heading: string
    watch: RowWatch<M>
    install: (name: string) => Promise<ModelJob>
    cancel: (id: string) => Promise<void>
    remove: (name: string) => Promise<void>
    // What the info mark says.
    about: Snippet
    // What Remove says it gives back and costs.
    gives: Snippet<[M]>
    // The room a model takes on the Mac.
    room: (m: M) => number
    // The line while it comes.
    downloading: (m: M, p: ModelProgress | undefined) => string
    // The line once it is here, or what stands in its way.
    ready: (m: M) => Line
    cancelTitle: (p: ModelProgress | undefined) => string
    downloadTitle: (m: M) => string
    // A list to pick the model from, where a kind has more than one, as
    // FrameFairy's Settings has for its clip model. The row then names
    // what the model does, and the list names the model.
    pick?: {
      head: string
      label: string
      title: string
      options: { value: string; label: string; detail?: string; warn?: boolean; group?: string; mark?: { icon: IconName; title: string; faint?: boolean } }[]
      onpick: (name: string) => Promise<void>
    }
  } = $props()

  $effect(() => watch.watch())

  // A control that needs the model sends here, so the area is brought into
  // view once it knows how the model stands.
  let area = $state<HTMLElement>()
  let shown = false
  $effect(() => {
    if (shown || !watch.loaded || location.hash !== `#${id}`) return
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
      const j = await install(model.name)
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

  // A model picked from the list is the one in use at once, here or not.
  // The line under it then says whether it still has to be downloaded.
  // What went wrong picking a model or cancelling its download says so
  // under its row, and removing one under that model's row, see
  // lib/problem.svelte.ts. Why a download failed is the line under its
  // name, which the app keeps.
  const rowProblem = new Trouble()
  const removeProblem = new Trouble()
  let failedOn = $state('')

  async function choose(name: string) {
    failed = ''
    rowProblem.clear()
    try {
      await pick?.onpick(name)
    } catch (err) {
      rowProblem.say(err)
      watch.read()
    }
  }

  async function cancel() {
    if (!running) return
    cancelling = true
    rowProblem.clear()
    try {
      await stop(running.id)
    } catch (err) {
      cancelling = false
      rowProblem.say(err)
    }
    watch.read()
  }

  const fraction = $derived(running?.progress && running.progress.fraction >= 0 ? running.progress.fraction : -1)

  // The one line under the model, one line in every state.
  const line = $derived.by((): Line => {
    if (!watch.loaded) return { text: '', tone: '', state: 'busy' }
    if (!model) return { text: `No ${heading.toLowerCase()} is offered.`, tone: 'err', state: 'err' }
    if (running || starting) return { text: downloading(model, running?.progress), tone: '', state: 'busy' }
    if (failed) return { text: failed, tone: 'err', state: 'err' }
    if (!model.installed) return { text: `Not downloaded yet. ${size(model.download)}.`, tone: 'warn', state: 'warn' }
    return ready(model)
  })

  // The models on this Mac are only about the room they take, so they
  // stay folded until somebody wants some of it back.
  let open = $state(false)
  const here = $derived(models.filter((m) => m.installed))
  const taken = $derived(size(here.reduce((sum, m) => sum + room(m), 0)))

  // The model the box is asking about.
  let removing = $state<string | null>(null)
  const removingModel = $derived(models.find((m) => m.name === removing))

  async function remove(name: string) {
    removing = null
    removeProblem.clear()
    failedOn = name
    try {
      await drop(name)
    } catch (err) {
      removeProblem.say(err)
    }
    watch.read()
  }
</script>

<section class="group {id}" {id} bind:this={area}>
  <div class="headrow">
    <h2>{heading}</h2>
    <span class="ask">
      <Info label="About the {heading.toLowerCase()}" side="left">{@render about()}</Info>
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
        <span class="desc">{pick ? pick.head : (model?.title ?? heading)}</span>
        <span class="line small" class:warn={line.tone === 'warn'} class:error={line.tone === 'err'} title={line.text}>{line.text}</span>
      </div>
      <div class="actions" class:picks={!!pick}>
        {#if running}
          <!-- The download carries the beam and the fill in the one control
               that can do something about it. -->
          <button class="btn act" type="button" disabled={cancelling} onclick={cancel} title={cancelTitle(running.progress)}>
            <Busy {fraction} />{cancelling ? 'Cancelling' : 'Cancel'}
          </button>
        {:else if model && !model.installed}
          <button class="btn act" type="button" disabled={starting} onclick={download} title={downloadTitle(model)}>
            {#if starting}<Busy />{/if}{starting ? 'Starting' : 'Download'}
          </button>
        {/if}
        {#if pick}
          <Pick value={model?.name ?? ''} options={pick.options} label={pick.label} title={pick.title} align="right"
            tone={line.state === 'warn' || line.state === 'err' ? line.state : undefined}
            disabled={!!running || starting} onpick={choose} />
        {/if}
      </div>
      {#if rowProblem.text}<div class="failed"><Problem of={rowProblem} /></div>{/if}
    </div>
    {#if here.length > 0}
      <button class="setting disclose" type="button" aria-expanded={open} onclick={() => (open = !open)}>
        <span class="mark" aria-hidden="true"><Icon name="folder" size={14} /></span>
        <span class="info">
          <span class="desc">Downloaded models</span>
          <span class="line small count">{plural(here.length, 'model')} on this Mac, {taken}.</span>
        </span>
        <span class="chevron" class:open aria-hidden="true"><Icon name="chevron" size={14} /></span>
      </button>
      {#if open}
        {#each here as m (m.name)}
          <div class="setting model">
            <span class="name" title={m.about}>{m.title}</span>
            <span class="maker small">{m.maker}</span>
            <span class="room small count">{size(room(m))}</span>
            <button class="btn quiet icon danger" type="button" title="Remove {m.title} from this Mac"
              aria-label="Remove {m.title}" aria-haspopup="dialog" onclick={() => (removing = m.name)}>
              <Icon name="trash" />
            </button>
            {#if failedOn === m.name && removeProblem.text}<div class="failed"><Problem of={removeProblem} /></div>{/if}
          </div>
        {/each}
      {/if}
    {/if}
  </div>
</section>

{#if removingModel}
  <Confirm title="Remove {removingModel.title}?" oncancel={() => (removing = null)}>
    <p>{@render gives(removingModel)}</p>
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
  .actions { display: flex; gap: var(--gap-control); min-width: 0; }
  /* On a narrow window the controls go under the words, as LinkedinRow's
     list does, so the line under the name keeps its room. */
  @media (max-width: 760px) {
    .actions.picks { grid-column: 2 / -1; }
  }
  .act { position: relative; }
  /* What went wrong takes the whole width of its row, under the rest. */
  .failed { grid-column: 1 / -1; min-width: 0; }
  .count { font-variant-numeric: tabular-nums; }
  .mark { display: flex; width: 14px; justify-content: center; color: var(--ink-3); }
  .mark.ok { color: var(--good); }
  .mark.warn { color: var(--warn); }
  .mark.err { color: var(--bad); }
  /* A row that opens what is under it: the whole row is the button, and
     the chevron at its end turns, the way a row in the Mac's own settings
     leads on. */
  .disclose { width: 100%; border: none; border-radius: 0; background: transparent; text-align: left; color: inherit; font: inherit; cursor: pointer; }
  .disclose:hover { background: var(--hover); }
  .disclose:active { background: var(--press); }
  .chevron { display: flex; color: var(--ink-3); transition: transform .12s ease; }
  .chevron.open { transform: rotate(90deg); }
  @media (prefers-reduced-motion: reduce) { .chevron { transition: none; } }
  /* A model on this Mac, read like a table without its lines: the name,
     who made it and the room it takes, each in a column of its own. */
  .model { grid-template-columns: minmax(0, 1fr) auto auto auto; gap: 12px; padding-left: 40px; }
  .name { font-size: 14px; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .maker, .room { color: var(--ink-3); }
</style>
