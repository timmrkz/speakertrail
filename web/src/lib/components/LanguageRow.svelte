<script lang="ts">
  import { api, type LanguageModel, type ModelProgress } from '../api'
  import { fmtLeft, size } from '../format'
  import { languageWatch } from '../language.svelte'
  import ModelRow, { type Line } from './ModelRow.svelte'

  // The language model that drafts first messages, in the row every model
  // has, see ModelRow, with the list to pick it from that FrameFairy's
  // Settings has for its clip model: every model, the largest first by
  // the size beside it, the tick on those downloaded and a grey download
  // arrow on the rest, and a note beside the size where this Mac is short
  // of memory for it. Picking one makes it the one that drafts, here or
  // not, and Download fetches it. What is its own:
  // a Mac that has the model in FrameFairy's folder takes it from there,
  // and it needs llama-server in the app and the memory to run it.

  function downloading(m: LanguageModel, p: ModelProgress | undefined): string {
    if (p?.text === 'copying') return 'Taking FrameFairy’s copy, checking it first'
    const done = p && p.fraction > 0 ? `${size(p.fraction * m.download)} of ${size(m.download)}` : size(m.download)
    const left = p && p.remaining > 0 ? `, ${fmtLeft(p.remaining)}` : ''
    return `Downloading, ${done}${left}`
  }

  // How this Mac stands with a model, said beside its size when it is short
  // of memory for it, as FrameFairy's fitNote words it.
  function fitNote(fit: LanguageModel['fit']): string {
    if (fit === 'tight') return 'tight on this Mac'
    if (fit === 'too big') return 'too big for this Mac'
    return ''
  }

  const options = $derived(
    [...languageWatch.state.models]
      .sort((a, b) => b.download - a.download)
      .map((m) => {
        const note = fitNote(m.fit)
        return {
          value: m.name,
          label: m.title,
          detail: note ? `${size(m.download)}, ${note}` : size(m.download),
          warn: !!note,
          group: 'On this Mac',
          mark: m.installed ? { icon: 'check' as const, title: 'Downloaded' } : { icon: 'download' as const, title: 'Not downloaded yet', faint: true },
        }
      }),
  )

  // The answer is how the models stand with it picked, shown at once.
  async function use(name: string) {
    languageWatch.state = await api.useLanguageModel(name)
    languageWatch.loaded = true
  }

  function ready(m: LanguageModel): Line {
    if (!languageWatch.state.server) return { text: 'This build of the app has no llama-server to run it.', tone: 'err', state: 'err' }
    if (m.fit === 'too big') return { text: `It needs ${size(m.needs)} of memory, more than this Mac has.`, tone: 'err', state: 'err' }
    return { text: `By ${m.maker}. Takes about ${size(m.needs)} of memory while it writes.`, tone: '', state: 'ok' }
  }
</script>

<ModelRow id="language" heading="Language model" watch={languageWatch}
  install={api.installLanguageModel} cancel={api.cancelLanguageDownload} remove={api.removeLanguageModel}
  room={(m) => m.download} {downloading} {ready}
  cancelTitle={() => 'Stop the download. What has arrived stays, and Download carries on from it'}
  pick={{ head: 'Draft with', label: 'Language model', title: 'The model that writes drafts. One not downloaded yet is fetched with Download', options, onpick: use }}
  downloadTitle={(m) => `Fetch ${m.title}, ${size(m.download)}, or take FrameFairy’s copy. Drafts are written with it once it is here`}>
  {#snippet about()}
    Draft on a person writes a first message to them on this Mac, from their profile and the prompt
    below. Nothing of it is sent anywhere. The model is the one picked here, at first the largest
    this Mac holds comfortably, run with llama.cpp, and downloaded once from Hugging Face into <b>{languageWatch.state.folder || 'the app’s folder'}</b>,
    or taken from FrameFairy’s folder when it is there.
  {/snippet}
  {#snippet gives(m)}
    It gives back {size(m.download)}, unless FrameFairy keeps it too. Drafts are written with it, so it
    has to be downloaded again before the next draft.
  {/snippet}
</ModelRow>
