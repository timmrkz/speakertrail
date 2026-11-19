<script lang="ts">
  import { api, type LanguageModel, type ModelProgress } from '../api'
  import { fmtLeft, size } from '../format'
  import { languageWatch } from '../language.svelte'
  import ModelRow, { type Line } from './ModelRow.svelte'

  // The language model that drafts first messages, in the row every model
  // has, see ModelRow: the model this Mac holds comfortably, FrameFairy's
  // Settings, Clip model, without its list to pick from. What is its own:
  // a Mac that has the model in FrameFairy's folder takes it from there,
  // and it needs llama-server in the app and the memory to run it.

  function downloading(m: LanguageModel, p: ModelProgress | undefined): string {
    if (p?.text === 'copying') return 'Taking FrameFairy’s copy, checking it first'
    const done = p && p.fraction > 0 ? `${size(p.fraction * m.download)} of ${size(m.download)}` : size(m.download)
    const left = p && p.remaining > 0 ? `, ${fmtLeft(p.remaining)}` : ''
    return `Downloading, ${done}${left}`
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
  downloadTitle={(m) => `Fetch ${m.title}, ${size(m.download)}, or take FrameFairy’s copy. Drafts are written with it once it is here`}>
  {#snippet about()}
    Draft on a person writes a first message to them on this Mac, from their profile and the prompt
    below. Nothing of it is sent anywhere. The model is the largest this Mac holds comfortably, run
    with llama.cpp, and downloaded once from Hugging Face into <b>{languageWatch.state.folder || 'the app’s folder'}</b>,
    or taken from FrameFairy’s folder when it is there.
  {/snippet}
  {#snippet gives(m)}
    It gives back {size(m.download)}, unless FrameFairy keeps it too. Drafts are written with it, so it
    has to be downloaded again before the next draft.
  {/snippet}
</ModelRow>
