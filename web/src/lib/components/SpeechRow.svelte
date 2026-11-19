<script lang="ts">
  import { api, type ModelProgress, type SpeechModel } from '../api'
  import { fmtLeft, size } from '../format'
  import { speechWatch } from '../speech.svelte'
  import ModelRow from './ModelRow.svelte'

  // The speech model Dictate hears with, in the row every model has, see
  // ModelRow. What is its own: it is unpacked after it arrives, which is
  // told like the download, and what it takes is what it unpacks to.

  function downloading(m: SpeechModel, p: ModelProgress | undefined): string {
    // Unpacking is told like the download, against what the model takes
    // once unpacked, so neither is work with no end in sight.
    const unpacking = p?.text === 'unpacking'
    const whole = unpacking ? m.unpacked : m.download
    const done = p && p.fraction > 0 ? `${size(p.fraction * whole)} of ${size(whole)}` : size(whole)
    const left = p && p.remaining > 0 ? `, ${fmtLeft(p.remaining)}` : ''
    return `${unpacking ? 'Unpacking' : 'Downloading'}, ${done}${left}`
  }
</script>

<ModelRow id="speech" heading="Speech model" watch={speechWatch}
  install={api.installSpeechModel} cancel={api.cancelSpeechDownload} remove={api.removeSpeechModel}
  room={(m) => m.unpacked} {downloading}
  ready={(m) => ({ text: `By ${m.maker}. ${m.languages}.`, tone: '', state: 'ok' })}
  cancelTitle={(p) => (p?.text === 'unpacking'
    ? 'Stop unpacking. The download stays, and Download unpacks it again'
    : 'Stop the download. What has arrived stays, and Download carries on from it')}
  downloadTitle={(m) => `Fetch ${m.title}, ${size(m.download)}. Dictate hears with it once it is here`}>
  {#snippet about()}
    What you dictate in Report a Problem is heard on this Mac, and nothing of it is sent anywhere. The
    model is downloaded once from whoever published it, into <b>{speechWatch.state.folder || 'the app’s folder'}</b>.
  {/snippet}
  {#snippet gives(m)}
    It gives back {size(m.unpacked)}. Dictate hears with it, so it has to be
    downloaded again, {size(m.download)}, before you can dictate.
  {/snippet}
</ModelRow>
