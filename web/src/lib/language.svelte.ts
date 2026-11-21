// The language models, as the app says they stand: which are here, which
// drafts, and the latest download with how far it has come. The app sends
// the event language whenever that changes, and this is asked once when a
// screen that shows it opens and after each press. The same as the speech
// models, see speech.svelte.ts and cmd/speakertrail-app/languagemodels.go.
import { Events } from '@wailsio/runtime'
import { api, type LanguageModel, type LanguageState } from './api'

class LanguageWatch {
  state = $state<LanguageState>({ models: [], folder: '', server: false, standing: { mark: 'ok', why: '' } })
  // Whether the app has answered once, so nothing is said about the
  // models before it is known.
  loaded = $state(false)
  // The model that drafts, as the app marks it.
  readonly inUse = $derived<LanguageModel | undefined>(this.state.models.find((m) => m.inUse))
  private users = 0
  private off: (() => void) | undefined

  async read() {
    try {
      this.state = await api.language()
      this.loaded = true
    } catch {
      // Settings says what went wrong when it asks itself.
    }
  }

  // Follows the event while a screen that shows it is open. Returns what
  // stops following, for the screen's effect to call.
  watch(): () => void {
    if (this.users++ === 0) {
      this.off = Events.On('language', (ev) => {
        this.state = ev.data as LanguageState
        this.loaded = true
      })
    }
    this.read()
    return () => {
      if (--this.users === 0) {
        this.off?.()
        this.off = undefined
      }
    }
  }
}

export const languageWatch = new LanguageWatch()
